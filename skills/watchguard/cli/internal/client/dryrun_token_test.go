// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"watchguard-pp-cli/internal/config"
)

// hand-fix (handfixes.json dry-run-never-mints): every path that can call the
// token endpoint (own mint, refresh, audience token, audience lookup) refuses
// in a --dry-run client before dialing.
func TestDryRunClientNeverCallsTheTokenEndpoint(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"tok","expires_in":3600,"audience":"aud"}`))
	}))
	defer srv.Close()
	t.Setenv("WATCHGUARD_CLIENT_ID", "")
	t.Setenv("WATCHGUARD_CLIENT_SECRET", "")
	c := New(&config.Config{BaseURL: srv.URL, TokenURL: srv.URL + "/oauth/token", ClientID: "RW", ClientSecret: "rwsec", RefreshToken: "r"}, 5*time.Second, 0)
	c.DryRun = true
	ctx := context.Background()
	if err := c.mintClientCredentials(ctx, "RW", "rwsec"); !errors.Is(err, ErrDryRunNoTokenRequest) {
		t.Errorf("mint: %v", err)
	}
	if err := c.refreshAccessToken(ctx); !errors.Is(err, ErrDryRunNoTokenRequest) {
		t.Errorf("refresh: %v", err)
	}
	if _, _, err := c.ScopedBearer(ctx, "aud-x"); !errors.Is(err, ErrDryRunNoTokenRequest) {
		t.Errorf("audience token: %v", err)
	}
	if _, err := c.AudienceFor(ctx, "ACC-B"); !errors.Is(err, ErrDryRunNoTokenRequest) {
		t.Errorf("audience lookup: %v", err)
	}
	if hits != 0 {
		t.Fatalf("a dry-run client made %d requests", hits)
	}
}
