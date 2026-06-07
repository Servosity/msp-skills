// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"action1-pp-cli/internal/config"
)

// Action1's token endpoint takes a JSON body (not form-encoded). This pins that
// contract: a fresh grant POSTs {client_id, client_secret} as application/json
// and the returned bearer is cached on the config.
func TestMintClientCredentials_JSONBody(t *testing.T) {
	var gotCT, gotMethod, gotPath string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCT = r.Header.Get("Content-Type")
		gotMethod = r.Method
		gotPath = r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"tok-abc","refresh_token":"ref-xyz","expires_in":3600,"token_type":"bearer"}`))
	}))
	defer srv.Close()

	cfg := &config.Config{
		BaseURL:      srv.URL,
		ClientID:     "id@example.com",
		ClientSecret: "shhh",
		Path:         filepath.Join(t.TempDir(), "config.toml"),
	}
	c := New(cfg, 10*time.Second, 0)

	h, err := c.authHeader(context.Background())
	if err != nil {
		t.Fatalf("authHeader returned error: %v", err)
	}
	if h != "Bearer tok-abc" {
		t.Fatalf("auth header = %q, want %q", h, "Bearer tok-abc")
	}
	if gotMethod != "POST" || gotPath != "/oauth2/token" {
		t.Fatalf("mint request = %s %s, want POST /oauth2/token", gotMethod, gotPath)
	}
	if gotCT != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json (Action1 takes a JSON body, not form-encoded)", gotCT)
	}
	if gotBody["client_id"] != "id@example.com" || gotBody["client_secret"] != "shhh" {
		t.Fatalf("token body = %v, want client_id+client_secret", gotBody)
	}
	if gotBody["grant_type"] != nil {
		t.Fatalf("fresh grant should not send grant_type, got %v", gotBody["grant_type"])
	}
	// Token cached on the config for reuse.
	if cfg.AccessToken != "tok-abc" || cfg.RefreshToken != "ref-xyz" {
		t.Fatalf("tokens not cached: access=%q refresh=%q", cfg.AccessToken, cfg.RefreshToken)
	}
	if cfg.TokenExpiry.IsZero() {
		t.Fatalf("token expiry not recorded")
	}
}

// An expired access token with a refresh token re-mints using the
// refresh_token grant shape.
func TestMintClientCredentials_RefreshGrant(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"tok-new","refresh_token":"ref-new","expires_in":3600,"token_type":"bearer"}`))
	}))
	defer srv.Close()

	cfg := &config.Config{
		BaseURL:      srv.URL,
		ClientID:     "id@example.com",
		ClientSecret: "shhh",
		AccessToken:  "tok-old",
		RefreshToken: "ref-old",
		TokenExpiry:  time.Now().Add(-time.Minute), // expired
		Path:         filepath.Join(t.TempDir(), "config.toml"),
	}
	c := New(cfg, 10*time.Second, 0)

	h, err := c.authHeader(context.Background())
	if err != nil {
		t.Fatalf("authHeader error: %v", err)
	}
	if h != "Bearer tok-new" {
		t.Fatalf("auth header = %q, want Bearer tok-new", h)
	}
	if gotBody["grant_type"] != "refresh_token" || gotBody["refresh_token"] != "ref-old" {
		t.Fatalf("refresh body = %v, want grant_type=refresh_token refresh_token=ref-old", gotBody)
	}
}

// A failed mint must never reflect the posted client_secret into the returned
// error, even when a misconfigured/echoing endpoint sends the request body
// back. Only a charset-validated RFC-6749 error code may survive.
func TestMintClientCredentials_FailureDoesNotEchoSecret(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		// Echo the request body back, the worst-case reflection.
		_, _ = w.Write([]byte(`{"error":"invalid_client","error_description":"bad request: ` + string(raw) + `"}`))
	}))
	defer srv.Close()

	cfg := &config.Config{
		BaseURL:      srv.URL,
		ClientID:     "api-key@example.com",
		ClientSecret: "super-secret-value-9911",
		Path:         filepath.Join(t.TempDir(), "config.toml"),
	}
	c := &Client{BaseURL: srv.URL, Config: cfg, HTTPClient: srv.Client()}

	_, err := c.mintClientCredentials(context.Background())
	if err == nil {
		t.Fatal("expected error from 400 token endpoint")
	}
	if msg := err.Error(); strings.Contains(msg, "super-secret-value-9911") {
		t.Fatalf("client_secret leaked into mint error: %s", msg)
	}
	// The echoed body is invalid JSON (unescaped reflection), so no error code
	// can be extracted — dropping it entirely is the correct defensive outcome.
	if !strings.Contains(err.Error(), "HTTP 400") {
		t.Fatalf("expected status code in mint error, got: %s", err.Error())
	}
}

func TestTokenErrCode_RejectsUnsafeCodes(t *testing.T) {
	cases := map[string]string{
		`{"error":"invalid_client"}`:        " (invalid_client)",
		`{"error":"has spaces and secret"}`: "",
		`{"error":""}`:                      "",
		`not json`:                          "",
	}
	for in, want := range cases {
		if got := tokenErrCode([]byte(in)); got != want {
			t.Errorf("tokenErrCode(%s) = %q, want %q", in, got, want)
		}
	}
}
