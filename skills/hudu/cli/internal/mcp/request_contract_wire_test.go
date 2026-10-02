// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
package mcp

import (
	"context"
	"encoding/json"
	mcplib "github.com/mark3labs/mcp-go/mcp"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// Exercise the real MCP execute handler and client against a local server. These
// requests never use a tenant or send credentials outside the fixture.
func TestRequestContractWire(t *testing.T) {
	type observed struct {
		method, path, query, auth string
		body                      []byte
	}
	seen := make(chan observed, 10)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		seen <- observed{r.Method, r.URL.Path, r.URL.RawQuery, r.Header.Get("x-api-key"), b}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("HUDU_HOME", home)
	t.Setenv("HUDU_BASE_URL", srv.URL)
	t.Setenv("HUDU_API_KEY", "fixture-token")
	t.Setenv("HUDU_ACCESS_TOKEN", "fixture-token")
	t.Setenv("HUDU_CLIENT_ID", "")
	t.Setenv("HUDU_CLIENT_SECRET", "")
	t.Setenv("PRINTING_PRESS_CLIENT_PROFILE", "")
	cfg := filepath.Join(home, ".config", "hudu-cli", "config.toml")
	if err := os.MkdirAll(filepath.Dir(cfg), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg, []byte("access_token = \"fixture-token\"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HUDU_CONFIG", cfg)
	for _, tc := range []struct {
		id, method, path string
		params           map[string]any
		query, body      string
	}{

		{"articles.create", "POST", "/articles", map[string]any{"name": "Runbook", "content": "<p>Example</p>", "company_id": float64(7)}, "", `{"name":"Runbook","content":"<p>Example</p>","company_id":7}`},
		{"articles.get", "GET", "/articles/42", map[string]any{"id": "42"}, "", ""},
		{"articles.list", "GET", "/articles", map[string]any{"company-id": float64(7), "page-size": float64(2)}, "company_id=7&page_size=2", ""},
	} {
		t.Run(tc.id, func(t *testing.T) {
			req := mcplib.CallToolRequest{}
			req.Params.Arguments = map[string]any{"endpoint_id": tc.id, "params": tc.params}
			result, err := handleCodeOrchExecute(context.Background(), req)
			if err != nil {
				t.Fatal(err)
			}
			if result == nil || result.IsError {
				t.Fatalf("execute failed: %#v", result)
			}
			var got observed
			select {
			case got = <-seen:
			default:
				t.Fatal("execute sent no request")
			}
			if got.method != tc.method || got.path != tc.path || got.query != tc.query {
				t.Fatalf("wire = %s %s?%s; want %s %s?%s", got.method, got.path, got.query, tc.method, tc.path, tc.query)
			}
			if got.auth != "fixture-token" {
				t.Fatalf("auth header = %q", got.auth)
			}
			if tc.body == "" {
				if len(got.body) != 0 {
					t.Fatalf("unexpected body %s", got.body)
				}
				return
			}
			var wantBody, gotBody any
			if err := json.Unmarshal([]byte(tc.body), &wantBody); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(got.body, &gotBody); err != nil {
				t.Fatalf("invalid JSON body %s: %v", got.body, err)
			}
			if !reflect.DeepEqual(gotBody, wantBody) {
				t.Fatalf("JSON body = %s; want %s", got.body, tc.body)
			}
		})
	}
}
