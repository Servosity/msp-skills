// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
package mcp

import (
	"context"
	"fmt"
	mcplib "github.com/mark3labs/mcp-go/mcp"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestAcronisDirectMCPContracts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/2/search":
			if r.URL.Query().Get("text") != "Customer" || r.URL.Query().Get("tenant") != "root" {
				t.Error("search wire params", r.URL)
			}
			fmt.Fprint(w, `{"items":[]}`)
		case "/api/task_manager/v2/tasks":
			if r.URL.Query().Has("tenant_id") || r.URL.Query().Get("resultCode") != "error" {
				t.Error("task wire params", r.URL)
			}
			fmt.Fprint(w, `{"items":[{"id":1,"tenant":{"uuid":"root"}}],"paging":{"cursors":{}}}`)
		default:
			t.Error("unexpected request", r.URL)
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	t.Setenv("ACRONIS_BASE_URL", srv.URL)
	t.Setenv("ACRONIS_BEARER_AUTH", "fixture-token")
	t.Setenv("ACRONIS_CONFIG", filepath.Join(t.TempDir(), "config.toml"))
	for _, path := range []string{"/api/2/search", "/api/task_manager/v2/tasks"} {
		args := map[string]any{"tenant_id": "root", "limit": 100}
		if path == "/api/2/search" {
			args["query"] = "Customer"
		} else {
			args["result_code"] = "error"
		}
		var bindings []mcpParamBinding
		for name := range args {
			bindings = append(bindings, mcpParamBinding{PublicName: name, WireName: name, Location: "query"})
		}
		handler := makeAPIHandler("GET", path, true, false, nil, bindings, nil)
		result, err := handler(context.Background(), mcplib.CallToolRequest{Params: mcplib.CallToolParams{Arguments: args}})
		if err != nil || result.IsError {
			t.Fatalf("%s: result=%+v err=%v", path, result, err)
		}
	}
}
