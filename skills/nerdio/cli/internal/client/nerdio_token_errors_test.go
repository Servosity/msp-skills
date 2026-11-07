// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

// nerdio handfix: rejected-mint-returns. The token exchange runs with the
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
	"sync/atomic"
	"testing"
	"time"

	"nerdio-pp-cli/internal/cliutil"
	"nerdio-pp-cli/internal/cliutil/testenv"
	"nerdio-pp-cli/internal/config"
)

// The token endpoint (an Entra tenant, another host than the NMM instance)
// refuses the client and repeats the pair it was sent; the mint must return
// promptly, the error must not repeat the secret, and the token endpoint
// must really have been asked (a mint refused before it posts, for want of
// NERDIO_OAUTH_SCOPE or NERDIO_TOKEN_URL, would prove nothing here).
func TestRejectedMintReturnsWithoutTheSecret(t *testing.T) {
	const id, secret = "rejUnitClientId-6a2c8e4b", "rejUnitSecret-3f7b1d9a5c"
	var tokenRequests atomic.Int32
	entra := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokenRequests.Add(1)
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprintf(w, `{"error":"invalid_client","error_description":"AADSTS7000215: unknown client %s / %s"}`, id, secret)
	}))
	defer entra.Close()
	nmm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"error":"invalid_token"}`)
	}))
	defer nmm.Close()
	restore, err := cliutil.SetHomeOverride("")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(restore)
	home := testenv.Isolate(t, cliutil.ConfigDir, cliutil.DataDir, cliutil.StateDir, cliutil.CacheDir)
	t.Setenv("LOCALAPPDATA", filepath.Join(home, "localapp"))
	t.Setenv("APPDATA", filepath.Join(home, "appdata"))
	for _, stale := range []bool{false, true} {
		cfg := &config.Config{BaseURL: nmm.URL, TokenURL: entra.URL + "/rej-tenant/oauth2/v2.0/token", ClientID: id, ClientSecret: secret, Path: filepath.Join(home, "config.toml")}
		if stale {
			// A stored, unexpired token the API rejects: the 401 recovery re-mints under the lock.
			cfg.AccessToken, cfg.TokenExpiry = "staleUnitToken-1e5c9a3f", time.Now().Add(time.Hour)
		}
		c := New(cfg, 5*time.Second, 0)
		c.HTTPClient = nmm.Client()
		before := tokenRequests.Load()
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
			if tokenRequests.Load() == before {
				t.Fatalf("stale=%v: the token endpoint was never asked (%v); the rejected-mint path did not run", stale, err)
			}
		case <-time.After(15 * time.Second):
			t.Fatalf("stale=%v: a rejected token mint hung (the token-state lock is taken twice)", stale)
		}
	}
}
