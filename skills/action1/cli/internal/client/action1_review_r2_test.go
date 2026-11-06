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
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"action1-pp-cli/internal/cliutil"
	"action1-pp-cli/internal/config"
)

// Codex r2 (action1 reprint on 4.34.2) class cached authentication ignores
// credential override, --config form: the global credential store holds
// client A's pair and A's unexpired token; a separate 0600 --config file
// holds client B's pair and no token. The request must carry a token minted
// for B (A's token returns A's organizations), and minting for B must not
// overwrite A's record in the global store.
func TestR2ConfigFilePairNeverSendsTheGlobalRecordsToken(t *testing.T) {
	isolateCredentialStore(t)
	var mints atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		mints.Add(1)
		id, _ := body["client_id"].(string)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"token-for-`+id+`","expires_in":3600}`)
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
	stored := "base_url = " + strconv.Quote(srv.URL) + "\nclient_id = \"api-a@example.test\"\nclient_secret = \"secret-a-9d2e\"\n" +
		"access_token = \"token-for-api-a@example.test\"\ntoken_expiry = " + time.Now().Add(time.Hour).UTC().Format(time.RFC3339) + "\n"
	if err := os.WriteFile(store, []byte(stored), 0o600); err != nil {
		t.Fatal(err)
	}
	clientB := filepath.Join(t.TempDir(), "client-b.toml")
	if err := os.WriteFile(clientB, []byte("client_id = \"api-b@example.test\"\nclient_secret = \"secret-b-1b8f\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(clientB)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.StoredTokenOwned() {
		t.Fatalf("client B (from --config) owns the token client A's record supplied")
	}
	h, err := New(cfg, 5*time.Second, 0).authHeader(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if h != "Bearer token-for-api-b@example.test" || mints.Load() != 1 {
		t.Fatalf("--config names client B; the request carried %q after %d mints (client A's token authenticates as A)", h, mints.Load())
	}
	after, err := os.ReadFile(store)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(after), "api-a@example.test") || strings.Contains(string(after), "api-b@example.test") {
		t.Fatalf("minting for client B rewrote client A's global record:\n%s", after)
	}
}
