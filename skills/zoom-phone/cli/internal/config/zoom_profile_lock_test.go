// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"zoom-phone-pp-cli/internal/cliutil"
	"zoom-phone-pp-cli/internal/cliutil/testenv"
)

func zoomLockTestProfile(t *testing.T, clientID, accountID string) string {
	t.Helper()
	home := testenv.Isolate(t)
	for _, k := range []string{"ZOOM_PHONE_ACCOUNT_ID", "ZOOM_PHONE_CLIENT_ID", "ZOOM_PHONE_CLIENT_SECRET"} {
		t.Setenv(k, "")
	}
	acct := filepath.Join(home, "accounts", "acme")
	t.Setenv("ZOOM_PHONE_HOME", acct)
	path := filepath.Join(acct, "config.toml")
	zoomLockTestWrite(t, path, clientID, accountID)
	return path
}

func zoomLockTestWrite(t *testing.T, path, clientID, accountID string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	body := "client_id = '" + clientID + "'\nclient_secret = 'secret-" + clientID + "'\nphone_account_id = '" + accountID + "'\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestZoomProfileLockRefusesReplacedProfile(t *testing.T) {
	path := zoomLockTestProfile(t, "CID_A", "ACCT_A")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	// Another process replaces the profile with tenant B.
	zoomLockTestWrite(t, path, "CID_B", "ACCT_B")

	if _, err := cfg.ZoomLockProfileForSave(); !errors.Is(err, ErrZoomProfileChanged) {
		t.Fatalf("stale save: err=%v, want ErrZoomProfileChanged", err)
	}
	// An explicit replacement (auth login) only locks.
	release, err := cfg.ZoomLockProfileForReplace()
	if err != nil {
		t.Fatalf("replace lock: %v", err)
	}
	release()
}

func TestZoomProfileLockAllowsUnchangedAndFirstSave(t *testing.T) {
	path := zoomLockTestProfile(t, "CID_A", "ACCT_A")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	release, err := cfg.ZoomLockProfileForSave()
	if err != nil {
		t.Fatalf("unchanged profile refused: %v", err)
	}
	if err := cfg.SaveTokens(cfg.ClientID, cfg.ClientSecret, "tok-A", "", time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	release()

	// The generated save moved the identity into the credentials file and
	// cleared the file snapshot; a later save in this process is still checked.
	reloaded, err := Load(path)
	if err != nil || reloaded.ClientID != "CID_A" || reloaded.AccessToken != "tok-A" {
		t.Fatalf("after save: %+v %v", reloaded, err)
	}
	if err := os.Remove(filepath.Join(filepath.Dir(path), "data", "credentials.toml")); err != nil {
		t.Fatal(err)
	}
	zoomLockTestWrite(t, path, "CID_B", "ACCT_B")
	if _, err := cfg.ZoomLockProfileForSave(); !errors.Is(err, ErrZoomProfileChanged) {
		t.Fatalf("second save after replacement: err=%v, want ErrZoomProfileChanged", err)
	}

	// Nothing loaded (environment-only first run): never refused.
	fresh := &Config{Path: filepath.Join(t.TempDir(), "config.toml")}
	fresh.snapshotFileConfig()
	release, err = fresh.ZoomLockProfileForSave()
	if err != nil {
		t.Fatalf("first save refused: %v", err)
	}
	release()
}

func TestZoomProfileLockRefusesRemovedProfile(t *testing.T) {
	path := zoomLockTestProfile(t, "CID_A", "ACCT_A")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Dir(path)); err != nil {
		t.Fatal(err)
	}
	if _, err := cfg.ZoomLockProfileForSave(); !errors.Is(err, ErrZoomProfileChanged) {
		t.Fatalf("removed profile: err=%v", err)
	}
	if _, err := os.Lstat(filepath.Dir(path)); !os.IsNotExist(err) {
		t.Fatalf("a refused save recreated the removed profile directory: %v", err)
	}
}

