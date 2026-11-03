// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written (NOT generated): the runtime proof behind LEDGER-NOTES "mint_lock_scan".
// fleet/bin/mint_lock_scan.py flags refreshAccessToken -> maskHeldError on this press
// shape; maskHeldError masks with held=true and never re-takes lockTokenState. These
// tests drive a REJECTED refresh on both entry paths and fail if either hangs.

package client

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"xero-pp-cli/internal/cliutil"
	"xero-pp-cli/internal/cliutil/testenv"
	"xero-pp-cli/internal/config"
)

func rejectedRefreshClient(t *testing.T, apiStatus int, expiry time.Time) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `{"error":"invalid_grant"}`)
			return
		}
		w.WriteHeader(apiStatus)
		fmt.Fprint(w, `{"Accounts":[]}`)
	}))
	t.Cleanup(srv.Close)
	restore, err := cliutil.SetHomeOverride("")
	if err != nil {
		t.Fatalf("reset home override: %v", err)
	}
	t.Cleanup(restore)
	home := testenv.Isolate(t, cliutil.ConfigDir, cliutil.DataDir, cliutil.StateDir, cliutil.CacheDir)
	t.Setenv("LOCALAPPDATA", filepath.Join(home, "localapp"))
	t.Setenv("APPDATA", filepath.Join(home, "appdata"))
	cfg := &config.Config{
		BaseURL:      srv.URL,
		TokenURL:     srv.URL + "/token",
		ClientID:     "test-client-id",
		ClientSecret: "test-client-secret",
		AccessToken:  "old-token",
		RefreshToken: "spent-refresh",
		TokenExpiry:  expiry,
		Path:         filepath.Join(home, "config.toml"),
	}
	c := New(cfg, 5*time.Second, 0)
	c.HTTPClient = srv.Client()
	c.NoCache = true
	return c
}

func getWithin(t *testing.T, c *Client, limit time.Duration) error {
	t.Helper()
	done := make(chan error, 1)
	go func() {
		_, err := c.Get(context.Background(), "/Accounts", nil)
		done <- err
	}()
	select {
	case err := <-done:
		return err
	case <-time.After(limit):
		t.Fatalf("a rejected token refresh did not return within %s: the token-state lock deadlocked", limit)
		return nil
	}
}

// Path 1: the token is near expiry, so authHeader refreshes before the call.
func TestRejectedRefreshBeforeCallReturns(t *testing.T) {
	c := rejectedRefreshClient(t, http.StatusOK, time.Now().Add(-time.Minute))
	err := getWithin(t, c, 10*time.Second)
	if err == nil || !strings.Contains(err.Error(), "refresh") {
		t.Fatalf("want a refresh error, got %v", err)
	}
}

// Path 2: the API answers 401, so doInternal refreshes after the response.
func TestRejectedRefreshAfter401Returns(t *testing.T) {
	c := rejectedRefreshClient(t, http.StatusUnauthorized, time.Now().Add(time.Hour))
	if err := getWithin(t, c, 10*time.Second); err == nil {
		t.Fatal("a 401 with a rejected refresh reported success")
	}
}
