// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"cloudally-pp-cli/internal/config"
)

// partnerAuthStub answers a partner login and an account-token mint.
func partnerAuthStub(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == cloudallyPartnerLoginPath:
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["password"] != "secret-pw" {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"errorMessage":"Bad credentials"}`))
				return
			}
			_, _ = w.Write([]byte(`{"accessToken":"partner-token","expiresIn":1893456000000,"refreshToken":"partner-refresh"}`))
		case strings.HasSuffix(r.URL.Path, "/partner-access-token"):
			_, _ = w.Write([]byte(`{"accessToken":"account-token"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func sidecarExists(t *testing.T) bool {
	t.Helper()
	path, err := authContextPath()
	if err != nil {
		t.Fatalf("authContextPath: %v", err)
	}
	_, err = os.Lstat(path)
	return err == nil
}

// The reviewer's sequence: partner login, act in a customer, log out. The
// partner tokens kept aside must go with the logout, so use-account --partner
// cannot bring the session back without a password.
func TestLogoutRemovesPartnerSidecar(t *testing.T) {
	isolateAuthState(t)
	srv := partnerAuthStub(t)
	t.Setenv("CLOUDALLY_CLIENT_ID", "cid")
	t.Setenv("CLOUDALLY_CLIENT_SECRET", "csecret")
	t.Setenv("CLOUDALLY_BASE_URL", srv.URL)

	if _, _, err := runAuthCmd(t, "secret-pw\n", "auth", "login", "--partner", "--email", "tech@example.com", "--password-stdin"); err != nil {
		t.Fatalf("login: %v", err)
	}
	if _, _, err := runAuthCmd(t, "", "auth", "use-account", "5f1c2a"); err != nil {
		t.Fatalf("use-account: %v", err)
	}
	if !sidecarExists(t) {
		t.Fatal("expected the sidecar after use-account")
	}

	if _, _, err := runAuthCmd(t, "", "auth", "logout"); err != nil {
		t.Fatalf("logout: %v", err)
	}
	if sidecarExists(t) {
		t.Fatal("logout left auth-context.json (with the partner tokens) behind")
	}
	_, _, err := runAuthCmd(t, "", "auth", "use-account", "--partner")
	if err == nil {
		t.Fatal("use-account --partner restored a partner session after logout")
	}
	if cliErrCode(t, err) != 4 {
		t.Fatalf("want an auth error, got %v", err)
	}
	cfg, err := config.Load("")
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if cfg.AccessToken != "" || cfg.RefreshToken != "" {
		t.Fatalf("a token is active after logout: (%q, %q)", cfg.AccessToken, cfg.RefreshToken)
	}
}

// A sidecar logout cannot delete is reported, not silently left behind.
func TestLogoutFailsLoudlyWhenSidecarCannotBeRemoved(t *testing.T) {
	isolateAuthState(t)
	path, err := authContextPath()
	if err != nil {
		t.Fatalf("authContextPath: %v", err)
	}
	// A non-empty directory at the sidecar path cannot be removed by
	// os.Remove on any platform.
	if err := os.MkdirAll(filepath.Join(path, "keep"), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	_, _, err = runAuthCmd(t, "", "auth", "logout")
	if err == nil {
		t.Fatal("logout succeeded although the partner session file could not be removed")
	}
	if !strings.Contains(err.Error(), authContextSource) || !strings.Contains(err.Error(), path) {
		t.Fatalf("error should name the file: %v", err)
	}
}

// set-token replaces the stored credential, so a sidecar describing the old
// session (and holding its partner tokens) has to go too.
func TestSetTokenRemovesStaleSidecar(t *testing.T) {
	isolateAuthState(t)
	if err := saveAuthContext(&authContext{Scope: authScopeAccount, AccountID: "5f1c2a", PartnerAccessToken: "partner-token", PartnerRefreshToken: "partner-refresh"}); err != nil {
		t.Fatalf("seed sidecar: %v", err)
	}
	if _, _, err := runAuthCmd(t, "replacement-token\n", "auth", "set-token"); err != nil {
		t.Fatalf("set-token: %v", err)
	}
	if sidecarExists(t) {
		t.Fatal("set-token left a sidecar that describes the previous session")
	}
	if _, _, err := runAuthCmd(t, "", "auth", "use-account", "--partner"); err == nil {
		t.Fatal("use-account --partner restored the replaced partner session")
	}
}

// Like credentials.toml, a sidecar another user can read is refused on load.
func TestLoadAuthContextRefusesLoosePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX mode bits; the Windows DACL guard is covered by cliutil's SDDL tests")
	}
	isolateAuthState(t)
	if err := saveAuthContext(&authContext{Scope: authScopeAccount, AccountID: "5f1c2a", PartnerAccessToken: "partner-token"}); err != nil {
		t.Fatalf("save: %v", err)
	}
	if _, ok := loadAuthContext(); !ok {
		t.Fatal("a private sidecar should load")
	}
	path, _ := authContextPath()
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	if actx, ok := loadAuthContext(); ok {
		t.Fatalf("a world-readable sidecar was trusted: %+v", actx)
	}

	// A symlink to a loose file is judged by its target.
	target := filepath.Join(t.TempDir(), "elsewhere.json")
	data, _ := json.Marshal(authContext{Scope: authScopePartner, PartnerAccessToken: "partner-token"})
	if err := os.WriteFile(target, data, 0o644); err != nil {
		t.Fatalf("write target: %v", err)
	}
	_ = os.Remove(path)
	if err := os.Symlink(target, path); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	if actx, ok := loadAuthContext(); ok {
		t.Fatalf("a sidecar linked to a world-readable file was trusted: %+v", actx)
	}
}

// The write is checked the way SaveCredentials checks credentials.toml; a
// file that fails the guard does not stay on disk holding partner tokens.
func TestSaveAuthContextVerifiesWrittenFile(t *testing.T) {
	isolateAuthState(t)
	unsafe := errors.New("owner is not the current user")
	orig := authContextVerifyPerms
	authContextVerifyPerms = func(string) error { return unsafe }
	t.Cleanup(func() { authContextVerifyPerms = orig })

	err := saveAuthContext(&authContext{Scope: authScopePartner, PartnerAccessToken: "partner-token", PartnerTokenExpiry: time.Now()})
	if !errors.Is(err, unsafe) {
		t.Fatalf("want the guard's error, got %v", err)
	}
	if sidecarExists(t) {
		t.Fatal("a sidecar that failed the private-file check was left on disk")
	}
}
