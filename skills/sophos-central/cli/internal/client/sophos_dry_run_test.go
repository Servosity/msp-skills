// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/sophos-central/handfixes.json
// under the "dry-run-never-mints" entry.

package client

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"sophos-central-pp-cli/internal/config"
)

// TestDryRunNeverMintsAToken: a preview must not POST the client secret to
// the token URL. The generated order resolved the auth header (and minted)
// before the dry-run branch.
func TestDryRunNeverMintsAToken(t *testing.T) {
	t.Setenv("SOPHOS_CENTRAL_HOME", t.TempDir())
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		_, _ = io.WriteString(w, `{"access_token":"t","expires_in":3600}`)
	}))
	defer srv.Close()
	c := New(&config.Config{BaseURL: srv.URL, TokenURL: srv.URL + "/token", ClientID: "id-1234567", ClientSecret: "secret-1234567"}, 5*time.Second, 0)
	c.DryRun = true
	if _, err := c.Get(context.Background(), "/whoami/v1", nil); err != nil {
		t.Fatalf("dry-run Get: %v", err)
	}
	if hits != 0 {
		t.Fatalf("dry-run sent %d request(s); a preview must stay offline", hits)
	}
}
