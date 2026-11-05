// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/dnsfilter/handfixes.json
// under the "credentials-file-follows-config",
// "explicit-config-logout-never-falls-back" and
// "save-never-leaves-an-unverified-secret" entries.

package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dnsfilter-pp-cli/internal/cliutil"
	"dnsfilter-pp-cli/internal/cliutil/testenv"
)

// r17Home isolates the user directories and saves "default-key" in the
// default credentials file, the one a fallback would reach.
func r17Home(t *testing.T) (home, defaultPath string) {
	t.Helper()
	home = testenv.Isolate(t, cliutil.ConfigDir, cliutil.DataDir, cliutil.StateDir, cliutil.CacheDir)
	t.Setenv("DNSFILTER_API_KEY", "")
	t.Setenv("DNSFILTER_BASE_URL", "")
	defaultPath, err := cliutil.CredentialsFilePath()
	if err != nil {
		t.Fatal(err)
	}
	if err := cliutil.SaveCredentialsAt(defaultPath, &cliutil.Credentials{DnsfilterApiKey: "default-key"}); err != nil {
		t.Fatal(err)
	}
	return home, defaultPath
}

func r17TenantConfig(t *testing.T, dir string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte("base_url = \"https://api.dnsfilter.com\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func r17Key(t *testing.T, configPath string) string {
	t.Helper()
	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load(%s): %v", configPath, err)
	}
	return cfg.DnsfilterApiKey
}

// A symlinked explicit config keeps one credentials location for load, save
// and logout: set-token writes through the link instead of replacing it, so
// the next load still finds the saved key and never falls back to the
// default file; logout clears it there.
func TestSymlinkedConfigCredentialsStayPutAfterSave(t *testing.T) {
	home, _ := r17Home(t)
	target := r17TenantConfig(t, filepath.Join(home, "real-tenant"))
	linkDir := filepath.Join(home, "linked-tenant")
	if err := os.MkdirAll(linkDir, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(linkDir, "config.toml")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable here: %v", err)
	}
	before, err := cliutil.CredentialsFilePathForConfig(link)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(link)
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.SaveCredential("tenant-key"); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Lstat(link); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("set-token replaced the config symlink with a regular file (%v)", err)
	}
	after, err := cliutil.CredentialsFilePathForConfig(link)
	if err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Fatalf("the credentials location moved after a save: %s -> %s", before, after)
	}
	if got := r17Key(t, link); got != "tenant-key" {
		t.Fatalf("after set-token through a symlinked config the key is %q, want tenant-key (fell back to the default file?)", got)
	}
	cfg, err = Load(link)
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.ClearTokens(); err != nil {
		t.Fatal(err)
	}
	if got := r17Key(t, link); got != "" {
		t.Fatalf("after logout the symlinked config still authenticates with %q", got)
	}
	if got := r17Key(t, ""); got != "default-key" {
		t.Fatalf("logout through a symlinked config touched the default credentials: %q", got)
	}
}

// After `--config X auth logout` the next `--config X` run is logged out: it
// does not fall back to the default credentials file (another tenant), and
// the default file is untouched. set-token under X works again afterwards.
func TestExplicitConfigLogoutNeverFallsBack(t *testing.T) {
	home, _ := r17Home(t)
	tenant := r17TenantConfig(t, filepath.Join(home, "tenant"))
	sibling, err := cliutil.CredentialsFilePathForConfig(tenant)
	if err != nil {
		t.Fatal(err)
	}
	if err := cliutil.SaveCredentialsAt(sibling, &cliutil.Credentials{DnsfilterApiKey: "tenant-key"}); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(tenant)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DnsfilterApiKey != "tenant-key" || cfg.CredentialsLoadedFrom() != sibling {
		t.Fatalf("loaded %q from %q, want tenant-key from %s", cfg.DnsfilterApiKey, cfg.CredentialsLoadedFrom(), sibling)
	}
	if scope := cfg.CredentialsFilesInScope(); len(scope) != 1 || scope[0] != sibling {
		t.Fatalf("credentials files in scope = %v, want only the sibling %s", scope, sibling)
	}
	if err := cfg.ClearTokens(); err != nil {
		t.Fatal(err)
	}
	after, err := Load(tenant)
	if err != nil {
		t.Fatal(err)
	}
	if after.DnsfilterApiKey != "" || after.CredentialConfigured() {
		t.Fatalf("after logout --config X authenticates with %q from %q (fell back to the default file)", after.DnsfilterApiKey, after.CredentialsLoadedFrom())
	}
	if scope := after.CredentialsFilesInScope(); len(scope) != 1 || scope[0] != sibling {
		t.Fatalf("after logout credentials files in scope = %v, want only the sibling", scope)
	}
	if got := r17Key(t, ""); got != "default-key" {
		t.Fatalf("logout under an explicit config touched the default credentials: %q", got)
	}
	if err := after.SaveCredential("tenant-key-2"); err != nil {
		t.Fatal(err)
	}
	if got := r17Key(t, tenant); got != "tenant-key-2" {
		t.Fatalf("set-token after logout: key %q, want tenant-key-2", got)
	}
}