func TestZoomProfileLockExcludesConcurrentHolders(t *testing.T) {
	path := zoomLockTestProfile(t, "CID_A", "ACCT_A")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	release, err := ZoomAcquireProfileLock(path)
	if err != nil {
		t.Fatal(err)
	}
	got := make(chan error, 1)
	go func() {
		r, err := cfg.ZoomLockProfileForSave()
		if err == nil {
			r()
		}
		got <- err
	}()
	select {
	case err := <-got:
		t.Fatalf("save took the lock while another holder had it (err=%v)", err)
	case <-time.After(200 * time.Millisecond):
	}
	release()
	select {
	case err := <-got:
		if err != nil {
			t.Fatalf("save after release: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("save never acquired the lock after release")
	}
}

// Codex round 9: a secret-only rotation is a different profile.
func TestZoomProfileLockRefusesSecretOnlyRotation(t *testing.T) {
	path := zoomLockTestProfile(t, "CID_A", "ACCT_A")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	body := "client_id = 'CID_A'\nclient_secret = 'rotated-secret'\nphone_account_id = 'ACCT_A'\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := cfg.ZoomLockProfileForSave(); !errors.Is(err, ErrZoomProfileChanged) {
		t.Fatalf("secret rotation: err=%v, want ErrZoomProfileChanged", err)
	}
	if strings.Contains(fmt.Sprint(err), "rotated-secret") || strings.Contains(fmt.Sprint(err), "secret-CID_A") {
		t.Fatalf("error leaked a secret: %v", err)
	}
}

// Codex round 9: an environment-only first run (nothing on disk at load) is
// refused when another writer created the profile meanwhile, and still saves
// when nothing appeared.
func TestZoomProfileLockFirstRunVsConcurrentCreation(t *testing.T) {
	testenv.Isolate(t)
	t.Setenv("ZOOM_PHONE_ACCOUNT_ID", "ACCT_A")
	t.Setenv("ZOOM_PHONE_CLIENT_ID", "CID_A")
	t.Setenv("ZOOM_PHONE_CLIENT_SECRET", "secret-A")

	cfg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.zoomLoadedSnapshot().empty() {
		t.Fatalf("env-only run loaded a stored identity: %+v", cfg.zoomLoadedSnapshot())
	}
	release, err := cfg.ZoomLockProfileForSave()
	if err != nil {
		t.Fatalf("true first save refused: %v", err)
	}
	release()

	// Another process saves tenant B to the default profile meanwhile.
	credsPath, err := cliutil.CredentialsFilePath()
	if err != nil {
		t.Fatal(err)
	}
	if err := cliutil.AtomicWritePrivateFile(credsPath, []byte("client_id = 'CID_B'\nclient_secret = 'secret-B'\nphone_account_id = 'ACCT_B'\n"), 0o600, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := cfg.ZoomLockProfileForSave(); !errors.Is(err, ErrZoomProfileChanged) {
		t.Fatalf("first run after a concurrent profile creation: err=%v, want ErrZoomProfileChanged", err)
	}
}

// Codex round 11: one lock-path function. Account profiles lock under
// <accounts>/.locks (0700 dir, 0600 file), case variants share the lock, a
// symlinked .locks is refused, and other configs lock beside themselves.
func TestZoomProfileLockPath(t *testing.T) {
	home := testenv.Isolate(t)
	root := filepath.Join(home, "cfg", "accounts")
	profile := filepath.Join(root, "acme", "config.toml")
	if got, want := ZoomProfileLockPath(profile), filepath.Join(root, ".locks", "acme"); got != want {
		t.Fatalf("lock path = %q, want %q", got, want)
	}
	for _, variant := range []string{filepath.Join(root, "ACME", "CONFIG.TOML"), filepath.Join(root, "acme", ".", "config.toml")} {
		if got := ZoomProfileLockPath(variant); got != filepath.Join(root, ".locks", "acme") {
			t.Fatalf("variant %s locks %q", variant, got)
		}
	}
	other := filepath.Join(home, "cfg", "zoom-phone-cli", "config.toml")
	if got := ZoomProfileLockPath(other); got != other {
		t.Fatalf("non-profile config locks %q, want beside itself", got)
	}

	release, err := ZoomAcquireProfileLock(profile)
	if err != nil {
		t.Fatal(err)
	}
	release()
	for path, want := range map[string]os.FileMode{filepath.Join(root, ".locks"): 0o700, filepath.Join(root, ".locks", "acme.lock"): 0o600} {
		st, err := os.Stat(path)
		if err == nil && runtime.GOOS == "windows" {
			continue // no POSIX modes on Windows (0777/0666)
		}
		if err != nil || st.Mode().Perm() != want {
			t.Fatalf("%s: mode %v err %v, want %o", path, st.Mode().Perm(), err, want)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "acme")); !os.IsNotExist(err) {
		t.Fatalf("taking the lock created the account directory: %v", err)
	}

	// A symlinked .locks directory is refused.
	if err := os.RemoveAll(filepath.Join(root, ".locks")); err != nil {
		t.Fatal(err)
	}
	elsewhere := filepath.Join(home, "elsewhere")
	if err := os.MkdirAll(elsewhere, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(elsewhere, filepath.Join(root, ".locks")); err != nil {
		t.Fatal(err)
	}
	if _, err := ZoomAcquireProfileLock(profile); err == nil {
		t.Fatal("symlinked .locks accepted")
	}
}

// The release must not return while the lock file is still locked and open:
// on Windows that handle under <accounts>/.locks blocks renaming or removing
// the accounts directory (Access is denied). A try-lock right after release
// must win every time; before the fix it raced the unlocking goroutine.
func TestZoomProfileLockReleaseWaitsForUnlock(t *testing.T) {
	path := zoomLockTestProfile(t, "C1", "A1")
	lockBase := ZoomProfileLockPath(path)
	for i := 0; i < 200; i++ {
		release, err := ZoomAcquireProfileLock(path)
		if err != nil {
			t.Fatal(err)
		}
		release()
		got, err := cliutil.TryWithFileLock(lockBase, func() error { return nil })
		if err != nil || !got {
			t.Fatalf("iteration %d: lock still held after release (got=%v err=%v)", i, got, err)
		}
	}
}
