// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/dnsfilter/handfixes.json
// under the "binary-envelope-never-hides-text" entry.

package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"dnsfilter-pp-cli/internal/config"
)

// TestBinaryLabelNeverHidesText: a JSON body served as octet-stream comes back
// as JSON (so the filters can read it) and, carrying a secret key, is not
// cached; a real binary body is wrapped and never cached.
func TestBinaryLabelNeverHidesText(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		if r.URL.Path == "/v1/networks" {
			_, _ = w.Write([]byte(`{"data":[{"id":"1","attributes":{"secret_key":"test-secret"}}]}`))
			return
		}
		_, _ = w.Write([]byte{0x89, 'P', 'N', 'G', 0x00, 0xff})
	}))
	t.Cleanup(server.Close)
	c := New(&config.Config{BaseURL: server.URL, DnsfilterApiKey: "test-key"}, 5*time.Second, 0)
	c.cacheDir = t.TempDir()
	data, err := c.Get(context.Background(), "/v1/networks", nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "_pp_binary") || !strings.Contains(string(data), `"secret_key"`) {
		t.Fatalf("a JSON body labelled binary was wrapped: %s", data)
	}
	data, err = c.Get(context.Background(), "/v1/logo", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, ok := UnwrapBinaryResponse(data); !ok {
		t.Fatalf("a real binary body was not wrapped: %s", data)
	}
	var cached []string
	_ = filepath.Walk(c.cacheDir, func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			cached = append(cached, p)
		}
		return nil
	})
	if len(cached) != 0 {
		t.Fatalf("a secret-bearing or binary body was cached: %v", cached)
	}
}
