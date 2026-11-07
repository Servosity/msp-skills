// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package config

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

// datto-rmm handfixes platform-routes-every-request, minted-token-bound-to-
// key-and-host and mint-pinned-to-api-host (datto_oauth.go).

type tokenDouble struct {
	srv    *httptest.Server
	mints  atomic.Int32
	mu     sync.Mutex
	forms  []url.Values
	access string
}

func newTokenDouble(t *testing.T, access string) *tokenDouble {
	t.Helper()
	d := &tokenDouble{access: access}
	d.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/auth/oauth/token" || r.Method != http.MethodPost {
			http.Error(w, "unexpected", http.StatusNotFound)
			return
		}
		if r.Header.Get("Authorization") != "Basic "+base64.StdEncoding.EncodeToString([]byte("public-client:public")) {
			http.Error(w, "client", http.StatusUnauthorized)
			return
		}
		_ = r.ParseForm()
		d.mu.Lock()
		d.forms = append(d.forms, r.PostForm)
		d.mu.Unlock()
		d.mints.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"` + d.access + `","token_type":"bearer","expires_in":359999}`))
	}))
	t.Cleanup(d.srv.Close)
	return d
}

func dattoEnv(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home) // os.UserHomeDir reads USERPROFILE on Windows
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))
	for _, k := range []string{"DATTO_RMM_BASE_URL", "DATTO_RMM_PLATFORM", "DATTO_RMM_API_URL", "DATTO_RMM_TOKEN",
		"DATTO_RMM_API_KEY", "DATTO_RMM_API_SECRET_KEY", "DATTO_RMM_CONFIG", NoConfigWriteEnv} {
		t.Setenv(k, "")
		os.Unsetenv(k)
	}
	dattoMintMu.Lock()
	dattoProcToken = map[string]dattoProcessToken{}
	dattoMintMu.Unlock()
	return home
}

func TestPlatformRoutesTheBaseURL(t *testing.T) {
	dattoEnv(t)
	t.Setenv("DATTO_RMM_PLATFORM", "Pinotage")
	cfg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.BaseURL != "https://pinotage-api.centrastage.net/api" {
		t.Fatalf("DATTO_RMM_PLATFORM=Pinotage: base %q", cfg.BaseURL)
	}
	t.Setenv("DATTO_RMM_API_URL", "https://zinfandel-api.centrastage.net/")
	if cfg, _ = Load(""); cfg.BaseURL != "https://zinfandel-api.centrastage.net/api" {
		t.Fatalf("DATTO_RMM_API_URL beats the platform: base %q", cfg.BaseURL)
	}
	t.Setenv("DATTO_RMM_BASE_URL", "https://vidal-api.centrastage.net/api")
	if cfg, _ = Load(""); cfg.BaseURL != "https://vidal-api.centrastage.net/api" {
		t.Fatalf("DATTO_RMM_BASE_URL beats every alias: base %q", cfg.BaseURL)
	}
	os.Unsetenv("DATTO_RMM_BASE_URL")
	os.Unsetenv("DATTO_RMM_API_URL")
	t.Setenv("DATTO_RMM_PLATFORM", "atlantis")
	if _, err := Load(""); err == nil || !strings.Contains(err.Error(), "unknown DATTO_RMM_PLATFORM") {
		t.Fatalf("an unknown platform must refuse to load, got %v", err)
	}
	t.Setenv("DATTO_RMM_PLATFORM", "${user_config.datto_rmm_platform}")
	if cfg, err = Load(""); err != nil || cfg.BaseURL != "https://merlot-api.centrastage.net/api" {
		t.Fatalf("an unresolved .mcpb placeholder is unset: base %q err %v", cfg.BaseURL, err)
	}
}

// The platform is an explicit endpoint choice: the untrusted-route checks see
// it exactly like DATTO_RMM_BASE_URL, so the mint and the requests agree.
func TestPlatformIsTheCredentialTarget(t *testing.T) {
	dattoEnv(t)
	t.Setenv("DATTO_RMM_PLATFORM", "syrah")
	cfg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.credentialTargetURL(); got != "https://syrah-api.centrastage.net/api" {
		t.Fatalf("credential target %q, want the platform base", got)
	}
}

func TestMintPostsThePasswordGrantToTheAPIHost(t *testing.T) {
	dattoEnv(t)
	d := newTokenDouble(t, "mintedTokenUnit-7c2e")
	t.Setenv("DATTO_RMM_BASE_URL", d.srv.URL+"/api")
	t.Setenv("DATTO_RMM_API_KEY", "keyUnit-AAAA1111")
	t.Setenv("DATTO_RMM_API_SECRET_KEY", "secretUnit-BBBB2222")
	cfg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if err := EnsureDattoToken(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.AuthHeader() != "Bearer mintedTokenUnit-7c2e" || d.mints.Load() != 1 {
		t.Fatalf("header %q after %d mints", cfg.AuthHeader(), d.mints.Load())
	}
	f := d.forms[0]
	if f.Get("grant_type") != "password" || f.Get("username") != "keyUnit-AAAA1111" || f.Get("password") != "secretUnit-BBBB2222" {
		t.Fatalf("form %v", f)
	}
	// A second client in this process reuses the token it minted.
	cfg2, _ := Load("")
	if err := EnsureDattoToken(context.Background(), cfg2); err != nil || d.mints.Load() != 1 {
		t.Fatalf("second client: err %v, %d mints", err, d.mints.Load())
	}
}

func TestStoredTokenIsBoundToItsKeyAndHost(t *testing.T) {
	dattoEnv(t)
	d := newTokenDouble(t, "tokenForKeyA-9f1d")
	t.Setenv("DATTO_RMM_BASE_URL", d.srv.URL+"/api")
	t.Setenv("DATTO_RMM_API_KEY", "keyA-CCCC3333")
	t.Setenv("DATTO_RMM_API_SECRET_KEY", "secretA-DDDD4444")
	cfg, _ := Load("")
	if err := EnsureDattoToken(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	// A new process (empty process cache) with the same key reuses the
	// stored token.
	dattoProcToken = map[string]dattoProcessToken{}
	cfg, _ = Load("")
	if err := EnsureDattoToken(context.Background(), cfg); err != nil || d.mints.Load() != 1 || cfg.AccessToken != "tokenForKeyA-9f1d" {
		t.Fatalf("same key in a new process: err %v, %d mints, token %q", err, d.mints.Load(), cfg.AccessToken)
	}
	// Another key never sends key A's token: it mints its own.
	dattoProcToken = map[string]dattoProcessToken{}
	d.access = "tokenForKeyB-3b8a"
	t.Setenv("DATTO_RMM_API_KEY", "keyB-EEEE5555")
	cfg, _ = Load("")
	if err := EnsureDattoToken(context.Background(), cfg); err != nil || d.mints.Load() != 2 || cfg.AccessToken != "tokenForKeyB-3b8a" {
		t.Fatalf("another key: err %v, %d mints, token %q", err, d.mints.Load(), cfg.AccessToken)
	}
	// With no key in effect, a stored minted token goes only to its host.
	os.Unsetenv("DATTO_RMM_API_KEY")
	os.Unsetenv("DATTO_RMM_API_SECRET_KEY")
	cfg, _ = Load("")
	if err := EnsureDattoToken(context.Background(), cfg); err != nil || cfg.AuthHeader() != "Bearer tokenForKeyB-3b8a" {
		t.Fatalf("no key, same host: err %v header %q", err, cfg.AuthHeader())
	}
	t.Setenv("DATTO_RMM_BASE_URL", "https://concord-api.centrastage.net/api")
	cfg, _ = Load("")
	if err := EnsureDattoToken(context.Background(), cfg); err != nil || cfg.AuthHeader() != "" {
		t.Fatalf("no key, another host: the stored token must not be sent (err %v header %q)", err, cfg.AuthHeader())
	}
}

func TestTokenRequestNeverFollowsARedirect(t *testing.T) {
	dattoEnv(t)
	var elsewhere atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { elsewhere.Add(1) }))
	t.Cleanup(other.Close)
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+"/auth/oauth/token", http.StatusTemporaryRedirect)
	}))
	t.Cleanup(api.Close)
	_, _, err := MintDattoToken(context.Background(), nil, api.URL+"/api", "keyRedirect-1111", "secretRedirect-2222")
	if err == nil || elsewhere.Load() != 0 {
		t.Fatalf("a redirected token request must fail with nothing sent elsewhere (err %v, %d requests elsewhere)", err, elsewhere.Load())
	}
	if strings.Contains(err.Error(), "secretRedirect-2222") || strings.Contains(err.Error(), other.URL) {
		t.Fatalf("the error names the secret or the redirect target: %v", err)
	}
	if _, _, err := MintDattoToken(context.Background(), nil, "https://user:pw@merlot-api.centrastage.net/api", "k-1111", "s-2222"); err == nil {
		t.Fatal("a base URL with userinfo must not mint")
	}
}

func TestStoreScopeSurvivesARemint(t *testing.T) {
	dattoEnv(t)
	base := "https://merlot-api.centrastage.net/api"
	scopeUser := "keyScope-1234"
	withKey := &Config{BaseURL: base, DattoRmmApiKey: scopeUser, AccessToken: "first"}
	scope := withKey.StoreScopeCredential()
	withKey.AccessToken = "second"
	if withKey.StoreScopeCredential() != scope || scope == "" {
		t.Fatalf("the store scope moved with the token: %q", scope)
	}
	tokenOnly := &Config{BaseURL: base, AccessToken: "third", ClientID: DattoTokenBinding(base, "keyScope-1234")}
	if tokenOnly.StoreScopeCredential() != scope {
		t.Fatal("a saved token minted for the same key opens another store")
	}
	if strings.Contains(scope, "keyScope-1234") {
		t.Fatal("the scope repeats the API key")
	}
	if b := DattoTokenBinding(base, "keyScope-1234"); len(b) != 32 || strings.ContainsAny(b, "|:/-") {
		t.Fatalf("binding %q is not 32 opaque hex digits", b)
	}
	if dattoBindingOrigin(DattoTokenBinding(base, "k")) != dattoOriginFingerprint("https://MERLOT-api.centrastage.net:443/api/v2") {
		t.Fatal("one host, two spellings: the fingerprints differ")
	}
}
