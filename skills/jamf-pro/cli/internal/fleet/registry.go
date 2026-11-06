// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: the local registry of Jamf Pro instances an MSP manages.

// Package fleet keeps the list of Jamf Pro instances (one per client) and
// runs this CLI against each of them with that instance's own API client.
// Secrets are never written to disk: every instance's client secret comes from
// the one JAMF_PRO_FLEET_SECRETS variable, by instance name (see ParseSecrets).
package fleet

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"jamf-pro-pp-cli/internal/cliutil"
)

// Instance is one client's Jamf Pro server in the fleet.
type Instance struct {
	Name     string   `json:"name"`
	Server   string   `json:"server"`
	ClientID string   `json:"client_id"`
	Tags     []string `json:"tags,omitempty"`
}

// Registry is the on-disk fleet file.
type Registry struct {
	Instances []Instance `json:"instances"`
}

var (
	slugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,62}$`) // instance names and tags
)

// DirEnv pins the fleet directory for child processes so a parent and its
// per-instance children always read the same registry.
const DirEnv = "JAMF_PRO_FLEET_DIR"

// BaseDir is the config directory the fleet file lives in. JAMF_PRO_FLEET_DIR
// pins it; --instance sets that before moving --home, so selecting an
// instance never moves the registry.
func BaseDir() (string, error) {
	if dir := strings.TrimSpace(os.Getenv(DirEnv)); dir != "" {
		if !filepath.IsAbs(dir) {
			return "", fmt.Errorf("%s must be an absolute path", DirEnv)
		}
		return filepath.Clean(dir), nil
	}
	return cliutil.ConfigDir()
}

// Path returns the fleet file location.
func Path() (string, error) {
	dir, err := BaseDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "fleet.json"), nil
}

// InstanceRoot holds every home ever used by one registered name.
func InstanceRoot(base, name string) string {
	return filepath.Join(base, "instance", name)
}

// Home is the isolated home for one instance identity: its config, token
// cache, local mirror and cache live here. The directory is derived from the
// server and client ID, so re-pointing a name at another server or API client
// lands in a fresh home and can never reuse a token minted for the old one.
// Fingerprint identifies a registry entry's credential binding: its name,
// server and client id. A fleet child refuses to run when
// the entry it was started for no longer has the parent's fingerprint.
func (p Instance) Fingerprint() string {
	sum := sha256.Sum256([]byte(p.Name + "\n" + strings.ToLower(p.Server) + "\n" + p.ClientID))
	return hex.EncodeToString(sum[:])
}

func (p Instance) Home(base string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(p.Server) + "\n" + p.ClientID))
	return filepath.Join(InstanceRoot(base, p.Name), hex.EncodeToString(sum[:8]))
}

// RemoveInstance deletes a registry entry and all of its local homes in one
// locked transaction, so a concurrent re-registration cannot interleave.
// It holds the instance's lock exclusively first, so it is refused while
// any command acts as the instance.
func RemoveInstance(name string) (bool, error) {
	found := false
	err := WithInstanceExclusive(name, func(string) error {
		return Update(func(reg *Registry) error {
			found = reg.Remove(name)
			return nil
		}, func(base string) error {
			if !found {
				return nil
			}
			return os.RemoveAll(InstanceRoot(base, name))
		})
	})
	return found, err
}

// PruneHomes removes homes left behind by an earlier identity of p.
// The caller holds the instance's lock exclusively (WithInstanceExclusive).
func PruneHomes(base string, p Instance) error {
	root := InstanceRoot(base, p.Name)
	keep := filepath.Base(p.Home(base))
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() && e.Name() != keep {
			if err := os.RemoveAll(filepath.Join(root, e.Name())); err != nil {
				return err
			}
		}
	}
	return nil
}

// Load reads the fleet file. A missing file is an empty fleet.
func Load() (*Registry, error) {
	path, err := Path()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Clean(path)) // #nosec G304 -- app-owned config path
	if errors.Is(err, os.ErrNotExist) {
		return &Registry{Instances: []Instance{}}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading fleet file %s: %w", path, err)
	}
	var reg Registry
	if err := json.Unmarshal(data, &reg); err != nil {
		return nil, fmt.Errorf("parsing fleet file %s: %w", path, err)
	}
	if reg.Instances == nil {
		reg.Instances = []Instance{}
	}
	if err := reg.Validate(); err != nil {
		return nil, fmt.Errorf("fleet file %s: %w", path, err)
	}
	return &reg, nil
}

// Validate checks every entry plus the cross-entry rule: names are unique (each
// name keys one JAMF_PRO_FLEET_SECRETS entry, so no two instances share a secret).
func (r *Registry) Validate() error {
	names := map[string]bool{}
	for _, p := range r.Instances {
		if err := p.Validate(); err != nil {
			return err
		}
		if names[p.Name] {
			return fmt.Errorf("instance %q is registered twice", p.Name)
		}
		names[p.Name] = true
	}
	return nil
}

// Update runs fn on the registry under an inter-process lock and saves the
// result, so concurrent add/remove calls cannot overwrite each other. Each
// after hook runs once the save succeeds, still under the lock.
func Update(fn func(*Registry) error, after ...func(base string) error) error {
	path, err := Path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return cliutil.WithFileLock(path, func() error {
		reg, err := Load()
		if err != nil {
			return err
		}
		if err := fn(reg); err != nil {
			return err
		}
		if err := reg.Validate(); err != nil {
			return err
		}
		if err := reg.Save(); err != nil {
			return err
		}
		base := filepath.Dir(path)
		for _, h := range after {
			if err := h(base); err != nil {
				return err
			}
		}
		return nil
	})
}

// Save writes the fleet file with private permissions.
func (r *Registry) Save() error {
	path, err := Path()
	if err != nil {
		return err
	}
	sort.Slice(r.Instances, func(i, j int) bool { return r.Instances[i].Name < r.Instances[j].Name })
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return cliutil.AtomicWritePrivateFile(path, append(data, '\n'), 0o600, 0o700)
}

// Get returns the instance with the given name.
func (r *Registry) Get(name string) (Instance, bool) {
	for _, p := range r.Instances {
		if p.Name == name {
			return p, true
		}
	}
	return Instance{}, false
}

// Names lists every registered instance name.
func (r *Registry) Names() []string {
	return NamesOf(r.Instances)
}

// Upsert adds or replaces an instance by name.
func (r *Registry) Upsert(p Instance) (replaced bool) {
	for i := range r.Instances {
		if r.Instances[i].Name == p.Name {
			r.Instances[i] = p
			return true
		}
	}
	r.Instances = append(r.Instances, p)
	return false
}

// Remove deletes an instance by name.
func (r *Registry) Remove(name string) bool {
	for i := range r.Instances {
		if r.Instances[i].Name == name {
			r.Instances = append(r.Instances[:i], r.Instances[i+1:]...)
			return true
		}
	}
	return false
}

// Select returns the instances matching names and tags. Empty filters select
// the whole fleet. Unknown names and tags no instance carries are errors so a
// typo never silently shrinks a fleet-wide report.
func (r *Registry) Select(names, tags []string) ([]Instance, error) {
	if len(names) == 0 && len(tags) == 0 {
		return append([]Instance(nil), r.Instances...), nil
	}
	if err := r.checkTags(tags); err != nil {
		return nil, err
	}
	var out []Instance
	seen := map[string]bool{}
	for _, n := range names {
		p, ok := r.Get(n)
		if !ok {
			return nil, fmt.Errorf("unknown instance %q (registered: %s)", n, strings.Join(r.Names(), ", "))
		}
		if !seen[p.Name] {
			seen[p.Name] = true
			out = append(out, p)
		}
	}
	for _, p := range r.Instances {
		if seen[p.Name] || !hasAnyTag(p, tags) {
			continue
		}
		seen[p.Name] = true
		out = append(out, p)
	}
	return out, nil
}

// checkTags rejects a tag that no registered instance carries, naming the
// tags in use the way an unknown --only name lists the registered names.
func (r *Registry) checkTags(tags []string) error {
	inUse := map[string]bool{}
	for _, p := range r.Instances {
		for _, t := range p.Tags {
			inUse[t] = true
		}
	}
	for _, t := range tags {
		if inUse[t] {
			continue
		}
		if len(inUse) == 0 {
			return fmt.Errorf("unknown tag %q (no registered instance has a tag; add one with instance add --tag)", t)
		}
		known := make([]string, 0, len(inUse))
		for k := range inUse {
			known = append(known, k)
		}
		sort.Strings(known)
		return fmt.Errorf("unknown tag %q (tags in use: %s)", t, strings.Join(known, ", "))
	}
	return nil
}

func hasAnyTag(p Instance, tags []string) bool {
	for _, want := range tags {
		for _, have := range p.Tags {
			if want == have {
				return true
			}
		}
	}
	return false
}

// Validate checks one entry. The server check is a trust boundary: the value
// becomes the host of every API and token request, so it must be a bare host
// or host:port with no scheme, credentials, path, or query.
func (p Instance) Validate() error {
	if !slugPattern.MatchString(p.Name) || strings.Contains(p.Name, "..") {
		return fmt.Errorf("invalid instance name %q: use lowercase letters, digits, '.', '_' or '-' (max 63)", p.Name)
	}
	if err := ValidateServer(p.Server); err != nil {
		return fmt.Errorf("instance %q: %w", p.Name, err)
	}
	if strings.TrimSpace(p.ClientID) == "" || strings.ContainsAny(p.ClientID, " \t\r\n") {
		return fmt.Errorf("instance %q: client_id must be the API client ID from Settings > System > API roles and clients", p.Name)
	}
	for _, t := range p.Tags {
		if !slugPattern.MatchString(t) {
			return fmt.Errorf("instance %q: invalid tag %q", p.Name, t)
		}
	}
	return nil
}

// ValidateServer accepts "host" or "host:port" only.
func ValidateServer(server string) error {
	if server == "" {
		return errors.New("server is required (e.g. acme.jamfcloud.com or jamf.example.com:8443)")
	}
	if strings.ContainsAny(server, "/\\@?#%[] \t\r\n") || strings.Contains(server, "://") {
		return errors.New("server must be a bare host or host:port, without scheme, path, or credentials") // the value may hold a password
	}
	u, err := url.Parse("https://" + server)
	if err != nil || u.Host != server || u.Hostname() == "" {
		return fmt.Errorf("server %q is not a valid host", server)
	}
	if port := u.Port(); port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return fmt.Errorf("server %q has an invalid port", server)
		}
	} else if strings.HasSuffix(server, ":") {
		return fmt.Errorf("server %q has an empty port", server)
	}
	host := u.Hostname()
	if net.ParseIP(host) != nil {
		return nil
	}
	for _, label := range strings.Split(host, ".") {
		if label == "" || len(label) > 63 || strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
			return fmt.Errorf("server %q has an invalid DNS label", server)
		}
		for _, c := range label {
			if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-') {
				return fmt.Errorf("server %q has an invalid character %q", server, c)
			}
		}
	}
	return nil
}
