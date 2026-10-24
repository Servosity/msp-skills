// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): regression tests for the batch-1b Codex
// round-1 inputs, pinned by skills/cipp/handfixes.json under
// "token-error-text-allowlist" and "success-body-credential-funnel".

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

// TestMintTokenDecodeErrorQuotesNothing: Codex's input on the re-mint. HTTP
// 200 whose expires_in overflows int: the error names the kind only.
func TestMintTokenDecodeErrorQuotesNothing(t *testing.T) {
	const token = "123456789012345678901234567890"
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"access_token":"` + token + `","expires_in":` + token + `}`))
	}))
	defer srv.Close()
	c := New(cfgWithCreds(t, srv.URL, t.TempDir()), 30*time.Second, 0)
	c.HTTPClient = srv.Client()
	resetMintState()
	t.Cleanup(resetMintState)
	err := c.mintClientCredentials(context.Background())
	if err == nil {
		t.Fatal("an undecodable token response was accepted")
	}
	if strings.Contains(err.Error(), token[:12]) || !strings.Contains(err.Error(), "could not be decoded") {
		t.Fatalf("the error quotes the response or does not name the kind: %v", err)
	}
}

// TestLargeBodyDoubleBase64BearerIsWithheld: Codex's input on the CLI path. A
// body past 1 MiB carries the configured key base64-encoded twice; neither the
// returned body nor the response cache may carry it.
func TestLargeBodyDoubleBase64BearerIsWithheld(t *testing.T) {
	clearVerifyEnv(t)
	const echo = "WVdGaFlXRmhZV0ZoWVdGaA=="
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"echo":"` + echo + `","padding":"` + strings.Repeat("z", 1<<20) + `"}`))
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
