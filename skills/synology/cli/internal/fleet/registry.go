// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// HAND-AUTHORED.

// Package fleet holds the MSP fleet layer: a registry of client NAS units (one
// credential per NAS, referenced by env-var prefix, never stored), a SQLite
// mirror of Active Backup for Business job status and storage health keyed by
// NAS, and the rollup queries the fleet/abb novel commands print.
package fleet

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"synology-pp-cli/internal/cliutil"
	"synology-pp-cli/internal/dsm"
)

// NAS is one registered client NAS. Secrets are never stored: the account,
// password, OTP and trusted-device token come from <EnvPrefix>_ACCOUNT,
// <EnvPrefix>_PASSWORD, <EnvPrefix>_OTP_CODE and <EnvPrefix>_DEVICE_ID.
type NAS struct {
	Name           string `json:"name"`
	URL            string `json:"url"`
	Client         string `json:"client,omitempty"`
	EnvPrefix      string `json:"env_prefix"`
	TLSFingerprint string `json:"tls_fingerprint,omitempty"`
	Insecure       bool   `json:"insecure,omitempty"`

	// Override supplies credentials directly (the implicit "default" NAS built
	// from the CLI config); never serialized.
	Override *dsm.Credentials `json:"-"`
}

// Registry is the on-disk list of NAS units.
type Registry struct {
	NAS []NAS `json:"nas"`
}

var nameRE = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,62}$`)

// ValidName reports whether name is usable as a fleet key.
func ValidName(name string) bool { return nameRE.MatchString(name) }

// DefaultEnvPrefix derives SYNOLOGY_<NAME> from a fleet name.
func DefaultEnvPrefix(name string) string {
	up := strings.ToUpper(regexp.MustCompile(`[^A-Za-z0-9]+`).ReplaceAllString(name, "_"))
	return "SYNOLOGY_" + strings.Trim(up, "_")
}

// RegistryPath is <config dir>/fleet.json.
func RegistryPath() (string, error) {
	dir, err := cliutil.ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "fleet.json"), nil
}

// LoadRegistry reads the registry; a missing file is an empty registry.
func LoadRegistry(path string) (*Registry, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- path is this CLI's own fleet registry (RegistryPath under the config dir; tests pass a temp path), never request input
	if errors.Is(err, os.ErrNotExist) {
		return &Registry{}, nil
	}
	if err != nil {
		return nil, err
	}
	var r Registry
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	return &r, nil
}

// Save writes the registry with owner-only permissions, atomically (temp file
// and rename). Writers go through UpdateRegistry so none is lost.
func (r *Registry) Save(path string) error {
	sort.Slice(r.NAS, func(i, j int) bool { return r.NAS[i].Name < r.NAS[j].Name })
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".fleet-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name()) // no-op after the rename
	if _, err := f.Write(append(data, '\n')); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path) // CreateTemp files are already 0600
}

// UpdateRegistry reloads the registry under a cross-process lock, applies fn
// and saves it, so two concurrent `fleet add`s never drop each other's NAS.
func UpdateRegistry(path string, fn func(*Registry) error) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	// An OS advisory lock (flock / LockFileEx) on path+".lock": released by the
	// kernel if the holder dies, so there is no stale lock to break.
	return cliutil.WithFileLock(path, func() error {
		r, err := LoadRegistry(path)
		if err != nil {
			return err
		}
		if err := fn(r); err != nil {
			return err
		}
		return r.Save(path)
	})
}

// Get returns the named NAS.
func (r *Registry) Get(name string) (NAS, bool) {
	for _, n := range r.NAS {
		if n.Name == name {
			return n, true
		}
	}
	return NAS{}, false
}

// Put adds or replaces a NAS by name.
func (r *Registry) Put(n NAS) {
	for i := range r.NAS {
		if r.NAS[i].Name == n.Name {
			r.NAS[i] = n
			return
		}
	}
	r.NAS = append(r.NAS, n)
}

// Remove deletes a NAS by name and reports whether it existed.
func (r *Registry) Remove(name string) bool {
	for i := range r.NAS {
		if r.NAS[i].Name == name {
			r.NAS = append(r.NAS[:i], r.NAS[i+1:]...)
			return true
		}
	}
	return false
}

// Credentials reads this NAS's login from its env-var prefix.
func (n NAS) Credentials() dsm.Credentials {
	c := dsm.Credentials{
		Account:  cliutil.EnvOverride(n.EnvPrefix + "_ACCOUNT"),
		Password: cliutil.EnvOverride(n.EnvPrefix + "_PASSWORD"),
		OTPCode:  cliutil.EnvOverride(n.EnvPrefix + "_OTP_CODE"),
		DeviceID: cliutil.EnvOverride(n.EnvPrefix + "_DEVICE_ID"),
	}
	if n.Override != nil {
		c = *n.Override
	}
	if c.DeviceID == "" {
		// A trusted-device token cached by an earlier two-factor login to this
		// host (fleet sync/add, or auth login) spares the one-time code.
		if dir, err := cliutil.DataDir(); err == nil {
			c.DeviceID = dsm.CachedDeviceID(dir, n.URL, c.Account)
		}
	}
	return c
}

// EnvStatus names this NAS's credential env vars and whether each is set.
// Values are never returned.
func (n NAS) EnvStatus() map[string]bool {
	out := map[string]bool{}
	for _, suffix := range []string{"_ACCOUNT", "_PASSWORD", "_DEVICE_ID"} {
		out[n.EnvPrefix+suffix] = cliutil.EnvOverride(n.EnvPrefix+suffix) != ""
	}
	return out
}

// OriginURL validates a NAS address and returns its scheme://host origin. It
// rejects user info (a password in the URL would be stored and printed) and
// any path, query or fragment.
func OriginURL(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if raw == "" || err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return "", errors.New("the NAS address must look like https://<nas>:5001")
	}
	if u.User != nil {
		return "", errors.New("the NAS address must not carry a user or password; credentials come from <prefix>_ACCOUNT and <prefix>_PASSWORD")
	}
	if (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
		return "", errors.New("the NAS address must be scheme://host:port only")
	}
	return u.Scheme + "://" + u.Host, nil
}

// HTTPClient builds an HTTP client honoring this NAS's TLS pin or skip.
func (n NAS) HTTPClient(timeout time.Duration) (*http.Client, error) {
	if strings.TrimSpace(n.TLSFingerprint) != "" && !strings.HasPrefix(n.URL, "https://") {
		return nil, fmt.Errorf("NAS %s: a certificate pin needs an https:// URL; refusing to send the password over plain HTTP", n.Name)
	}
	tr := http.DefaultTransport.(*http.Transport).Clone()
	cfg := &tls.Config{MinVersion: tls.VersionTLS12}
	if err := dsm.ApplyTLS(cfg, n.TLSFingerprint, n.Insecure); err != nil {
		return nil, fmt.Errorf("NAS %s: %w", n.Name, err)
	}
	tr.TLSClientConfig = cfg
	// No DSM request follows a redirect (dsm.RefuseRedirect), same origin included.
	return &http.Client{Timeout: timeout, Transport: tr, CheckRedirect: dsm.RefuseRedirect}, nil
}
