// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"action1-pp-cli/internal/cliutil"
	"action1-pp-cli/internal/config"
)

// Codex r1 (action1 reprint on 4.34.2) class cached authentication ignores
// credential override: the credential store holds client A's pair and the
// token minted for it; the environment names client B. A request for B must
// carry a token minted for B (A's token would return A's organizations).
// Without the environment pair, A's stored token is reused and nothing mints.
func TestStoredTokenOfAnotherClientIsNeverSent(t *testing.T) {
	isolateCredentialStore(t)
	var mints atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		mints.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"token-for-`+body["client_id"].(string)+`","expires_in":3600}`)
	}))
	defer srv.Close()
	t.Setenv("ACTION1_BASE_URL", srv.URL)
	store, err := cliutil.CredentialsFilePath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(store), 0o700); err != nil {
		t.Fatal(err)
	}
	stored := "base_url = " + strconv.Quote(srv.URL) + "\nclient_id = \"client-a-4f1c\"\nclient_secret = \"secret-a-9d2e\"\n" +
		"access_token = \"token-for-client-a-4f1c\"\ntoken_expiry = " + time.Now().Add(time.Hour).UTC().Format(time.RFC3339) + "\n"
	if err := os.WriteFile(store, []byte(stored), 0o600); err != nil {
		t.Fatal(err)
	}
	header := func() string {
		t.Helper()
		cfg, err := config.Load("")
		if err != nil {
			t.Fatal(err)
		}
		h, err := New(cfg, 5*time.Second, 0).authHeader(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		return h
	}
	if h := header(); h != "Bearer token-for-client-a-4f1c" || mints.Load() != 0 {
		t.Fatalf("the stored client's own token: got %q after %d mints, want it reused", h, mints.Load())
	}
	t.Setenv("ACTION1_CLIENT_ID", "client-b-7e3a")
	t.Setenv("ACTION1_CLIENT_SECRET", "secret-b-1b8f")
	if h := header(); h != "Bearer token-for-client-b-7e3a" {
		t.Fatalf("the environment names client B; the request carried %q (client A's stored token authenticates as A)", h)
	}
}
