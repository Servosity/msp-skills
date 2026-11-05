// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"immybot-pp-cli/internal/cliutil"
	"immybot-pp-cli/internal/cliutil/testenv"
	"immybot-pp-cli/internal/config"
)

// TestRejectedMintReturnsInsteadOfDeadlocking: an Entra error from the token
// endpoint (a wrong or expired client secret) must come back as an error. The
// mint runs under the token-state lock, so its error body must be masked
// without taking that lock again.
func TestRejectedMintReturnsInsteadOfDeadlocking(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid_client","error_description":"AADSTS7000215: Invalid client secret provided for test-client-secret"}`))
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
	cfg := &config.Config{
		BaseURL:      srv.URL,
		TokenURL:     srv.URL + "/token",
		ClientID:     "test-client-id",
		ClientSecret: "test-client-secret",
		Path:         filepath.Join(home, "config.toml"),
	}
	c := New(cfg, 5*time.Second, 0)
	done := make(chan error, 1)
	go func() {
		_, err := c.authHeader(context.Background())
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("a rejected mint returned no error")
		}
		if strings.Contains(err.Error(), "test-client-secret") {
			t.Fatalf("the error shows the client secret: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("authHeader did not return: the rejected mint deadlocked on the token-state lock")
	}
}
