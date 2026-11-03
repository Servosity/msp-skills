// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"watchguard-pp-cli/internal/cliutil/testenv"
	"watchguard-pp-cli/internal/config"
)

// hand-fix (handfixes.json token-bound-to-client): a cached token minted for
// one OAuth client (Access ID), scope or token URL must not be reused by a
// process configured with another.
func TestCachedTokenBoundToClientAndScope(t *testing.T) {
	t.Setenv("WATCHGUARD_OAUTH_SCOPE", "")
	t.Setenv("WATCHGUARD_CLIENT_ID", "")
	t.Setenv("WATCHGUARD_CLIENT_SECRET", "")
	minted := config.TokenBindingFor("rw-access-id", resolveClientCredentialsScope(), effectiveTokenURL(nil))
	newClient := func(id, tokenURL string) *Client {
		return &Client{Config: &config.Config{
			AccessToken:  "cached-rw-token",
			TokenExpiry:  time.Now().Add(time.Hour),
			ClientID:     id,
			ClientSecret: "s",
			TokenURL:     tokenURL,
			TokenBinding: minted,
		}}
	}
	if newClient("rw-access-id", "").needsClientCredentialsMint() {
		t.Fatal("same client, same scope: cached token should be reused")
	}
	if !newClient("ro-access-id", "").needsClientCredentialsMint() {
		t.Fatal("read-only Access ID reused the read-write Access ID's cached token")
	}
	if !newClient("rw-access-id", "https://other.example/oauth/token").needsClientCredentialsMint() {
		t.Fatal("a different token URL reused a token minted at the default token URL")
	}
	t.Setenv("WATCHGUARD_OAUTH_SCOPE", "narrower")
	if !newClient("rw-access-id", "").needsClientCredentialsMint() {
		t.Fatal("a different WATCHGUARD_OAUTH_SCOPE reused a token minted with api-access")
	}
}

// The binding fails closed: a process that cannot mint (no secret, or no
// client credentials at all) never falls back to another client's token, on
// the request path or the response-cache path.
func TestCachedTokenBindingFailsClosed(t *testing.T) {
	t.Setenv("WATCHGUARD_OAUTH_SCOPE", "")
	t.Setenv("WATCHGUARD_CLIENT_ID", "")
	t.Setenv("WATCHGUARD_CLIENT_SECRET", "")
	minted := config.TokenBindingFor("rw-access-id", resolveClientCredentialsScope(), effectiveTokenURL(nil))
	cached := func(id, secret, binding string) *Client {
		return &Client{Config: &config.Config{
			AccessToken:  "cached-rw-token",
			TokenExpiry:  time.Now().Add(time.Hour),
			ClientID:     id,
			ClientSecret: secret,
			TokenBinding: binding,
		}}
	}
	ctx := context.Background()
	if _, err := cached("ro-access-id", "", minted).authHeader(ctx); !errors.Is(err, ErrTokenBoundElsewhere) {
		t.Fatalf("client ID without a secret rode another client's cached token: %v", err)
	}
	if err := cached("ro-access-id", "", minted).validateCachedRequestAuth(ctx); err == nil {
		t.Fatal("cache path accepted another client's cached token")
	}
	t.Setenv("WATCHGUARD_OAUTH_SCOPE", "narrower")
	if _, err := cached("", "", minted).authHeader(ctx); err == nil {
		t.Fatal("WATCHGUARD_OAUTH_SCOPE=narrower with no client credentials rode the api-access token")
	}
	t.Setenv("WATCHGUARD_OAUTH_SCOPE", "")
	if h, err := cached("rw-access-id ", "", minted).authHeader(ctx); err != nil || h != "Bearer cached-rw-token" {
		t.Fatalf("same client (padded ID) lost its own token: %q %v", h, err)
	}
	if !cached("rw-access-id", "s", "").needsClientCredentialsMint() {
		t.Fatal("unbound token outranked configured client credentials")
	}
	if h, err := cached("", "", "").authHeader(ctx); err != nil || h != "Bearer cached-rw-token" {
		t.Fatalf("auth set-token token unusable with no client credentials: %q %v", h, err)
	}
}

// An auth set-token token is used only while no client ID or secret is set,
// and the refusal is the typed auth-class error.
func TestSetTokenYieldsToAnyClientCredential(t *testing.T) {
	t.Setenv("WATCHGUARD_OAUTH_SCOPE", "")
	t.Setenv("WATCHGUARD_CLIENT_ID", "")
	t.Setenv("WATCHGUARD_CLIENT_SECRET", "")
	c := &Client{Config: &config.Config{AccessToken: "pasted-token", TokenExpiry: time.Now().Add(time.Hour), ClientID: "ro-access-id"}}
	if _, err := c.authHeader(context.Background()); !errors.Is(err, ErrTokenBoundElsewhere) {
		t.Fatalf("set-token token rode a client ID with no secret: %v", err)
	}
}

