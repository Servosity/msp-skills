// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written (NOT generated): the runtime proof behind LEDGER-NOTES "mint_lock_scan".
// fleet/bin/mint_lock_scan.py flags doInternal/authHeader -> mintClientCredentials and
// refreshAccessToken -> maskHeldError on this press shape; the mint masks with held=true
// and never re-takes lockTokenState. These tests drive a REJECTED client_credentials mint
// on both entry paths and fail if either hangs.

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

	"connectwise-rmm-pp-cli/internal/cliutil"
	"connectwise-rmm-pp-cli/internal/cliutil/testenv"
	"connectwise-rmm-pp-cli/internal/config"
)

func rejectedMintClient(t *testing.T, apiStatus int, accessToken string) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/token" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprint(w, `{"error":"invalid_client","client_secret":"test-client-secret"}`)
			return
		}
		w.WriteHeader(apiStatus)
		fmt.Fprint(w, `{"companies":[]}`)
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
		TokenURL:     srv.URL + "/v1/token",
		ClientID:     "test-client-id",
		ClientSecret: "test-client-secret",
		AccessToken:  accessToken,
		Path:         filepath.Join(home, "config.toml"),
	}
	if accessToken != "" {
		cfg.TokenExpiry = time.Now().Add(time.Hour)
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
		_, err := c.Get(context.Background(), "/api/platform/v1/company/companies", nil)
		done <- err
	}()
	select {
	case err := <-done:
		return err
	case <-time.After(limit):
		t.Fatalf("a rejected token mint did not return within %s: the token-state lock deadlocked", limit)
		return nil
	}
}

// Path 1: no cached token, so authHeader mints before the call.
func TestRejectedMintBeforeCallReturns(t *testing.T) {
	c := rejectedMintClient(t, http.StatusOK, "")
	err := getWithin(t, c, 10*time.Second)
	if err == nil || !strings.Contains(err.Error(), "HTTP 401") {
		t.Fatalf("want the mint's HTTP 401, got %v", err)
	}
	if strings.Contains(err.Error(), "test-client-secret") {
		t.Fatalf("the rejected mint echoed the client secret: %v", err)
	}
}

// Path 2: the API answers 401, so doInternal re-mints after the response.
func TestRejectedMintAfter401Returns(t *testing.T) {
	c := rejectedMintClient(t, http.StatusUnauthorized, "cached-token")
	err := getWithin(t, c, 10*time.Second)
	if err == nil {
		t.Fatal("a 401 with a rejected re-mint reported success")
	}
	if strings.Contains(err.Error(), "test-client-secret") {
		t.Fatalf("the rejected re-mint echoed the client secret: %v", err)
	}
}
