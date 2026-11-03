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

// A rejected client-credentials mint runs while authHeader holds the
// token-state lock (sync.Mutex, not reentrant), so its error masking must not
// take that lock again: the request has to fail with ErrTokenMintRejected, not
// hang. mint_lock_scan.py flags this path statically; this proves it returns.
func TestRejectedMintReturnsUnderTokenLock(t *testing.T) {
	t.Setenv("WATCHGUARD_CLIENT_ID", "")
	t.Setenv("WATCHGUARD_CLIENT_SECRET", "")
	t.Setenv("PRINTING_PRESS_VERIFY", "")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/oauth/token" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"invalid_client"}`))
			return
		}
		t.Errorf("API request %s sent without a token", r.URL.Path)
	}))
	defer srv.Close()
	t.Setenv("WATCHGUARD_BASE_URL", srv.URL)
	t.Setenv("WATCHGUARD_TOKEN_URL", srv.URL+"/oauth/token")
	c := New(&config.Config{BaseURL: srv.URL, TokenURL: srv.URL + "/oauth/token", ClientID: "AID", ClientSecret: "PW", WatchguardApiKey: "KEY"}, 5*time.Second, 0)
	done := make(chan error, 1)
	go func() {
		_, err := c.authHeader(context.Background())
		done <- err
	}()
	select {
	case err := <-done:
		if !errors.Is(err, ErrTokenMintRejected) {
			t.Fatalf("rejected mint error = %v, want ErrTokenMintRejected", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("authHeader hung on a rejected mint (token-state lock re-entered)")
	}
}
