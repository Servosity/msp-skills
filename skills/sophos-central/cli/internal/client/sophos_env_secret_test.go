// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/sophos-central/handfixes.json
// under the "env-client-secret-not-persisted" entry.

package client

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"sophos-central-pp-cli/internal/config"
)

// TestMintDoesNotPersistEnvSuppliedSecret: an automatic token mint from
// SOPHOS_CENTRAL_CLIENT_SECRET caches the token but never writes the secret.
func TestMintDoesNotPersistEnvSuppliedSecret(t *testing.T) {
	const secret = "ENV-SECRET-SENTINEL-77"
	home := t.TempDir()
	t.Setenv("SOPHOS_CENTRAL_HOME", home)
	t.Setenv("SOPHOS_CENTRAL_CLIENT_ID", "env-client-id-123")
	t.Setenv("SOPHOS_CENTRAL_CLIENT_SECRET", secret)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/token") {
			_, _ = io.WriteString(w, `{"access_token":"minted-token-marker","expires_in":3600}`)
			return
		}
		_, _ = io.WriteString(w, `{"id":"t1","idType":"tenant"}`)
	}))
	defer srv.Close()
	t.Setenv("SOPHOS_CENTRAL_TOKEN_URL", srv.URL+"/token")
	t.Setenv("SOPHOS_CENTRAL_BASE_URL", srv.URL)
	cfg, err := config.Load("")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	c := New(cfg, 5*time.Second, 0)
	if _, err := c.GetNoCache(context.Background(), "/whoami/v1", nil); err != nil {
		t.Fatalf("GET: %v", err)
	}
	sawToken := false
	_ = filepath.WalkDir(home, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		b, _ := os.ReadFile(path)
		if strings.Contains(string(b), secret) {
			t.Fatalf("env-supplied client secret persisted to %s", path)
		}
		if strings.Contains(string(b), "minted-token-marker") {
			sawToken = true
		}
		return nil
	})
	if !sawToken {
		t.Fatal("the minted token was not cached; later runs would re-mint on every call")
	}
}
