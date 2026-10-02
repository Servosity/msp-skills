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
	"strings"
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
		seen <- observed{r.Method, r.URL.Path, r.URL.RawQuery, r.Header.Get("Authorization"), b}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("NINJAONE_HOME", home)
	t.Setenv("NINJAONE_BASE_URL", srv.URL)
	t.Setenv("NINJAONE_API_KEY", "fixture-token")
	t.Setenv("NINJAONE_ACCESS_TOKEN", "fixture-token")
	t.Setenv("NINJAONE_CLIENT_ID", "")
	t.Setenv("NINJAONE_CLIENT_SECRET", "")
	t.Setenv("PRINTING_PRESS_CLIENT_PROFILE", "")
	cfg := filepath.Join(home, ".config", "ninjaone-cli", "config.toml")
	if err := os.MkdirAll(filepath.Dir(cfg), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg, []byte("access_token = \"fixture-token\"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("NINJAONE_CONFIG", cfg)
	for _, tc := range []struct {
		id, method, path string
		params           map[string]any
		query, body      string
	}{

		{"document-templates.create", "POST", "/v2/document-templates", map[string]any{"name": "Runbook", "fields": []any{map[string]any{"name": "Hostname", "type": "TEXT"}}}, "", `{"name":"Runbook","fields":[{"name":"Hostname","type":"TEXT"}]}`},
		{"document-templates.get", "GET", "/v2/document-templates/42", map[string]any{"documentTemplateId": float64(42), "includeTechnicianRoles": true}, "includeTechnicianRoles=true", ""},
		{"device.script.run-on-device", "POST", "/v2/device/42/script/run", map[string]any{"path_id": "42", "id": "script-17", "type": "SCRIPT"}, "", `{"id":"script-17","type":"SCRIPT"}`},
		{"document-templates.archive", "POST", "/v2/document-templates/archive", map[string]any{"body": []any{float64(42), float64(43)}}, "", `[42,43]`},
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
			if got.auth != "Bearer fixture-token" {
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

// Missing a device path input must not reinterpret the script's body ID as a
// device ID. Invalid local config proves this fails before client construction.
func TestRequestContractMissingCollisionPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cfg := filepath.Join(home, ".config", "ninjaone-cli", "config.toml")
	if err := os.MkdirAll(filepath.Dir(cfg), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg, []byte("[invalid TOML"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("NINJAONE_CONFIG", cfg)
	for _, params := range []map[string]any{
		{"id": "script-17", "type": "SCRIPT"},
		{"path_id": "", "id": "script-17", "type": "SCRIPT"},
		{"path_id": nil, "id": "script-17", "type": "SCRIPT"},
	} {
		req := mcplib.CallToolRequest{}
		req.Params.Arguments = map[string]any{"endpoint_id": "device.script.run-on-device", "params": params}
		result, err := handleCodeOrchExecute(context.Background(), req)
		if err != nil {
			t.Fatal(err)
		}
		if result == nil || !result.IsError {
			t.Fatalf("missing path_id was accepted: %#v", result)
		}
		content := result.Content[0].(mcplib.TextContent).Text
		if !strings.Contains(content, `required path parameter "path_id" is missing`) {
			t.Fatalf("wrong failure: %s", content)
		}
		if params["id"] != "script-17" {
			t.Fatalf("body id consumed: %#v", params)
		}
	}
}
