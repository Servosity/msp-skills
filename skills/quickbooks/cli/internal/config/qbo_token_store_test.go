// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (issue #492). Not generated. Recorded hand-fix
// `qbo-token-refresh` in handfixes.json.

package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"quickbooks-pp-cli/internal/cliutil"
	"quickbooks-pp-cli/internal/cliutil/testenv"
)

func qboTokenEnv(t *testing.T) string {
	t.Helper()
	home := testenv.Isolate(t)
	for _, name := range []string{"QUICKBOOKS_ACCESS_TOKEN", "QUICKBOOKS_CLIENT_ID", "QUICKBOOKS_CLIENT_SECRET", "QUICKBOOKS_REFRESH_TOKEN", "QUICKBOOKS_BASE_URL", "QUICKBOOKS_REALM_ID"} {
		t.Setenv(name, "")
	}
	return home
}

// QuickBooks rotates refresh tokens, so a refresh whose config file cannot
// be rewritten keeps the new tokens in the credentials file instead of
// putting the earlier file back (SaveTokens does, which would lose the only
// copy of the rotated refresh token).
func TestQBOSaveRefreshedTokensKeepsTheRotatedTokenWhenTheConfigWriteFails(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory modes do not make a write fail on Windows")
	}
	home := qboTokenEnv(t)
	cfgDir := filepath.Join(home, "ro")
	// The profile's own credential store (data/) stays writable; only the
	// directory holding config.toml becomes read-only.
	if err := os.MkdirAll(filepath.Join(cfgDir, "data"), 0o700); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(cfgDir, "config.toml")
	// A config that still carries credential fields shadows the store, so
	// the refresh must rewrite it, and here it cannot.
	if err := os.WriteFile(cfgPath, []byte("client_id = \"cid\"\nclient_secret = \"zz-secret\"\nrefresh_token = \"zz-refresh-old\"\naccess_token = \"zz-access-old\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(cfgDir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(cfgDir, 0o700) })
	if f, err := os.CreateTemp(cfgDir, "probe"); err == nil {
		_ = f.Close()
		_ = os.Remove(f.Name())
		t.Skip("this user can write a mode 0500 directory")
	}
	err = cfg.QBOSaveRefreshedTokens("zz-access-new", "zz-refresh-rotated", time.Now().Add(time.Hour))
	if err == nil {
		t.Fatal("a failed config rewrite was not reported")
	}
	creds, ok, lerr := cliutil.LoadCredentialsForConfig(cfgPath)
	if lerr != nil || !ok || creds.RefreshToken != "zz-refresh-rotated" || creds.AccessToken != "zz-access-new" {
		t.Fatalf("the credentials file lost the rotated tokens (loaded %v, err %v)", ok, lerr)
	}
}

// QBOReload re-reads with the argument Load was given: an explicit --config
// file stays explicit.
func TestQBOReloadReadsTheSameConfigFile(t *testing.T) {
	home := qboTokenEnv(t)
	p := filepath.Join(home, "explicit.toml")
	if err := os.WriteFile(p, []byte("base_url = \"https://sandbox-quickbooks.api.intuit.com/v3/company/42\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	fresh, err := cfg.QBOReload()
	if err != nil {
		t.Fatal(err)
	}
	if fresh.Path != p || fresh.BaseURL != "https://sandbox-quickbooks.api.intuit.com/v3/company/42" {
		t.Fatalf("reload read %q (%q), want the explicit file", fresh.Path, fresh.BaseURL)
	}
}

// The stored token is the one the files hold, not QUICKBOOKS_ACCESS_TOKEN.
func TestQBOStoredTokenIgnoresTheEnvironmentToken(t *testing.T) {
	qboTokenEnv(t)
	cfg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	exp := time.Now().Add(time.Hour).Truncate(time.Second)
	if err := cfg.SaveTokens("cid", "secret", "zz-stored-access", "zz-stored-refresh", exp); err != nil {
		t.Fatal(err)
	}
	t.Setenv("QUICKBOOKS_ACCESS_TOKEN", "zz-env-access")
	cfg, err = Load("")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AccessToken != "zz-env-access" {
		t.Fatalf("environment token not applied")
	}
	if got, gotExp := cfg.QBOStoredToken(); got != "zz-stored-access" || !gotExp.Equal(exp) {
		t.Fatalf("stored token = %q %v, want the file's", got, gotExp)
	}
	cfg.QBOAdoptTokens("zz-new-access", "zz-new-refresh", exp)
	if cfg.AuthHeader() != "Bearer zz-new-access" {
		t.Fatalf("an adopted token did not replace the environment's")
	}
}

// Codex round 2, class "every writer of a credential store shares the
// refresh lock": while a refresh holds the lock (between its re-read and its
// save), SaveTokens (auth set-token, auth login) and ClearTokens (auth
// logout) wait, so neither can snapshot, write and then restore the store
// over the rotated pair the refresh saves.
func TestQBOCredentialWritersWaitForARefresh(t *testing.T) {
	qboTokenEnv(t)
	cfg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	lockPath, err := cfg.QBOTokenLockPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o700); err != nil {
		t.Fatal(err)
	}
	for name, write := range map[string]func(*Config) error{
		"SaveTokens":  func(c *Config) error { return c.SaveTokens("", "", "zz-manual", "", time.Time{}) },
		"ClearTokens": func(c *Config) error { return c.ClearTokens() },
	} {
		t.Run(name, func(t *testing.T) {
			held, release, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
			go func() {
				_ = cliutil.QBOWithTokenLock(lockPath, func() error {
					close(held)
					<-release
					return nil
				})
			}()
			<-held
			go func() {
				c, err := Load("")
				if err != nil {
					done <- err
					return
				}
				done <- write(c)
			}()
			select {
			case err := <-done:
				close(release)
				t.Fatalf("%s wrote the store while a refresh held its lock (err %v)", name, err)
			case <-time.After(300 * time.Millisecond):
			}
			close(release)
			if err := <-done; err != nil {
				t.Fatalf("%s after the refresh: %v", name, err)
			}
		})
	}
}

// A refresh writes the credentials file first and rewrites an older config
// file second, so a crash between the two leaves the rotated tokens beside a
// config whose credentials Load would prefer. Those are the tokens Intuit no
// longer honors, so the next Load must read the rotated pair. The failed
// rewrite here leaves the files exactly as that crash does.
func TestQBOARefreshThatStopsBeforeTheConfigRewriteStillWinsOnRestart(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory modes do not make a write fail on Windows")
	}
	cfgPath, cfg := qboShadowingConfig(t)
	cfgDir := filepath.Dir(cfgPath)
	if err := os.Chmod(cfgDir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(cfgDir, 0o700) })
	if f, err := os.CreateTemp(cfgDir, "probe"); err == nil {
		_ = f.Close()
		_ = os.Remove(f.Name())
		t.Skip("this user can write a mode 0500 directory")
	}
	if err := cfg.QBOSaveRefreshedTokens("zz-access-new", "zz-refresh-new", time.Now().Add(time.Hour)); err == nil {
		t.Fatal("the config rewrite did not fail, so this test proves nothing")
	}
	if err := os.Chmod(cfgDir, 0o700); err != nil {
		t.Fatal(err)
	}
	reloaded, err := Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.AccessToken != "zz-access-new" || reloaded.RefreshToken != "zz-refresh-new" {
		t.Fatalf("restart read access=%q refresh=%q; want the rotated pair", reloaded.AccessToken, reloaded.RefreshToken)
	}
	if reloaded.ClientSecret != "zz-secret" {
		t.Fatalf("restart read client_secret=%q; want the one the store kept", reloaded.ClientSecret)
	}

	// Someone edits the config afterwards: that is a new choice, and it wins.
	edited := []byte("client_id = \"cid\"\nclient_secret = \"zz-secret\"\nrefresh_token = \"zz-refresh-edited\"\naccess_token = \"zz-access-edited\"\n")
	if err := os.WriteFile(cfgPath, edited, 0o600); err != nil {
		t.Fatal(err)
	}
	reloaded, err = Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.AccessToken != "zz-access-edited" || reloaded.RefreshToken != "zz-refresh-edited" {
		t.Fatalf("an edited config read access=%q refresh=%q; want the edit", reloaded.AccessToken, reloaded.RefreshToken)
	}
}

