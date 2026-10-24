// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): regression tests for the final Codex session's
// round-1 input, pinned by skills/cipp/handfixes.json under
// "success-body-credential-funnel".

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

// spacedEcho is the configured key aaaaaaaaaaaa base64-encoded twice
// (WVdGaFlXRmhZV0ZoWVdGaA==), cut by whitespace as Codex sent it.
const spacedEcho = "WVdG aFlX RmhZ V0Zo WVdG aA=="

// TestSplitEncodedBearerIsWithheld: Codex's exact input and its variants on
// the CLI path, cache on. CIPP_API_KEY is aaaaaaaaaaaa; each body carries the
// key base64-encoded twice, cut by whitespace or format characters. Nothing of
// it may come back or be cached.
func TestSplitEncodedBearerIsWithheld(t *testing.T) {
	cases := map[string]struct{ contentType, body string }{
		// Codex's input: one JSON string, spaces inside the run, 70000 spaces after.
		"codex-spaces": {"application/json", `{"echo":"` + spacedEcho + strings.Repeat(" ", 70000) + `"}`},
		// The run cut by tabs, newlines, carriage returns and spaces, padded
		// with them (JSON escapes). Tabs and newlines alone were already
		// stripped as control characters; mixed with spaces they kept the run
		// split.
		"tabs-newlines": {"application/json", `{"echo":"WVdG\taFlX\nRmhZ V0Zo\r\nWVdG aA==` + strings.Repeat(` \t\n`, 70000) + `"}`},
		// The run split across format characters and spaces (JSON escapes).
		"format-chars": {"application/json", `{"echo":"WVdG\u200baFlX \u2060RmhZ\ufeff V0Zo\u200d WVdG\u200caA==` + strings.Repeat(" ", 70000) + `"}`},
		// A body that is not JSON.
		"text-plain": {"text/plain", spacedEcho + strings.Repeat(" ", 70000)},
		// The run split by Unicode spaces (no-break, ideographic), padded with them.
		"unicode-spaces": {"application/json", `{"echo":"WVdG\u00a0aFlX\u3000RmhZ V0Zo\u00a0WVdG aA==` + strings.Repeat(`\u3000`, 25000) + `"}`},
		// A body past 1 MiB with one value past the 64 KiB cap once whitespace
		// goes: the body is withheld as too large to check.
		"over-cap-run": {"application/json", `{"echo":"WVdGaFlXRmhZV0ZoWVdGaA==` + strings.Repeat(".", 1<<20+10) + `"}`},
	}
	for name, tc := range cases {
		tc := tc
		t.Run(name, func(t *testing.T) {
			clearVerifyEnv(t)
			api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", tc.contentType)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer api.Close()
			c := New(&config.Config{BaseURL: api.URL, CippApiKey: "aaaaaaaaaaaa", AuthSource: "env:CIPP_API_KEY", Path: filepath.Join(t.TempDir(), "config.toml")}, 30*time.Second, 0)
			cache := t.TempDir()
			c.cacheDir = cache
			data, _ := c.Get(context.Background(), "/ListUsers", map[string]string{"tenantFilter": "t.example"})
			if strings.Contains(string(data), "WVdG") {
				t.Fatal("the split double-base64 bearer came back in the body")
			}
			_ = filepath.WalkDir(cache, func(p string, d fs.DirEntry, err error) error {
				if err == nil && !d.IsDir() {
					if b, rerr := os.ReadFile(p); rerr == nil && strings.Contains(string(b), "WVdG") {
						t.Errorf("the split double-base64 bearer was cached in %s", p)
					}
				}
				return nil
			})
		})
	}
}
