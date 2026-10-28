// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: the local registry of PRTG core servers an MSP manages.

// Package fleet keeps the list of PRTG core servers (usually one per client)
// and runs this CLI against each of them with that core's own API key.
// Secrets are never written to disk: every core's API key comes from the one
// PRTG_FLEET_KEYS variable (see ParseKeys).
package fleet

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	"prtg-pp-cli/internal/cliutil"
)

// Core is one PRTG core server in the fleet.
type Core struct {
	Name   string   `json:"name"`
	Server string   `json:"server"`
	Tags   []string `json:"tags,omitempty"`
	// InsecureTLS skips certificate verification for this core only (a
	// self-signed certificate), so one such core does not weaken the others.
	InsecureTLS bool `json:"insecure_tls,omitempty"`
}

// Registry is the on-disk fleet file.
type Registry struct {
	Cores []Core `json:"cores"`
}

var slugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,62}$`) // core names and tags

// validName reports whether name is a core name: a slug without "..", so it
// is safe as a directory name under the fleet directory.
func validName(name string) bool {
	return slugPattern.MatchString(name) && !strings.Contains(name, "..")
}

// DirEnv pins the fleet directory for child processes so a parent and its
// per-core children always read the same registry. It is parent-to-child
// plumbing in the internal PP_ family; an operator who sets it moves the
// fleet directory the same way.
const DirEnv = "PP_PRTG_FLEET_DIR"

// BaseDir is the config directory the fleet file lives in. DirEnv pins it;
// --core sets that before moving --home, so selecting a core never moves the
// registry.
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

// CoreRoot holds every home ever used by one registered name.
func CoreRoot(base, name string) string {
	return filepath.Join(base, "cores", name)
}

// Home is the isolated home for one core identity: its config, cache and
// local mirror live here. The directory is derived from the server and the
// core name, so re-pointing a name at another core lands in a fresh home,
// two cores never share one, and one client's mirror never mixes with
// another's.
func (c Core) Home(base string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(c.Server) + "\n" + c.Name))
	return filepath.Join(CoreRoot(base, c.Name), hex.EncodeToString(sum[:8]))
}

// BaseURL is the API v2 root of the core.
func (c Core) BaseURL() string {
	return "https://" + c.Server + "/api/v2"
}

// RemoveCore deletes a registry entry and all of its local homes in one
// locked transaction, so a concurrent re-registration cannot interleave.
func RemoveCore(name string) (bool, error) {
	found := false
	err := Update(func(reg *Registry) error {
		found = reg.Remove(name)
		return nil
	}, func(base string) error {
		if !found || !validName(name) {
			return nil
		}
		return os.RemoveAll(CoreRoot(base, name))
	})
	return found, err
}

// PruneHomes removes homes left behind by an earlier identity of c.
func PruneHomes(base string, c Core) error {
	root := CoreRoot(base, c.Name)
	keep := filepath.Base(c.Home(base))
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

// Load reads the fleet file. A missing file is an empty fleet. The per-core
// key variable name an earlier build recorded is ignored (keys now come from
// PRTG_FLEET_KEYS) and dropped on the next save.
func Load() (*Registry, error) {
	path, err := Path()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Clean(path)) // #nosec G304 -- app-owned config path
	if errors.Is(err, os.ErrNotExist) {
		return &Registry{Cores: []Core{}}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading fleet file %s: %w", path, err)
	}
	var reg Registry
	if err := json.Unmarshal(data, &reg); err != nil {
		return nil, fmt.Errorf("parsing fleet file %s: %w", path, err)
	}
	if reg.Cores == nil {
		reg.Cores = []Core{}
	}
	if err := reg.Validate(); err != nil {
		return nil, fmt.Errorf("fleet file %s: %w", path, err)
	}
	return &reg, nil
}

// Validate checks every entry plus the cross-entry rule that names are
// unique: a core's key is looked up by its name in PRTG_FLEET_KEYS, so a
// shared name would send one client's API key to another client's core.
func (r *Registry) Validate() error {
	names := map[string]bool{}
	for _, c := range r.Cores {
		if err := c.Validate(); err != nil {
			return err
		}
		if names[c.Name] {
			return fmt.Errorf("core %q is registered twice", c.Name)
		}
		names[c.Name] = true
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
	sort.Slice(r.Cores, func(i, j int) bool { return r.Cores[i].Name < r.Cores[j].Name })
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return cliutil.AtomicWritePrivateFile(path, append(data, '\n'), 0o600, 0o700)
}

// Get returns the core with the given name.
func (r *Registry) Get(name string) (Core, bool) {
	for _, c := range r.Cores {
		if c.Name == name {
			return c, true
		}
	}
	return Core{}, false
}

// Names lists every registered core name.
func (r *Registry) Names() []string {
	out := make([]string, 0, len(r.Cores))
	for _, c := range r.Cores {
		out = append(out, c.Name)
	}
	sort.Strings(out)
	return out
}

// Upsert adds or replaces a core by name.
func (r *Registry) Upsert(c Core) (replaced bool) {
	for i := range r.Cores {
		if r.Cores[i].Name == c.Name {
			r.Cores[i] = c
			return true
		}
	}
	r.Cores = append(r.Cores, c)
	return false
}

// Remove deletes a core by name.
func (r *Registry) Remove(name string) bool {
	for i := range r.Cores {
		if r.Cores[i].Name == name {
			r.Cores = append(r.Cores[:i], r.Cores[i+1:]...)
			return true
		}
	}
	return false
}

// Select returns the cores matching names and tags. Empty filters select the
// whole fleet; names and tags both narrow (a core must be named AND carry one
// of the tags when both are given). Unknown names and tags no core carries are
// errors so a typo never silently shrinks a fleet-wide report.
func (r *Registry) Select(names, tags []string) ([]Core, error) {
	if len(names) == 0 && len(tags) == 0 {
		return append([]Core(nil), r.Cores...), nil
	}
	if err := r.checkTags(tags); err != nil {
		return nil, err
	}
	named := map[string]bool{}
	for _, n := range names {
		c, ok := r.Get(n)
		if !ok {
			return nil, fmt.Errorf("unknown core %q (registered: %s)", n, strings.Join(r.Names(), ", "))
		}
		if len(tags) > 0 && !hasAnyTag(c, tags) {
			return nil, fmt.Errorf("core %q carries none of the tags %s; drop --tag or name another core", n, strings.Join(tags, ", "))
		}
		named[n] = true
	}
	var out []Core
	for _, c := range r.Cores {
		if len(names) > 0 && !named[c.Name] {
			continue
		}
		if len(tags) > 0 && !hasAnyTag(c, tags) {
			continue
		}
		out = append(out, c)
	}
	return out, nil
}

// checkTags rejects a tag that no registered core carries, naming the tags in
// use the way an unknown --only name lists the registered names.
func (r *Registry) checkTags(tags []string) error {
	inUse := map[string]bool{}
	for _, c := range r.Cores {
		for _, t := range c.Tags {
			inUse[t] = true
		}
	}
	for _, t := range tags {
		if inUse[t] {
			continue
		}
		if len(inUse) == 0 {
			return fmt.Errorf("unknown tag %q (no registered core has a tag; add one with core add --tag)", t)
		}
		return fmt.Errorf("unknown tag %q (tags in use: %s)", t, strings.Join(slices.Sorted(maps.Keys(inUse)), ", "))
	}
	return nil
}

func hasAnyTag(c Core, tags []string) bool {
	return slices.ContainsFunc(tags, func(t string) bool { return slices.Contains(c.Tags, t) })
}

// Validate checks one entry. The server check is a trust boundary: the value
// becomes the host of every API request, so it must be a bare host or
// host:port with no scheme, credentials, path, or query.
func (c Core) Validate() error {
	if !validName(c.Name) {
		return fmt.Errorf("invalid core name %q: use lowercase letters, digits, '.', '_' or '-' (max 63)", c.Name)
	}
	if err := ValidateServer(c.Server); err != nil {
		return fmt.Errorf("core %q: %w", c.Name, err)
	}
	for _, t := range c.Tags {
		if !slugPattern.MatchString(t) {
			return fmt.Errorf("core %q: invalid tag %q", c.Name, t)
		}
	}
	return nil
}

// ValidateServer accepts "host" or "host:port" only.
func ValidateServer(server string) error {
	if server == "" {
		return errors.New("server is required (e.g. prtg.acme.com or prtg.acme.com:1616)")
	}
	if strings.ContainsAny(server, "/\\@?#%[] \t\r\n") || strings.Contains(server, "://") {
		return errors.New("server must be a bare host or host:port, without scheme, path, or credentials")
	}
	u, err := url.Parse("https://" + server)
	if err != nil || u.Host != server || u.Hostname() == "" {
		return errors.New("server is not a valid host")
	}
	if port := u.Port(); port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return errors.New("server has an invalid port")
		}
	} else if strings.HasSuffix(server, ":") {
		return errors.New("server has an empty port")
	}
	host := u.Hostname()
	if net.ParseIP(host) != nil {
		return nil
	}
	for _, label := range strings.Split(host, ".") {
		if label == "" || len(label) > 63 || strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
			return errors.New("server has an invalid DNS label")
		}
		for _, ch := range label {
			if !(ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || ch == '-') {
				return fmt.Errorf("server has an invalid character %q", ch)
			}
		}
	}
	return nil
}
