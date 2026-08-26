// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel feature (preserved across regen). Regression tests for the
// findings an independent Codex review raised against the client-credentials
// re-mint. Each test is written to FAIL against the code as it stood before the
// fix, so the ledger marker and the test agree on what is being protected.

package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"cipp-pp-cli/internal/config"
)

func cfgWithCreds(t *testing.T, authority, dir string) *config.Config {
	t.Helper()
	path := filepath.Join(dir, "config.toml")
	cfg := &config.Config{
		Path:         path,
		BaseURL:      "https://cipp.example-msp.test/api",
		TenantID:     "11111111-1111-1111-1111-111111111111",
		ClientID:     "00000000-0000-0000-0000-000000000000",
		ClientSecret: "s3cr3t-value",
		Authority:    authority,
		AccessToken:  "stale-token",
		TokenExpiry:  time.Now().Add(-time.Hour), // genuinely expired
	}
	return cfg
}

// A plaintext authority would put client_secret on the wire in cleartext. The
// token endpoint must be refused, not downgraded.
func TestTokenEndpointRefusesPlaintextAuthority(t *testing.T) {
	dir := t.TempDir()
	cfg := cfgWithCreds(t, "http://login.evil.test", dir)
	if got := cfg.TokenEndpoint(); got != "" {
		t.Fatalf("a plaintext authority must yield no token endpoint, got %q", got)
	}
	cfg.Authority = "https://login.microsoftonline.com"
	if got := cfg.TokenEndpoint(); got == "" || !strings.HasPrefix(got, "https://") {
		t.Fatalf("an https authority must still work, got %q", got)
	}
	// Loopback stays usable so a local mock authority and the test harness work.
	cfg.Authority = "http://127.0.0.1:8080"
	if got := cfg.TokenEndpoint(); got == "" {
		t.Fatal("loopback http must remain allowed for local mock authorities")
	}
}

func TestAuthorityIsSecure(t *testing.T) {
	for _, tc := range []struct {
		authority string
		want      bool
	}{
		{"https://login.microsoftonline.com", true},
		{"https://login.microsoftonline.us", true},
		{"http://127.0.0.1:9000", true},
		{"http://localhost:9000", true},
		{"http://login.microsoftonline.com", false},
		{"http://192.168.1.10", false},
		{"ftp://login.example.com", false},
		{"", false},
		{"not a url", false},
	} {
		if got := config.AuthorityIsSecure(tc.authority); got != tc.want {
			t.Errorf("AuthorityIsSecure(%q) = %v, want %v", tc.authority, got, tc.want)
		}
	}
}

// A vendor-controlled `error` code can echo the submitted form. Masking only
// error_description leaves the secret reachable through the other field.
func TestMintErrorMasksTheSecretInBothVendorFields(t *testing.T) {
	dir := t.TempDir()
	secret := "super-secret-value"
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error":             secret, // the hostile case: the code echoes the form
			"error_description": "invalid_client " + secret,
		})
	}))
	defer srv.Close()

	cfg := cfgWithCreds(t, srv.URL, dir)
	cfg.ClientSecret = secret
	c := New(cfg, 30*time.Second, 0)
	c.HTTPClient = srv.Client()

	err := c.mintClientCredentials(context.Background())
	if err == nil {
		t.Fatal("expected the mint to fail")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("the client secret leaked into the error: %s", err)
	}
}

// time.Duration is int64 nanoseconds; a huge expires_in wraps negative and the
// freshly minted token looks already-expired, re-minting on every request.
func TestTokenLifetimeToDurationDoesNotOverflow(t *testing.T) {
	for _, seconds := range []int{1<<62 - 1, 1 << 40, MaxTokenLifetimeSeconds + 1} {
		d := TokenLifetimeToDuration(seconds)
		if d <= 0 {
			t.Fatalf("TokenLifetimeToDuration(%d) = %v, must stay positive", seconds, d)
		}
	}
	if got := TokenLifetimeToDuration(3600); got != time.Hour {
		t.Fatalf("a normal lifetime must pass through unchanged, got %v", got)
	}
	if got := TokenLifetimeToDuration(0); got != 0 {
		t.Fatalf("zero means unknown, got %v", got)
	}
}

func TestMintClampsAbsurdExpiresIn(t *testing.T) {
	dir := t.TempDir()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "fresh", "expires_in": 1 << 40, "token_type": "Bearer",
		})
	}))
	defer srv.Close()
	cfg := cfgWithCreds(t, srv.URL, dir)
	c := New(cfg, 30*time.Second, 0)
	c.HTTPClient = srv.Client()

	if err := c.mintClientCredentials(context.Background()); err != nil {
		t.Fatalf("mint failed: %v", err)
	}
	if !cfg.TokenExpiry.After(time.Now()) {
		t.Fatalf("expiry wrapped into the past: %v", cfg.TokenExpiry)
	}
}

// The MCP server builds a fresh Client and re-reads config per tool call. When
// the config file cannot be written, nothing carries the token between calls -
// so without a process-level cache every call mints again.
func TestFailedPersistenceDoesNotStormTheTokenEndpoint(t *testing.T) {
	var mints int64
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&mints, 1)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "fresh", "expires_in": 3600, "token_type": "Bearer",
		})
	}))
	defer srv.Close()

	// A directory standing where the config file should be: every write fails.
	dir := t.TempDir()
	unwritable := filepath.Join(dir, "config.toml")
	if err := os.Mkdir(unwritable, 0o755); err != nil {
		t.Fatal(err)
	}

	ForgetMintedTokens()
	t.Cleanup(ForgetMintedTokens)

	for i := 0; i < 8; i++ {
		cfg := cfgWithCreds(t, srv.URL, dir) // fresh config, exactly like a new tool call
		c := New(cfg, 30*time.Second, 0)
		c.HTTPClient = srv.Client()
		if _, err := c.authHeader(context.Background()); err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
	}
	if got := atomic.LoadInt64(&mints); got != 1 {
		t.Fatalf("token endpoint hit %d times across 8 calls with an unwritable config; want 1", got)
	}
}

// The cache must never hand a token to a different tenant or client.
func TestMintCacheIsKeyedOnTheCredential(t *testing.T) {
	var mints int64
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&mints, 1)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "fresh", "expires_in": 3600, "token_type": "Bearer",
		})
	}))
	defer srv.Close()
	dir := t.TempDir()
	ForgetMintedTokens()
	t.Cleanup(ForgetMintedTokens)

	a := cfgWithCreds(t, srv.URL, dir)
	ca := New(a, 30*time.Second, 0)
	ca.HTTPClient = srv.Client()
	if _, err := ca.authHeader(context.Background()); err != nil {
		t.Fatal(err)
	}

	b := cfgWithCreds(t, srv.URL, dir)
	b.TenantID = "22222222-2222-2222-2222-222222222222"
	cb := New(b, 30*time.Second, 0)
	cb.HTTPClient = srv.Client()
	if _, err := cb.authHeader(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := atomic.LoadInt64(&mints); got != 2 {
		t.Fatalf("a second tenant must mint its own token; endpoint hit %d times, want 2", got)
	}
}
