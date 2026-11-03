// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"threatdown-pp-cli/internal/config"
)

// TestRejectedMintAndRefreshNeverDeadlock: a token endpoint that rejects the
// client credentials (mint) or the refresh token must return an error, not hang.
// mint_lock_scan.py flags both paths because they run under lockTokenState and
// call maskHeldError; maskHeldError passes held=true and never takes the lock
// again. This test is the runtime proof (it hangs if that ever changes).
func TestRejectedMintAndRefreshNeverDeadlock(t *testing.T) {
	home := t.TempDir()
	for _, k := range []string{"HOME", "USERPROFILE", "XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME", "XDG_CACHE_HOME", "THREATDOWN_HOME"} {
		t.Setenv(k, home)
	}
	t.Setenv("PRINTING_PRESS_VERIFY", "")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"error":"invalid_client","access_token":"rejected-echo-token-value"}`)
	}))
	defer srv.Close()
	within := func(name string, f func() error) error {
		done := make(chan error, 1)
		go func() { done <- f() }()
		select {
		case err := <-done:
			return err
		case <-time.After(5 * time.Second):
			t.Fatalf("%s: a rejected token request hung (token-state lock taken twice)", name)
			return nil
		}
	}
	mintCfg := &config.Config{Path: filepath.Join(home, "config.toml"), BaseURL: srv.URL, TokenURL: srv.URL + "/token",
		ClientID: "stub-client-id", ClientSecret: "stub-client-secret"}
	c := New(mintCfg, 5*time.Second, 0)
	err := within("mint", func() error {
		_, _, err := c.do(context.Background(), http.MethodGet, "/v1/sites", nil, nil, nil)
		return err
	})
	if !errors.Is(err, ErrTokenMintRejected) {
		t.Fatalf("rejected mint: want ErrTokenMintRejected, got %v", err)
	}
	if strings.Contains(err.Error(), "rejected-echo-token-value") {
		t.Fatalf("rejected mint error echoes the token body: %v", err)
	}
	refreshCfg := &config.Config{Path: filepath.Join(home, "config2.toml"), BaseURL: srv.URL, TokenURL: srv.URL + "/token",
		ClientID: "stub-client-id", AccessToken: "old-access", RefreshToken: "stub-refresh-token"}
	r := New(refreshCfg, 5*time.Second, 0)
	if err := within("refresh", func() error { return r.refreshAccessToken(context.Background()) }); err == nil {
		t.Fatal("rejected refresh returned no error")
	}
}
