// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package client

// appdirect handfix: rejected-mint-returns. The token exchange runs with the
// token-state lock held; its error path masks the token endpoint's answer, and
// masking took that same lock, so a rejected mint hung forever (jamf-pro and
// threecx, 2026-10-27; fleet/bin/mint_lock_scan.py).

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"appdirect-pp-cli/internal/cliutil"
	"appdirect-pp-cli/internal/cliutil/testenv"
	"appdirect-pp-cli/internal/config"
)

func TestRejectedMintReturnsWithoutTheSecret(t *testing.T) {
	const id, secret = "rejUnitClientId-6a2c8e4b", "rejUnitSecret-3f7b1d9a5c"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/oauth/token" {
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprintf(w, `{"error":"access_denied","error_description":"unknown client %s / %s"}`, id, secret)
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"error":"invalid_token"}`)
	}))
	defer srv.Close()
	restore, err := cliutil.SetHomeOverride("")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(restore)
	home := testenv.Isolate(t, cliutil.ConfigDir, cliutil.DataDir, cliutil.StateDir, cliutil.CacheDir)
	t.Setenv("LOCALAPPDATA", filepath.Join(home, "localapp"))
	t.Setenv("APPDATA", filepath.Join(home, "appdata"))
	for _, stale := range []bool{false, true} {
		cfg := &config.Config{BaseURL: srv.URL, TokenURL: srv.URL + "/oauth/token", ClientID: id, ClientSecret: secret, Path: filepath.Join(home, "config.toml")}
		if stale {
			// A stored, unexpired token the API rejects: the 401 recovery re-mints under the lock.
			cfg.AccessToken, cfg.TokenExpiry = "staleUnitToken-1e5c9a3f", time.Now().Add(time.Hour)
		}
		c := New(cfg, 5*time.Second, 0)
		c.HTTPClient = srv.Client()
		c.NoCache = true
		done := make(chan error, 1)
		go func() {
			_, err := c.Get(context.Background(), "/companies", nil)
			done <- err
		}()
		select {
		case err := <-done:
			if err == nil {
				t.Fatalf("stale=%v: a rejected mint succeeded", stale)
			}
			if strings.Contains(err.Error(), secret) {
				t.Fatalf("stale=%v: the error repeats the client secret: %v", stale, err)
			}
		case <-time.After(15 * time.Second):
			t.Fatalf("stale=%v: a rejected token mint hung (the token-state lock is taken twice)", stale)
		}
	}
}
