// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"sophos-central-pp-cli/internal/cliutil"
	"sophos-central-pp-cli/internal/cliutil/testenv"
	"sophos-central-pp-cli/internal/config"
)

// A rejected token request must come back as an error, never hang: the error
// text is masked while the token-state lock is held, and that mutex is not
// reentrant (the jamf-pro / threecx deadlock, mint_lock_scan.py). Covers the
// pre-request mint, the mint after an API 401, and the refresh grant.
func TestRejectedTokenRequestReturnsInsteadOfHanging(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Both the token endpoint and the API reject, echoing the secret.
		http.Error(w, `{"error":"invalid_client","detail":"test-client-secret"}`, http.StatusUnauthorized)
	}))
	defer srv.Close()

	restore, err := cliutil.SetHomeOverride("")
	if err != nil {
		t.Fatalf("reset home override: %v", err)
	}
	t.Cleanup(restore)
	home := testenv.Isolate(t, cliutil.ConfigDir, cliutil.DataDir, cliutil.StateDir, cliutil.CacheDir)
	t.Setenv("LOCALAPPDATA", filepath.Join(home, "localapp"))
	t.Setenv("APPDATA", filepath.Join(home, "appdata"))

	newClient := func(cfg config.Config) *Client {
		cfg.BaseURL = srv.URL
		cfg.TokenURL = srv.URL + "/token"
		cfg.ClientID = "test-client-id"
		cfg.ClientSecret = "test-client-secret"
		cfg.Path = filepath.Join(home, "config.toml")
		c := New(&cfg, 5*time.Second, 0)
		c.HTTPClient = srv.Client()
		c.NoCache = true
		return c
	}
	within := func(name string, f func() error) {
		t.Helper()
		done := make(chan error, 1)
		go func() { done <- f() }()
		select {
		case err := <-done:
			if err == nil {
				t.Fatalf("%s: a rejected token request returned no error", name)
			}
		case <-time.After(10 * time.Second):
			t.Fatalf("%s: a rejected token request hung (token-state lock re-entered)", name)
		}
	}

	within("mint before the request", func() error {
		_, err := newClient(config.Config{}).Get(context.Background(), "/whoami", nil)
		return err
	})
	within("mint after an API 401", func() error {
		c := newClient(config.Config{AccessToken: "live-token", TokenExpiry: time.Now().Add(time.Hour)})
		_, err := c.Get(context.Background(), "/whoami", nil)
		return err
	})
	within("refresh grant", func() error {
		c := newClient(config.Config{AccessToken: "live-token", RefreshToken: "refresh-token", TokenExpiry: time.Now().Add(time.Hour)})
		return c.refreshAccessToken(context.Background())
	})
}
