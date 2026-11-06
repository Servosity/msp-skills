// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"jamf-pro-pp-cli/internal/cliutil"
	"jamf-pro-pp-cli/internal/cliutil/testenv"
	"jamf-pro-pp-cli/internal/config"
)

// TestRejectedMintReturns: a token endpoint that rejects the client (an
// error body that echoes the secret) fails the request; it never hangs on the
// token lock authHeader holds while minting. The real transport replaces such a
// body (tokenError) and the kit guards every output (internal/jamfblackbox).
func TestRejectedMintReturns(t *testing.T) {
	const secret = "rejected-client-secret-value"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprintf(w, `{"error":"invalid_client","echo":%q}`, base64.StdEncoding.EncodeToString([]byte(secret)))
	}))
	defer srv.Close()
	restore, err := cliutil.SetHomeOverride("")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(restore)
	home := testenv.Isolate(t, cliutil.ConfigDir, cliutil.DataDir, cliutil.StateDir, cliutil.CacheDir)
	cfg := &config.Config{
		BaseURL:      srv.URL,
		TokenURL:     srv.URL + "/token",
		ClientID:     "test-client-id",
		ClientSecret: secret,
		Path:         filepath.Join(home, "config.toml"),
	}
	c := New(cfg, 5*time.Second, 0)
	c.HTTPClient = srv.Client()
	done := make(chan error, 1)
	go func() {
		_, err := c.Get(context.Background(), "/probe", nil)
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("a rejected mint must fail the request")
		}
	case <-time.After(20 * time.Second):
		t.Fatal("a rejected mint hung (something waited on the token lock authHeader holds)")
	}
}
