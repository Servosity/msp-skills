// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): regression test for the batch-1b Codex
// round-2 input on large text bodies, pinned by skills/cipp/handfixes.json
// under "success-body-credential-funnel".

package client

import (
	"context"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cipp-pp-cli/internal/config"
)

// TestLargeTextDoubleBase64BearerIsWithheld: Codex's input on the CLI path. A
// text/plain body past 1 MiB starts with the key base64-encoded twice; neither
// the returned body nor the response cache may carry it.
func TestLargeTextDoubleBase64BearerIsWithheld(t *testing.T) {
	clearVerifyEnv(t)
	const echo = "WVdGaFlXRmhZV0ZoWVdGaA=="
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte(echo + "\n" + strings.Repeat("z", 1<<20)))
	}))
	defer api.Close()
	c := New(&config.Config{BaseURL: api.URL, CippApiKey: "aaaaaaaaaaaa", AuthSource: "env:CIPP_API_KEY", Path: filepath.Join(t.TempDir(), "config.toml")}, 30*time.Second, 0)
	cache := t.TempDir()
	c.cacheDir = cache
	data, _ := c.Get(context.Background(), "/ListUsers", map[string]string{"tenantFilter": "t.example"})
	if strings.Contains(string(data), echo[:16]) {
		t.Fatal("the double-base64 bearer came back in the body")
	}
	_ = filepath.WalkDir(cache, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if b, rerr := os.ReadFile(p); rerr == nil && strings.Contains(string(b), echo[:16]) {
				t.Errorf("the double-base64 bearer was cached in %s", p)
			}
		}
		return nil
	})
}
