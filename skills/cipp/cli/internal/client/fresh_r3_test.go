// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): regression tests for the fresh Codex round-3
// input on the success-body funnel, pinned by skills/cipp/handfixes.json under
// "success-body-credential-funnel".

package client

import (
	"context"
	"errors"
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

// TestSuccessFunnelZeroWidthSplitBearer: Codex's input. CIPP_API_KEY is
// abcdefghijklmnop and a 200 body's note is "abcdefgh", 200000 U+200B, then
// "ijklmnop": more than any detection window held. Neither the returned body
// nor the response cache may carry the bearer once format characters drop.
func TestSuccessFunnelZeroWidthSplitBearer(t *testing.T) {
	clearVerifyEnv(t)
	const bearer = "abcdefghijklmnop"
	note := "abcdefgh" + strings.Repeat("\u200b", 200000) + "ijklmnop"
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"Results":[{"id":"u1","note":"` + note + `"}]}`))
	}))
	defer api.Close()
	cache := t.TempDir()
	c := New(&config.Config{BaseURL: api.URL, CippApiKey: bearer, AuthSource: "env:CIPP_API_KEY", Path: filepath.Join(t.TempDir(), "config.toml")}, 5*time.Second, 0)
	c.cacheDir = cache
	data, _ := c.Get(context.Background(), "/ListUsers", map[string]string{"tenantFilter": "t.example"})
	if strings.Contains(strings.ReplaceAll(string(data), "\u200b", ""), bearer) {
		t.Fatal("the zero-width-split bearer came back in the body")
	}
	_ = filepath.WalkDir(cache, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if b, rerr := os.ReadFile(p); rerr == nil && strings.Contains(strings.ReplaceAll(string(b), "\u200b", ""), bearer) {
			t.Errorf("the bearer was cached in %s", p)
		}
		return nil
	})
}

// TestSuccessFunnelScanLimitAndCleanBytes: a clean body past the shared
// detector's whole-scan limit comes back unchanged; one holding a single value
// over that limit is withheld, and so is one over the response size limit; a
// clean body keeps its bytes, format characters included.
func TestSuccessFunnelScanLimitAndCleanBytes(t *testing.T) {
	c := &Client{}
	auth := "Bearer abcdefghijklmnop"
	big := listUsersBody(2<<20, `"E-1001"`)
	if out, err := c.redactSuccessBody(big, auth); err != nil || string(out) != string(big) {
		t.Fatalf("a clean 2 MiB body did not come back unchanged: %d bytes, %v", len(out), err)
	}
	oneValue := []byte(`{"note":"` + strings.Repeat("x", 2<<20) + `"}`)
	if _, err := c.redactSuccessBody(oneValue, auth); !errors.Is(err, ErrSuccessBodyValueTooLarge) {
		t.Fatalf("a body with one 2 MiB value was not withheld as too large to check: %v", err)
	}
	t.Setenv(maxResponseBytesEnv, "1500000")
	if _, err := c.redactSuccessBody(big, auth); err == nil || !strings.Contains(err.Error(), "response size limit") {
		t.Fatalf("a body over the response size limit was not withheld: %v", err)
	}
	clean := []byte("{\"displayName\":\"Family \U0001F468\u200d\U0001F469\"}")
	if out, err := c.redactSuccessBody(clean, auth); err != nil || string(out) != string(clean) {
		t.Fatalf("a clean body changed: %q, %v", out, err)
	}
}
