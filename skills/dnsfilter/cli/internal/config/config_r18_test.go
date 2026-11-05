// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/dnsfilter/handfixes.json
// under the "env-api-key-wins" and "credentials-file-follows-config"
// entries.

package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"dnsfilter-pp-cli/internal/cliutil"
	"dnsfilter-pp-cli/internal/cliutil/testenv"
)

// init makes every credentials write in this package's tests refuse a path
// outside the home testenv.Isolate set up, the way the Windows write-time
// guard refuses a file under a TEMP whose inherited ACL grants another
// principal access (only the Isolate sandbox has that ACL stripped). A
// fixture created under a separate t.TempDir() therefore fails on every OS,
// not only on a Windows runner with a broad TEMP.
func init() {
	orig := cliutil.SavedCredentialsPermsCheck
	cliutil.SavedCredentialsPermsCheck = func(path string) error {
		if home, err := os.UserHomeDir(); err == nil && !r18Within(home, path) {
			return fmt.Errorf("credential fixture %s is outside the testenv.Isolate sandbox %s (a broad inherited TEMP ACL refuses it on Windows)", path, home)
		}
		return orig(path)
	}
}

func r18Within(root, path string) bool {
	resolve := func(p string) string {
		if r, err := filepath.EvalSymlinks(p); err == nil {
			return r
		}
		return filepath.Clean(p)
	}
	rel, err := filepath.Rel(resolve(root), resolve(filepath.Dir(path)))
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// DNSFILTER_API_KEY wins over a stored auth_header (in the config file or
// the credentials file): the key AuthHeader returns, the one every request
// sends, is the one AuthSource names.
func TestEnvironmentKeyWinsOverStoredAuthHeader(t *testing.T) {
	testenv.Isolate(t, cliutil.ConfigDir, cliutil.DataDir, cliutil.StateDir, cliutil.CacheDir)
	t.Setenv("DNSFILTER_BASE_URL", "")
	dir, err := cliutil.ConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(dir, "config.toml")
	credsPath, err := cliutil.CredentialsFilePath()
	if err != nil {
		t.Fatal(err)
	}
	for _, where := range []string{"config file", "credentials file"} {
		t.Run(where, func(t *testing.T) {
			_ = os.Remove(credsPath)
			if where == "config file" {
				if err := os.WriteFile(configPath, []byte("base_url = \"https://api.dnsfilter.com\"\nauth_header = \"TENANT-A-KEY\"\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := os.WriteFile(configPath, []byte("base_url = \"https://api.dnsfilter.com\"\n"), 0o600); err != nil {
					t.Fatal(err)
				}
				if err := cliutil.SaveCredentialsAt(credsPath, &cliutil.Credentials{AuthHeaderVal: "TENANT-A-KEY"}); err != nil {
					t.Fatal(err)
				}
			}
			t.Setenv("DNSFILTER_API_KEY", "TENANT-B-KEY")
			cfg, err := Load("")
			if err != nil {
				t.Fatal(err)
			}
			if cfg.AuthSource != "env:DNSFILTER_API_KEY" || cfg.AuthHeader() != "TENANT-B-KEY" {
				t.Fatalf("source %q sends %q, want env:DNSFILTER_API_KEY sending TENANT-B-KEY", cfg.AuthSource, cfg.AuthHeader())
			}
			if cfg.StoreScopeCredential() != "TENANT-B-KEY" {
				t.Fatalf("the local store is scoped to %q, not the key sent", cfg.StoreScopeCredential())
			}
			t.Setenv("DNSFILTER_API_KEY", "")
			if cfg, err = Load(""); err != nil || cfg.AuthHeader() != "TENANT-A-KEY" || cfg.AuthSource != "config" {
				t.Fatalf("without the environment key: source %q sends %q (%v), want the stored TENANT-A-KEY", cfg.AuthSource, cfg.AuthHeader(), err)
			}
		})
	}
}

// Plain `auth set-token` and logout (no --config) succeed when the default
// config.toml is a symlink to a read-only file, as a dotfiles manager makes
// it: the token goes to the default credentials file and the link and its
// target are left as they were.
func TestPlainSetTokenWithAReadOnlyLinkedDefaultConfig(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory write permission is set with chmod")
	}
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	home := testenv.Isolate(t, cliutil.ConfigDir, cliutil.DataDir, cliutil.StateDir, cliutil.CacheDir)
	t.Setenv("DNSFILTER_API_KEY", "")
	t.Setenv("DNSFILTER_BASE_URL", "")
	ro := filepath.Join(home, "dotfiles")
	target := r17TenantConfig(t, ro)
	before, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	dir, err := cliutil.ConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "config.toml")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable here: %v", err)
	}
	if err := os.Chmod(ro, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(ro, 0o700) })

	cfg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.SaveCredential("R18PLAIN01"); err != nil {
		t.Fatalf("set-token with a read-only linked config.toml: %v", err)
	}
	if got := r17Key(t, ""); got != "R18PLAIN01" {
		t.Fatalf("after set-token the default config authenticates with %q", got)
	}
	if info, err := os.Lstat(link); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("set-token replaced the managed config.toml link (%v)", err)
	}
	if cfg, err = Load(""); err != nil {
		t.Fatal(err)
	}
	if err := cfg.ClearTokens(); err != nil {
		t.Fatalf("logout with a read-only linked config.toml: %v", err)
	}
	if got := r17Key(t, ""); got != "" {
		t.Fatalf("after logout the default config still authenticates with %q", got)
	}
	if after, err := os.ReadFile(target); err != nil || string(after) != string(before) {
		t.Fatalf("the linked target changed: %q (%v)", after, err)
	}
}

// An explicit symlinked config whose file must change (a credential field to
// move out) is still written through the link, so its sibling credentials
// location does not move.
func TestExplicitLinkedConfigIsWrittenThroughWhenItChanges(t *testing.T) {
	home, _ := r17Home(t)
	target := r17TenantConfig(t, filepath.Join(home, "real-tenant"))
	if err := os.WriteFile(target, []byte("base_url = \"https://api.dnsfilter.com\"\napi_key = \"file-key\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	linkDir := filepath.Join(home, "linked-tenant")
	if err := os.MkdirAll(linkDir, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(linkDir, "config.toml")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable here: %v", err)
	}
	cfg, err := Load(link)
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.SaveCredential("tenant-key"); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Lstat(link); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("set-token replaced the explicit config link (%v)", err)
	}
	if data, _ := os.ReadFile(target); strings.Contains(string(data), "file-key") {
		t.Fatalf("the credential field was not moved out of the linked config: %q", data)
	}
	if got := r17Key(t, link); got != "tenant-key" {
		t.Fatalf("after set-token the linked config authenticates with %q, want tenant-key", got)
	}
}