func qboShadowingConfig(t *testing.T) (string, *Config) {
	t.Helper()
	home := qboTokenEnv(t)
	cfgDir := filepath.Join(home, "ro")
	if err := os.MkdirAll(filepath.Join(cfgDir, "data"), 0o700); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(cfgDir, "config.toml")
	if err := os.WriteFile(cfgPath, []byte("client_id = \"cid\"\nclient_secret = \"zz-secret\"\nrefresh_token = \"zz-refresh-old\"\naccess_token = \"zz-access-old\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	return cfgPath, cfg
}

// Every writer of a --config profile's credentials uses the store a refresh of
// that profile uses, under its lock. A token set by hand lands where the next
// Load looks, and a logout signs this profile out and no other.
func TestQBOWritersFollowTheStoreARefreshUses(t *testing.T) {
	home := qboTokenEnv(t)
	cfgDir := filepath.Join(home, "acme")
	if err := os.MkdirAll(cfgDir, 0o700); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(cfgDir, "config.toml")
	if err := os.WriteFile(cfgPath, []byte("client_id = \"cid\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := cliutil.CredentialsFilePathForConfig(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := cliutil.QBOSaveCredentialsTo(store, &cliutil.Credentials{AccessToken: "zz-acme-old", RefreshToken: "zz-acme-refresh"}); err != nil {
		t.Fatal(err)
	}
	if err := cliutil.SaveCredentials(&cliutil.Credentials{AccessToken: "zz-global"}); err != nil {
		t.Fatal(err)
	}
	global, err := cliutil.CredentialsFilePath()
	if err != nil {
		t.Fatal(err)
	}
	load := func() *Config {
		t.Helper()
		c, err := Load(cfgPath)
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	globalIntact := func(when string) {
		t.Helper()
		b, err := os.ReadFile(global)
		if err != nil || !strings.Contains(string(b), "zz-global") {
			t.Errorf("%s: the global store changed: %q, %v", when, b, err)
		}
	}

	lockPath, err := load().QBOTokenLockPath()
	if err != nil {
		t.Fatal(err)
	}
	held, release, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() {
		_ = cliutil.QBOWithTokenLock(lockPath, func() error {
			close(held)
			<-release
			return nil
		})
	}()
	<-held
	go func() { done <- load().SaveTokens("", "", "zz-acme-manual", "", time.Time{}) }()
	select {
	case err := <-done:
		close(release)
		t.Fatalf("SaveTokens wrote while a refresh of this profile held its lock (err %v)", err)
	case <-time.After(300 * time.Millisecond):
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if got := load().AccessToken; got != "zz-acme-manual" {
		t.Errorf("after SaveTokens the profile reads %q; want the token just set", got)
	}
	globalIntact("SaveTokens")

	if err := load().ClearTokens(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(store); !os.IsNotExist(err) {
		t.Errorf("logout left this profile's store: %v", err)
	}
	globalIntact("ClearTokens")
}

// A config that inherited the global store: a refresh saves there and names
// the config it supersedes, so a restart reads the digest there too.
func TestQBOAStoppedRefreshOfAnInheritedStoreStillWins(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory modes do not make a write fail on Windows")
	}
	home := qboTokenEnv(t)
	cfgDir := filepath.Join(home, "inherits")
	if err := os.MkdirAll(cfgDir, 0o700); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(cfgDir, "config.toml")
	if err := os.WriteFile(cfgPath, []byte("client_id = \"cid\"\nrefresh_token = \"zz-refresh-old\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := cliutil.SaveCredentials(&cliutil.Credentials{AccessToken: "zz-access-old", RefreshToken: "zz-refresh-old"}); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(cfgDir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(cfgDir, 0o700) })
	if f, err := os.CreateTemp(cfgDir, "probe"); err == nil {
		_ = f.Close()
		_ = os.Remove(f.Name())
		t.Skip("this user can write a mode 0500 directory")
	}
	if err := cfg.QBOSaveRefreshedTokens("zz-access-new", "zz-refresh-new", time.Now().Add(time.Hour)); err == nil {
		t.Fatal("the config rewrite did not fail, so this test proves nothing")
	}
	if err := os.Chmod(cfgDir, 0o700); err != nil {
		t.Fatal(err)
	}
	reloaded, err := Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.AccessToken != "zz-access-new" || reloaded.RefreshToken != "zz-refresh-new" {
		t.Fatalf("restart read access=%q refresh=%q; want the rotated pair", reloaded.AccessToken, reloaded.RefreshToken)
	}
}
