// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"ninjaone-pp-cli/internal/config"
)

// TestDryRunNeverMintsAToken: with client credentials and no cached token, a
// --dry-run request puts nothing on the wire. authHeader used to mint before
// the dry-run branch, POSTing the client secret to the token URL and caching
// the token during a preview. The live control proves the same client does
// reach the token URL, so the zero is the guard and not a probe that never got
// that far. Hand-owned (handfixes.json: dry-run-never-mints).
func TestDryRunNeverMintsAToken(t *testing.T) {
	home := t.TempDir()
	for _, k := range []string{"HOME", "USERPROFILE", "XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME", "XDG_CACHE_HOME", "NINJAONE_HOME"} {
		t.Setenv(k, home)
	}
	t.Setenv("PRINTING_PRESS_VERIFY", "")
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"stub-access-token","token_type":"Bearer","expires_in":3600}`)
	}))
	defer srv.Close()
	cfg := &config.Config{
		Path:         filepath.Join(home, "config.toml"),
		BaseURL:      srv.URL,
		TokenURL:     srv.URL + "/token",
		ClientID:     "stub-client-id",
		ClientSecret: "stub-client-secret",
	}
	c := New(cfg, 5*time.Second, 0)
	c.DryRun = true
	for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodDelete} {
		if _, _, err := c.do(context.Background(), method, "/dry-run-probe", nil, map[string]any{"name": "probe"}, nil); err != nil {
			t.Fatalf("dry-run %s: %v", method, err)
		}
	}
	if n := hits.Load(); n != 0 {
		t.Fatalf("a dry run sent %d request(s); a preview must stay offline", n)
	}
	if cfg.AccessToken != "" {
		t.Fatal("a dry run minted and cached an access token")
	}
	c.DryRun = false
	_, _, _ = c.do(context.Background(), http.MethodGet, "/dry-run-probe", nil, nil, nil)
	if hits.Load() == 0 {
		t.Fatal("control: the same request without --dry-run sent nothing, so the dry-run check proved nothing")
	}
}
