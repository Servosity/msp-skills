// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"watchguard-pp-cli/internal/config"
)

// hand-fix (handfixes.json token-bound-to-client): a hand-written auth_header
// is an unbound token, so it yields to a configured client exactly like a
// set-token token.
func TestHandWrittenAuthHeaderYieldsToClient(t *testing.T) {
	t.Setenv("WATCHGUARD_OAUTH_SCOPE", "")
	t.Setenv("WATCHGUARD_CLIENT_ID", "")
	t.Setenv("WATCHGUARD_CLIENT_SECRET", "")
	c := &Client{Config: &config.Config{AuthHeaderVal: "Bearer rw-token", ClientID: "ro-access-id"}}
	if _, err := c.authHeader(context.Background()); !errors.Is(err, ErrTokenBoundElsewhere) {
		t.Fatalf("auth_header rode a client ID with no secret: %v", err)
	}
	c = &Client{Config: &config.Config{AuthHeaderVal: "Bearer rw-token"}}
	if h, err := c.authHeader(context.Background()); err != nil || h != "Bearer rw-token" {
		t.Fatalf("auth_header unusable with no client configured: %q %v", h, err)
	}
}

// A [headers] Authorization entry never replaces the bound token on the wire,
// while the WatchGuard-API-Key sibling credential and other headers still go.
func TestConfiguredHeadersNeverCarryAuthorization(t *testing.T) {
	t.Setenv("WATCHGUARD_OAUTH_SCOPE", "")
	t.Setenv("WATCHGUARD_CLIENT_ID", "")
	t.Setenv("WATCHGUARD_CLIENT_SECRET", "")
	var got, key, extra string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("Authorization")
		key = r.Header.Get("WatchGuard-API-Key")
		extra = r.Header.Get("X-Extra")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	c := New(&config.Config{
		BaseURL:          srv.URL,
		AccessToken:      "own-token",
		TokenExpiry:      time.Now().Add(time.Hour),
		WatchguardApiKey: "KEY",
		Headers:          map[string]string{"authorization": "Bearer rw-token", "X-Extra": "1"},
	}, 5*time.Second, 0)
	c.NoCache = true
	if _, err := c.Get(context.Background(), "/rest/probe", nil); err != nil {
		t.Fatal(err)
	}
	if got != "Bearer own-token" {
		t.Fatalf("Authorization on the wire = %q, want the bound token", got)
	}
	if key != "KEY" || extra != "1" {
		t.Fatalf("WatchGuard-API-Key=%q X-Extra=%q, want both still sent", key, extra)
	}
}

// Adversarial review round 11 (threatdown): a placeholder from the
// environment was reported as "configured in config.toml", a file that held
// no credential and that auth login cannot override.
func TestPlaceholderErrorNamesTheEnvVar(t *testing.T) {
	cfg := &config.Config{Path: "/tmp/home/config.toml"}
	t.Setenv("WATCHGUARD_API_KEY", "")
	t.Setenv("WATCHGUARD_CLIENT_ID", "rw-access-id")
	t.Setenv("WATCHGUARD_CLIENT_SECRET", "<your-client-secret>")
	err := authPlaceholderCredentialError(cfg)
	if !errors.Is(err, ErrPlaceholderCredential) || !strings.Contains(err.Error(), "$WATCHGUARD_CLIENT_SECRET") || strings.Contains(err.Error(), "config.toml") {
		t.Fatalf("env placeholder must name the variable, got %v", err)
	}
	t.Setenv("WATCHGUARD_CLIENT_SECRET", "")
	t.Setenv("WATCHGUARD_API_KEY", "<your-token>")
	err = authPlaceholderCredentialErrorWithSetup(cfg, "export WATCHGUARD_API_KEY=<your-token>")
	if !strings.Contains(err.Error(), "$WATCHGUARD_API_KEY") || strings.Contains(err.Error(), "config.toml") {
		t.Fatalf("env API-key placeholder must name the variable, got %v", err)
	}
	t.Setenv("WATCHGUARD_API_KEY", "")
	if err := authPlaceholderCredentialError(cfg); !strings.Contains(err.Error(), "config.toml") {
		t.Fatalf("a file placeholder still names the file, got %v", err)
	}
}

// Any <...> value is a doc placeholder, including the ones this CLI's own
// auth status prints (<client-id>, <client-secret>); a real value is not.
func TestAnglePlaceholdersAreNotCredentials(t *testing.T) {
	for _, v := range []string{"<client-id>", "<client-secret>", "<value>", "<YOUR_KEY>"} {
		if !LooksLikeCredentialPlaceholder(v) {
			t.Errorf("%q should be a placeholder", v)
		}
	}
	for _, v := range []string{"a1b2c3", "eyJhbGciOi.payload.sig", "Bearer abc"} {
		if LooksLikeCredentialPlaceholder(v) {
			t.Errorf("%q is a real-shaped value, not a placeholder", v)
		}
	}
}
