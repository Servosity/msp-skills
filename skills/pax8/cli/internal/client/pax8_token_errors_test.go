// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package client

// pax8 handfix: rejected-mint-returns. The token exchange runs with the
// token-state lock held, so its error path must not take that lock again: a
// rejected mint hung forever when it did (jamf-pro and threecx, 2026-10-27;
// fleet/bin/mint_lock_scan.py). Its error shows the token endpoint's answer, and
// a held client secret in that answer is withheld at the sink by the kit.

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"pax8-pp-cli/internal/cliutil"
	"pax8-pp-cli/internal/cliutil/testenv"
	"pax8-pp-cli/internal/config"
	"pax8-pp-cli/internal/credkit"
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
			// The error as a sink receives it. Nothing else in this package
			// prints through the kit, so holding the secret here touches no other test.
			credkit.Hold("env:PAX8_CLIENT_SECRET", secret)
			var shown bytes.Buffer
			_, _ = credkit.NewWriter(&shown, credkit.SinkStderr).Write([]byte(err.Error()))
			if strings.Contains(shown.String(), secret) {
				t.Fatalf("stale=%v: the error reached a sink with the client secret in it: %s", stale, shown.String())
			}
		case <-time.After(15 * time.Second):
			t.Fatalf("stale=%v: a rejected token mint hung (the token-state lock is taken twice)", stale)
		}
	}
}
