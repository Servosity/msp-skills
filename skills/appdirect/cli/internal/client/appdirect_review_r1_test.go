// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"appdirect-pp-cli/internal/config"
)

// Codex r1 (appdirect reprint on 4.34.2) class cached authentication ignores
// credential override: the credential store holds client A's pair and the
// token minted for it; the environment names client B. A request for B must
// carry a token minted for B (A's token would return A's companies).
// Without the environment pair, A's stored token is reused and nothing mints.
func TestStoredTokenOfAnotherClientIsNeverSent(t *testing.T) {
	isolateCredentialStore(t)
	var mints atomic.Int32
	// AppDirect's token endpoint: POST <marketplace>/oauth2/token, a form body
	// (grant_type=client_credentials&scope=...), the client authenticated with
	// HTTP Basic. It answers a token named for the client that asked.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, _, ok := r.BasicAuth()
		raw, _ := io.ReadAll(r.Body)
		form, _ := url.ParseQuery(string(raw))
		if r.Method != http.MethodPost || r.URL.Path != "/oauth2/token" || !ok || form.Get("grant_type") != "client_credentials" {
			t.Errorf("unexpected token request %s %s (basic %v, body %q)", r.Method, r.URL.Path, ok, raw)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		mints.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"token-for-`+id+`","expires_in":3600}`)
	}))
	defer srv.Close()
	t.Setenv("APPDIRECT_BASE_URL", srv.URL+"/api")
	// The files hold client A's pair and the token minted for it, as
	// `auth login` with A's pair writes them (config.toml records the
	// marketplace, the 0600 credential store the pair and the token).
	files, err := config.Load("")
	if err != nil {
		t.Fatal(err)
	}
	if err := files.SaveTokens("client-a-4f1c", "secret-a-9d2e", "token-for-client-a-4f1c", "", time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	header := func() string {
		t.Helper()
		cfg, err := config.Load("")
		if err != nil {
			t.Fatal(err)
		}
		h, err := New(cfg, 5*time.Second, 0).authHeader(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		return h
	}
	if h := header(); h != "Bearer token-for-client-a-4f1c" || mints.Load() != 0 {
		t.Fatalf("the stored client's own token: got %q after %d mints, want it reused", h, mints.Load())
	}
	t.Setenv("APPDIRECT_CLIENT_ID", "client-b-7e3a")
	t.Setenv("APPDIRECT_CLIENT_SECRET", "secret-b-1b8f")
	if h := header(); h != "Bearer token-for-client-b-7e3a" {
		t.Fatalf("the environment names client B; the request carried %q (client A's stored token authenticates as A)", h)
	}
}

// isolateCredentialStore points every config, data, state, cache and
// credential path at a temp dir and clears every AppDirect credential
// variable, so a Load, a mint or a token save never reaches the operator's
// own files.
func isolateCredentialStore(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for k, v := range map[string]string{"APPDIRECT_HOME": dir, "HOME": dir, "USERPROFILE": dir,
		"XDG_DATA_HOME": filepath.Join(dir, "data"), "XDG_CONFIG_HOME": filepath.Join(dir, "config"),
		"XDG_STATE_HOME": filepath.Join(dir, "state"), "XDG_CACHE_HOME": filepath.Join(dir, "cache"),
		"APPDIRECT_CONFIG": "", "APPDIRECT_CLIENT_ID": "", "APPDIRECT_CLIENT_SECRET": "", "APPDIRECT_BASE_URL": "",
		"APPDIRECT_TOKEN_URL": "", "APPDIRECT_OAUTH_SCOPE": "", "APPDIRECT_NO_CONFIG_WRITE": ""} {
		t.Setenv(k, v)
	}
	return dir
}