// failSavedCredsCheck makes the save-time private-file guard refuse on the
// calls listed (1-based) and pass on every other call.
func failSavedCredsCheck(t *testing.T, failOn ...int) {
	t.Helper()
	orig := cliutil.SavedCredentialsPermsCheck
	t.Cleanup(func() { cliutil.SavedCredentialsPermsCheck = orig })
	calls := 0
	cliutil.SavedCredentialsPermsCheck = func(path string) error {
		calls++
		for _, n := range failOn {
			if calls == n {
				return errors.New("token file DACL grants access to a broad group (BU); restrict it")
			}
		}
		return orig(path)
	}
}

func r17DirHolds(t *testing.T, dir, needle string) string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		data, _ := os.ReadFile(filepath.Join(dir, e.Name()))
		if strings.Contains(string(data), needle) {
			return e.Name()
		}
	}
	return ""
}

// A set-token whose file fails the private-file guard never leaves the new
// key on disk, and the previous credentials stay in effect: untouched when
// the temporary file is refused, restored when the published file is.
func TestSaveCredentialRestoresPreviousOnPermissionRefusal(t *testing.T) {
	home, _ := r17Home(t)
	tenant := r17TenantConfig(t, filepath.Join(home, "tenant"))
	sibling, err := cliutil.CredentialsFilePathForConfig(tenant)
	if err != nil {
		t.Fatal(err)
	}
	if err := cliutil.SaveCredentialsAt(sibling, &cliutil.Credentials{DnsfilterApiKey: "old-key"}); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		failOn []int
	}{
		{"temporary file refused", []int{1}},
		{"published file refused", []int{2}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			failSavedCredsCheck(t, tc.failOn...)
			cfg, err := Load(tenant)
			if err != nil {
				t.Fatal(err)
			}
			err = cfg.SaveCredential("new-key-r17")
			var permErr *cliutil.CredentialsPermissionError
			if !errors.As(err, &permErr) {
				t.Fatalf("SaveCredential() = %v, want a credentials permission refusal", err)
			}
			if name := r17DirHolds(t, filepath.Dir(sibling), "new-key-r17"); name != "" {
				t.Fatalf("the refused key is on disk in %s", name)
			}
			cliutil.SavedCredentialsPermsCheck = func(string) error { return nil }
			if got := r17Key(t, tenant); got != "old-key" {
				t.Fatalf("after a refused set-token the config authenticates with %q, want the previous old-key", got)
			}
		})
	}
}

// Under agentcookie the config file itself carries the key, so it gets the
// same guard: a refused set-token publishes nothing and the previous config
// stays as it was.
func TestAgentcookieConfigSaveIsVerified(t *testing.T) {
	r17Home(t)
	dir, err := cliutil.ConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".agentcookie-managed"), []byte("managed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(dir, "config.toml")
	cfg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.SaveCredential("old-agentcookie-key"); err != nil {
		t.Fatal(err)
	}
	failSavedCredsCheck(t, 1)
	cfg, err = Load("")
	if err != nil {
		t.Fatal(err)
	}
	var permErr *cliutil.CredentialsPermissionError
	if err := cfg.SaveCredential("new-agentcookie-key-r17"); !errors.As(err, &permErr) {
		t.Fatalf("SaveCredential() under agentcookie = %v, want a permission refusal", err)
	}
	if name := r17DirHolds(t, dir, "new-agentcookie-key-r17"); name != "" {
		t.Fatalf("the refused key is on disk in %s", name)
	}
	if data, err := os.ReadFile(configPath); err != nil || !strings.Contains(string(data), "old-agentcookie-key") {
		t.Fatalf("the previous agentcookie config changed: %q (%v)", data, err)
	}
}