// The onboarding receipt, end to end through config.Load and the shared
// credentials file: a process run with the read-write Access ID mints and
// caches a token; a later process run with the read-only Access ID and its own
// API password must mint its own token, and the API must see that token.
func TestReadOnlyAccessIDNeverRidesReadWriteToken(t *testing.T) {
	testenv.Isolate(t)
	t.Setenv("WATCHGUARD_OAUTH_SCOPE", "")
	t.Setenv("WATCHGUARD_AUDIENCE", "")
	t.Setenv("WATCHGUARD_API_KEY", "")
	var mu sync.Mutex
	var mints, bearers []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/oauth/token" {
			raw, _ := base64.StdEncoding.DecodeString(strings.TrimPrefix(r.Header.Get("Authorization"), "Basic "))
			id, _, _ := strings.Cut(string(raw), ":")
			mu.Lock()
			mints = append(mints, id)
			mu.Unlock()
			_, _ = w.Write([]byte(`{"access_token":"tok-` + id + `","expires_in":3600}`))
			return
		}
		mu.Lock()
		bearers = append(bearers, r.Header.Get("Authorization"))
		mu.Unlock()
		_, _ = w.Write([]byte(`{"objects":[]}`))
	}))
	defer srv.Close()
	t.Setenv("WATCHGUARD_BASE_URL", srv.URL)
	t.Setenv("WATCHGUARD_TOKEN_URL", srv.URL+"/oauth/token")

	read := func(id, secret string) error {
		t.Setenv("WATCHGUARD_CLIENT_ID", id)
		t.Setenv("WATCHGUARD_CLIENT_SECRET", secret)
		cfg, err := config.Load("")
		if err != nil {
			t.Fatal(err)
		}
		c := New(cfg, 5*time.Second, 0)
		c.NoCache = true
		_, err = c.Get(context.Background(), "/rest/probe", nil)
		return err
	}
	if err := read("RW", "rwsecret"); err != nil {
		t.Fatal(err)
	}
	if err := read("RW", "rwsecret"); err != nil {
		t.Fatal(err)
	}
	if len(mints) != 1 {
		t.Fatalf("token requests %v: the same Access ID must reuse its own cached token", mints)
	}
	if err := read("RO", "other"); err != nil {
		t.Fatal(err)
	}
	if len(mints) != 2 || mints[1] != "RO" {
		t.Fatalf("token requests by Access ID = %v, want [RW RO]: the read-only setup made no token request of its own", mints)
	}
	if bearers[2] != "Bearer tok-RO" {
		t.Fatalf("read-only call sent %q, want its own token", bearers[2])
	}
	// The read-only Access ID with no API password cannot mint, so it must
	// stop rather than send the read-write token that is cached now.
	if err := read("RW", "rwsecret"); err != nil {
		t.Fatal(err)
	}
	if err := read("RO", ""); !errors.Is(err, ErrTokenBoundElsewhere) {
		t.Fatalf("read-only Access ID without a secret: err=%v, want ErrTokenBoundElsewhere", err)
	}
	if got := bearers[len(bearers)-1]; got != "Bearer tok-RW" || len(bearers) != 4 {
		t.Fatalf("API calls %v: the refused process must send nothing", bearers)
	}
}

// Verify mode's mock token belongs to this process, so a second call never
// reads it as bound elsewhere and dials a token endpoint.
func TestVerifyMockTokenNeverMints(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"tok","expires_in":3600}`))
	}))
	defer srv.Close()
	home := testenv.Isolate(t)
	t.Setenv("PRINTING_PRESS_VERIFY", "1")
	t.Setenv("PRINTING_PRESS_VERIFY_LIVE_HTTP", "")
	t.Setenv("WATCHGUARD_OAUTH_SCOPE", "")
	t.Setenv("WATCHGUARD_CLIENT_ID", "verify-id")
	t.Setenv("WATCHGUARD_CLIENT_SECRET", "verify-secret")
	c := &Client{Config: &config.Config{Path: filepath.Join(home, "config.toml"), TokenURL: srv.URL + "/oauth/token"}}
	for i := 0; i < 2; i++ {
		if h, err := c.authHeader(context.Background()); err != nil || h != "Bearer mock-token-for-testing" {
			t.Fatalf("call %d: %q %v", i, h, err)
		}
	}
	if hits != 0 {
		t.Fatalf("verify mode dialed the token endpoint %d times", hits)
	}
}
