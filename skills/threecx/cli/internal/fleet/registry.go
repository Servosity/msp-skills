// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: the local registry of 3CX phone systems an MSP manages.

// Package fleet keeps the list of 3CX phone systems (one per client) and
// runs this CLI against each of them with that system's own credential.
// Secrets are never written to disk: each entry names the environment
// variable that holds its client secret.
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

	"threecx-pp-cli/internal/cliutil"
)

// PBX is one 3CX phone system in the fleet.
type PBX struct {
	Name      string   `json:"name"`
	FQDN      string   `json:"fqdn"`
	ClientID  string   `json:"client_id"`
	EnvPrefix string   `json:"env_prefix"`
	Tags      []string `json:"tags,omitempty"`
	// SecretVar is EnvPrefix + SecretSuffix, the only variable this PBX's
	// secret is read from. The registry derives it (Validate, Upsert) and
	// never reads it from disk; a PBX built without the registry has none
	// and gets no secret. Scrub (runner.go, bound to the base policy by its
	// file hash) reads it by this name.
	SecretVar string `json:"-"`
}

// Registry is the on-disk fleet file.
type Registry struct {
	PBXes []PBX `json:"pbxes"`
}

var (
	slugPattern      = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,62}$`) // PBX names and tags
	envPrefixPattern = regexp.MustCompile(`^[A-Z]([A-Z0-9_]{0,99}[A-Z0-9])?$`)
	nonEnvChars      = regexp.MustCompile(`[^A-Z0-9]+`)
)

// SecretSuffix ends the name of every PBX's client-secret variable: a PBX's
// secret is read only as os.Getenv(p.EnvPrefix + "_CLIENT_SECRET"), so the name
// is the connector's own shape and never free-form operator data.
const SecretSuffix = "_CLIENT_SECRET"

// DefaultEnvPrefix derives THREECX_<NAME> from a PBX name; its secret is then
// THREECX_<NAME>_CLIENT_SECRET.
func DefaultEnvPrefix(name string) string {
	return "THREECX_" + strings.Trim(nonEnvChars.ReplaceAllString(strings.ToUpper(name), "_"), "_")
}

// SecretVarName is the variable holding p's client secret.
func (p PBX) SecretVarName() string { return p.EnvPrefix + SecretSuffix }

// reservedEnvPrefix rejects a prefix whose secret variable the CLI or its
// environment already uses: THREECX names the connector's own
// THREECX_CLIENT_SECRET, and PRINTING_PRESS_ is the platform's.
func reservedEnvPrefix(prefix string) bool {
	return prefix == "THREECX" || strings.HasPrefix(prefix, "PRINTING_PRESS")
}

// DirEnv pins the fleet directory for child processes so a parent and its
// per-PBX children always read the same registry.
const DirEnv = "THREECX_FLEET_DIR"

// BaseDir is the config directory the fleet file lives in. THREECX_FLEET_DIR
// pins it; --pbx sets that before moving --home, so selecting a PBX never
// moves the registry.
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

// PBXRoot holds every home ever used by one registered name.
func PBXRoot(base, name string) string {
	return filepath.Join(base, "pbx", name)
}

// Home is the isolated home for one PBX identity: its config, token cache,
// local mirror and cache live here. The directory is derived from the FQDN
// and client ID, so re-pointing a name at another host or client lands in a
// fresh home and can never reuse a token minted for the old one.
func (p PBX) Home(base string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(p.FQDN) + "\n" + p.ClientID))
	return filepath.Join(PBXRoot(base, p.Name), hex.EncodeToString(sum[:8]))
}

// RemovePBX deletes a registry entry and all of its local homes in one
// locked transaction, so a concurrent re-registration cannot interleave.
func RemovePBX(name string) (bool, error) {
	found := false
	err := Update(func(reg *Registry) error {
		found = reg.Remove(name)
		return nil
	}, func(base string) error {
		if !found || !slugPattern.MatchString(name) {
			return nil
		}
		return os.RemoveAll(PBXRoot(base, name))
	})
	return found, err
}

// PruneHomes removes homes left behind by an earlier identity of p.
func PruneHomes(base string, p PBX) error {
	root := PBXRoot(base, p.Name)
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
		return &Registry{PBXes: []PBX{}}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading fleet file %s: %w", path, err)
	}
	var reg Registry
	if err := json.Unmarshal(data, &reg); err != nil {
		return nil, fmt.Errorf("parsing fleet file %s: %w", path, err)
	}
	if reg.PBXes == nil {
		reg.PBXes = []PBX{}
	}
	if err := reg.Validate(); err != nil {
		return nil, fmt.Errorf("fleet file %s: %w", path, err)
	}
	return &reg, nil
}

// Validate checks every entry plus the cross-entry rules: names are unique
// and no two PBXes share a secret env var (which would send one client's
// API key to another client's phone system).
func (r *Registry) Validate() error {
	names := map[string]bool{}
	secrets := map[string]string{}
	for i := range r.PBXes {
		r.PBXes[i].SecretVar = r.PBXes[i].SecretVarName()
		p := r.PBXes[i]
		if err := p.Validate(); err != nil {
			return err
		}
		if names[p.Name] {
			return fmt.Errorf("PBX %q is registered twice", p.Name)
		}
		names[p.Name] = true
		if other, dup := secrets[p.EnvPrefix]; dup {
			return fmt.Errorf("PBX %q and %q both use env prefix %s; give each phone system its own prefix", other, p.Name, p.EnvPrefix)
		}
		secrets[p.EnvPrefix] = p.Name
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
	sort.Slice(r.PBXes, func(i, j int) bool { return r.PBXes[i].Name < r.PBXes[j].Name })
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return cliutil.AtomicWritePrivateFile(path, append(data, '\n'), 0o600, 0o700)
}

// Get returns the PBX with the given name.
func (r *Registry) Get(name string) (PBX, bool) {
	for _, p := range r.PBXes {
		if p.Name == name {
			return p, true
		}
	}
	return PBX{}, false
}

// Names lists every registered PBX name.
func (r *Registry) Names() []string {
	return NamesOf(r.PBXes)
}

// Upsert adds or replaces a PBX by name.
func (r *Registry) Upsert(p PBX) (replaced bool) {
	p.SecretVar = p.SecretVarName()
	for i := range r.PBXes {
		if r.PBXes[i].Name == p.Name {
			r.PBXes[i] = p
			return true
		}
	}
	r.PBXes = append(r.PBXes, p)
	return false
}

// Remove deletes a PBX by name.
func (r *Registry) Remove(name string) bool {
	for i := range r.PBXes {
		if r.PBXes[i].Name == name {
			r.PBXes = append(r.PBXes[:i], r.PBXes[i+1:]...)
			return true
		}
	}
	return false
}

// Select returns the PBXes matching names and tags. Empty filters select
// the whole fleet. Unknown names and tags no PBX carries are errors so a
// typo never silently shrinks a fleet-wide report.
func (r *Registry) Select(names, tags []string) ([]PBX, error) {
	if len(names) == 0 && len(tags) == 0 {
		return append([]PBX(nil), r.PBXes...), nil
	}
	if err := r.checkTags(tags); err != nil {
		return nil, err
	}
	var out []PBX
	seen := map[string]bool{}
	for _, n := range names {
		p, ok := r.Get(n)
		if !ok {
			return nil, fmt.Errorf("unknown PBX %q (registered: %s)", n, strings.Join(r.Names(), ", "))
		}
		if !seen[p.Name] {
			seen[p.Name] = true
			out = append(out, p)
		}
	}
	for _, p := range r.PBXes {
		if seen[p.Name] || !hasAnyTag(p, tags) {
			continue
		}
		seen[p.Name] = true
		out = append(out, p)
	}
	return out, nil
}

// checkTags rejects a tag that no registered PBX carries, naming the tags
// in use the way an unknown --only name lists the registered names.
func (r *Registry) checkTags(tags []string) error {
	inUse := map[string]bool{}
	for _, p := range r.PBXes {
		for _, t := range p.Tags {
			inUse[t] = true
		}
	}
	for _, t := range tags {
		if inUse[t] {
			continue
		}
		if len(inUse) == 0 {
			return fmt.Errorf("unknown tag %q (no registered PBX has a tag; add one with fleet add --tag)", t)
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

func hasAnyTag(p PBX, tags []string) bool {
	for _, want := range tags {
		for _, have := range p.Tags {
			if want == have {
				return true
			}
		}
	}
	return false
}

// Validate checks one entry. The FQDN check is a trust boundary: the value
// becomes the host of every API and token request, so it must be a bare
// host or host:port with no scheme, credentials, path, or query.
func (p PBX) Validate() error {
	if !slugPattern.MatchString(p.Name) || strings.Contains(p.Name, "..") {
		return fmt.Errorf("invalid PBX name %q: use lowercase letters, digits, '.', '_' or '-' (max 63)", p.Name)
	}
	if err := ValidateFQDN(p.FQDN); err != nil {
		return fmt.Errorf("PBX %q: %w", p.Name, err)
	}
	if strings.TrimSpace(p.ClientID) == "" || strings.ContainsAny(p.ClientID, " \t\r\n") {
		return fmt.Errorf("PBX %q: client_id must be the service principal's client ID (its DN number)", p.Name)
	}
	if !envPrefixPattern.MatchString(p.EnvPrefix) {
		return fmt.Errorf("PBX %q: env_prefix %q must be upper-case letters, digits and '_', like THREECX_ACME (its secret is then THREECX_ACME_CLIENT_SECRET)", p.Name, p.EnvPrefix)
	}
	if reservedEnvPrefix(p.EnvPrefix) {
		return fmt.Errorf("PBX %q: env_prefix %s is reserved; use a per-PBX prefix like %s", p.Name, p.EnvPrefix, DefaultEnvPrefix(p.Name))
	}
	for _, t := range p.Tags {
		if !slugPattern.MatchString(t) {
			return fmt.Errorf("PBX %q: invalid tag %q", p.Name, t)
		}
	}
	return nil
}

// ValidateFQDN accepts "host" or "host:port" only.
func ValidateFQDN(fqdn string) error {
	if fqdn == "" {
		return errors.New("fqdn is required (e.g. acme.3cx.us or pbx.example.com:5001)")
	}
	if strings.ContainsAny(fqdn, "/\\@?#%[] \t\r\n") || strings.Contains(fqdn, "://") {
		return fmt.Errorf("fqdn %q must be a bare host or host:port, without scheme, path, or credentials", fqdn)
	}
	u, err := url.Parse("https://" + fqdn)
	if err != nil || u.Host != fqdn || u.Hostname() == "" {
		return fmt.Errorf("fqdn %q is not a valid host", fqdn)
	}
	if port := u.Port(); port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return fmt.Errorf("fqdn %q has an invalid port", fqdn)
		}
	} else if strings.HasSuffix(fqdn, ":") {
		return fmt.Errorf("fqdn %q has an empty port", fqdn)
	}
	host := u.Hostname()
	if net.ParseIP(host) != nil {
		return nil
	}
	for _, label := range strings.Split(host, ".") {
		if label == "" || len(label) > 63 || strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
			return fmt.Errorf("fqdn %q has an invalid DNS label", fqdn)
		}
		for _, c := range label {
			if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-') {
				return fmt.Errorf("fqdn %q has an invalid character %q", fqdn, c)
			}
		}
	}
	return nil
}

// SecretEnvNames is every registered PBX's client-secret variable.
func SecretEnvNames() []string {
	reg, err := Load()
	if err != nil {
		return nil
	}
	out := make([]string, 0, len(reg.PBXes))
	for _, p := range reg.PBXes {
		out = append(out, p.SecretVarName())
	}
	return out
}
