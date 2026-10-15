// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"watchguard-pp-cli/internal/config"
)

// hand-fix (handfixes.json token-bound-to-client): auth login given one flag
// saves the exact client pair it minted with, so the saved secret can mint
// again later and never sits beside another client's ID.
func TestAuthLoginSavesThePairItUsed(t *testing.T) {
	isolateAuthEnv(t)
	srv := tokenServer(t)
	home := t.TempDir()
	t.Setenv("WATCHGUARD_HOME", home)
	t.Setenv("WATCHGUARD_TOKEN_URL", srv.URL+"/oauth/token")
	t.Setenv("WATCHGUARD_CLIENT_ID", "rw")
	if out, err := runAuthCmd(t, "", "auth", "login", "--client-secret", "rwsec"); err != nil {
		t.Fatalf("auth login: %v\n%s", err, out)
	}
	t.Setenv("WATCHGUARD_CLIENT_ID", "")
	t.Setenv("WATCHGUARD_TOKEN_URL", "")
	cfg, err := config.Load(filepath.Join(home, "config", "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ClientID != "rw" || cfg.ClientSecret != "rwsec" {
		t.Fatalf("login saved id=%q secret=%q, want the pair it used", cfg.ClientID, cfg.ClientSecret)
	}
	if want := config.TokenBindingFor("rw", resolveClientCredentialsScope(), srv.URL+"/oauth/token"); cfg.TokenBinding != want {
		t.Fatalf("login saved binding %q, want the one for the client it minted with (%q)", cfg.TokenBinding, want)
	}
}

// auth login refuses a placeholder client ID or secret (flag or env) before
// any token request.
func TestAuthLoginRefusesPlaceholders(t *testing.T) {
	isolateAuthEnv(t)
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"tok","expires_in":3600}`))
	}))
	defer srv.Close()
	t.Setenv("WATCHGUARD_HOME", t.TempDir())
	t.Setenv("WATCHGUARD_TOKEN_URL", srv.URL+"/oauth/token")
	t.Setenv("WATCHGUARD_CLIENT_ID", "<client-id>")
	t.Setenv("WATCHGUARD_CLIENT_SECRET", "<client-secret>")
	out, err := runAuthCmd(t, "", "auth", "login")
	if err == nil || !strings.Contains(out+err.Error(), "$WATCHGUARD_CLIENT_ID holds a placeholder") {
		t.Fatalf("env placeholder login: err=%v out=%s", err, out)
	}
	out, err = runAuthCmd(t, "", "auth", "login", "--client-id", "rw", "--client-secret", "<value>")
	if err == nil || !strings.Contains(out+err.Error(), "--client-secret holds a placeholder") {
		t.Fatalf("flag placeholder login: err=%v out=%s", err, out)
	}
	// An unresolved MCPB ${...} template is unset, as config.Load reads it.
	t.Setenv("WATCHGUARD_CLIENT_ID", "${user_config.watchguard_client_id}")
	t.Setenv("WATCHGUARD_CLIENT_SECRET", "${user_config.watchguard_client_secret}")
	out, err = runAuthCmd(t, "", "auth", "login")
	if err == nil || !strings.Contains(out+err.Error(), "client ID and secret required") {
		t.Fatalf("unresolved template login: err=%v out=%s", err, out)
	}
	if n := hits.Load(); n != 0 {
		t.Fatalf("placeholder credentials reached the token endpoint %d times", n)
	}
}

// hand-fix (handfixes.json token-error-withholds-credentials): auth login's
// own token request withholds an error that echoes the Basic credentials.
func TestAuthLoginErrorNeverEchoesCredentials(t *testing.T) {
	isolateAuthEnv(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid_client","authorization":"` + r.Header.Get("Authorization") + `"}`))
	}))
	defer srv.Close()
	t.Setenv("WATCHGUARD_HOME", t.TempDir())
	t.Setenv("WATCHGUARD_TOKEN_URL", srv.URL+"/oauth/token")
	out, err := runAuthCmd(t, "", "auth", "login", "--client-id", "AccessID-7f3a91", "--client-secret", "ApiPw-SENTINEL-9z")
	b64 := base64.StdEncoding.EncodeToString([]byte("AccessID-7f3a91:ApiPw-SENTINEL-9z"))
	if err == nil || strings.Contains(out+err.Error(), b64) || strings.Contains(out+err.Error(), "ApiPw-SENTINEL") || !strings.Contains(err.Error(), "withheld") {
		t.Fatalf("auth login echoed credentials: err=%v out=%s", err, out)
	}
}
