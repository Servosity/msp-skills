// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"threatdown-pp-cli/internal/config"
)

// hand-fix (handfixes.json token-bound-to-client): a hand-written auth_header
// is an unbound token, so it yields to a configured client exactly like a
// set-token token (Codex review round 6).
func TestHandWrittenAuthHeaderYieldsToClient(t *testing.T) {
	t.Setenv("THREATDOWN_OAUTH_SCOPE", "")
	t.Setenv("THREATDOWN_CLIENT_ID", "")
	t.Setenv("THREATDOWN_CLIENT_SECRET", "")
	c := &Client{Config: &config.Config{AuthHeaderVal: "Bearer admin-token", ClientID: "reporting-client"}}
	if _, err := c.authHeader(context.Background()); !errors.Is(err, ErrTokenBoundElsewhere) {
		t.Fatalf("auth_header rode a client ID with no secret: %v", err)
	}
	c = &Client{Config: &config.Config{AuthHeaderVal: "Bearer admin-token"}}
	if h, err := c.authHeader(context.Background()); err != nil || h != "Bearer admin-token" {
		t.Fatalf("auth_header unusable with no client configured: %q %v", h, err)
	}
}

// A [headers] Authorization entry never replaces the bound token on the wire.
func TestConfiguredHeadersNeverCarryAuthorization(t *testing.T) {
	t.Setenv("THREATDOWN_OAUTH_SCOPE", "")
	t.Setenv("THREATDOWN_CLIENT_ID", "")
	t.Setenv("THREATDOWN_CLIENT_SECRET", "")
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	c := New(&config.Config{
		BaseURL:     srv.URL,
		AccessToken: "own-token",
		TokenExpiry: time.Now().Add(time.Hour),
		Headers:     map[string]string{"authorization": "Bearer admin-token", "X-Extra": "1"},
	}, 5*time.Second, 0)
	c.NoCache = true
	if _, err := c.Get(context.Background(), "/v1/info/account", nil); err != nil {
		t.Fatal(err)
	}
	if got != "Bearer own-token" {
		t.Fatalf("Authorization on the wire = %q, want the bound token", got)
	}
}
