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

	"dnsfilter-pp-cli/internal/cliutil"
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

// TestNonTextBodyNeverPassesAsText: a body that is not UTF-8 text is wrapped
// whatever its label (UTF-16 JSON labelled application/json), and under the
// read-only switch it is refused instead.
func TestNonTextBodyNeverPassesAsText(t *testing.T) {
	utf16 := []byte{'{', 0, '"', 0, 'k', 0, '"', 0, ':', 0, '1', 0, '}', 0}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-16")
		_, _ = w.Write(utf16)
	}))
	t.Cleanup(server.Close)
	c := New(&config.Config{BaseURL: server.URL, DnsfilterApiKey: "test-key"}, 5*time.Second, 0)
	c.NoCache = true
	data, err := c.Get(context.Background(), "/v1/networks/33", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, ok := UnwrapBinaryResponse(data); !ok {
		t.Fatalf("a UTF-16 body passed as text: %q", data)
	}
	t.Setenv(cliutil.MCPReadOnlyEnvVar, "1")
	if _, err := c.Get(context.Background(), "/v1/networks/33", nil); err == nil || !strings.Contains(err.Error(), "not UTF-8 text") {
		t.Fatalf("read-only mode returned a body no rule can read: %v", err)
	}
}

// TestReadOnlyBodiesTheFiltersCannotRead: under the read-only switch a 2xx
// body that is not JSON (XML, one-column CSV) is refused, and in every mode an
// error body that is not UTF-8 text (UTF-16) is summarized, never shown.
func TestReadOnlyBodiesTheFiltersCannotRead(t *testing.T) {
	utf16 := []byte{'s', 0, 'e', 0, 'c', 0, 'r', 0, 'e', 0, 't', 0, '_', 0, 'k', 0, 'e', 0, 'y', 0, ':', 0, 'Z', 0, 'Q', 0}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/networks/40":
			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write([]byte(`<network><secret_key>ZQ</secret_key></network>`))
		case "/v1/networks/45":
			w.Header().Set("Content-Type", "text/csv")
			_, _ = w.Write([]byte("secret_key\nZQ"))
		case "/v1/networks/21":
			w.Header().Set("Content-Type", "application/json; charset=utf-16")
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write(utf16)
		default:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"id":"1"}}`))
		}
	}))
	t.Cleanup(server.Close)
	c := New(&config.Config{BaseURL: server.URL, DnsfilterApiKey: "test-key"}, 5*time.Second, 0)
	c.NoCache = true
	if _, err := c.Get(context.Background(), "/v1/networks/21", nil); err == nil || strings.Contains(err.Error(), "Z\x00Q") || strings.Contains(err.Error(), `\u0000`) || !strings.Contains(err.Error(), "not UTF-8 text") {
		t.Fatalf("a UTF-16 error body was shown: %v", err)
	}
	t.Setenv(cliutil.MCPReadOnlyEnvVar, "1")
	for _, p := range []string{"/v1/networks/40", "/v1/networks/45"} {
		if data, err := c.Get(context.Background(), p, nil); err == nil || !strings.Contains(err.Error(), "not JSON") {
			t.Fatalf("%s: read-only mode returned a non-JSON body: %s %v", p, data, err)
		}
	}
	if _, err := c.Get(context.Background(), "/v1/networks/1", nil); err != nil {
		t.Fatalf("read-only mode refused a JSON body: %v", err)
	}
}
