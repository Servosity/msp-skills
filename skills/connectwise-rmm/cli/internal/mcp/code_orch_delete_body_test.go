// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/connectwise-rmm/handfixes.json
// under the "code-orch-delete-body" and "tickets-paging-together" entries.

package mcp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	mcplib "github.com/mark3labs/mcp-go/mcp"
)

// TestCodeOrchExecuteSendsUninstallBody drives the execute tool against a stub
// Asio and asserts endpoints.uninstall arrives as DELETE with the endpoints,
// feedback and reason in a JSON body, the same request the CLI's
// `endpoints uninstall` sends. The generated routing put DELETE params on the
// query string and sent no body.
func TestCodeOrchExecuteSendsUninstallBody(t *testing.T) {
	var gotMethod, gotQuery string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/token") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"access_token":"stub-token","expires_in":3600}`)
			return
		}
		gotMethod, gotQuery = r.Method, r.URL.RawQuery
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{}`)
	}))
	defer srv.Close()

	t.Setenv("CONNECTWISE_RMM_HOME", t.TempDir())
	t.Setenv("CONNECTWISE_RMM_BASE_URL", srv.URL)
	t.Setenv("CONNECTWISE_RMM_TOKEN_URL", srv.URL+"/token")
	t.Setenv("CONNECTWISE_RMM_CLIENT_ID", "stub-id")
	t.Setenv("CONNECTWISE_RMM_CLIENT_SECRET", "stub-secret")
	t.Setenv("CONNECTWISE_RMM_MCP_READ_ONLY", "")

	req := mcplib.CallToolRequest{Params: mcplib.CallToolParams{Arguments: map[string]any{
		"endpoint_id": "endpoints.uninstall",
		"params": map[string]any{
			"endpoints": []any{"7c9e6679-7425-40de-944b-e07fc1f90ae7"},
			"feedback":  "decommissioned",
			"reason":    "hardware retired",
		},
	}}}
	res, err := handleCodeOrchExecute(context.Background(), req)
	if err != nil || res == nil || res.IsError {
		t.Fatalf("execute failed: err=%v result=%+v", err, res)
	}
	if gotMethod != http.MethodDelete {
		t.Fatalf("method = %q, want DELETE", gotMethod)
	}
	if gotQuery != "" {
		t.Fatalf("params leaked to the query string: %q", gotQuery)
	}
	eps, _ := gotBody["endpoints"].([]any)
	if len(eps) != 1 || gotBody["feedback"] != "decommissioned" || gotBody["reason"] != "hardware retired" {
		t.Fatalf("uninstall body = %v, want endpoints/feedback/reason", gotBody)
	}
}

// TestCodeOrchTicketsListPairsPagingKeys: the execute twin of the CLI's
// tickets list pairing. A lone pageSize gains pageNum=1; a lone pageNum is
// refused before any request.
func TestCodeOrchTicketsListPairsPagingKeys(t *testing.T) {
	var queries []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/token") {
			_, _ = io.WriteString(w, `{"access_token":"stub-token","expires_in":3600}`)
			return
		}
		queries = append(queries, r.URL.RawQuery)
		_, _ = io.WriteString(w, `{"tickets":[]}`)
	}))
	defer srv.Close()
	t.Setenv("CONNECTWISE_RMM_HOME", t.TempDir())
	t.Setenv("CONNECTWISE_RMM_BASE_URL", srv.URL)
	t.Setenv("CONNECTWISE_RMM_TOKEN_URL", srv.URL+"/token")
	t.Setenv("CONNECTWISE_RMM_CLIENT_ID", "stub-id")
	t.Setenv("CONNECTWISE_RMM_CLIENT_SECRET", "stub-secret")

	call := func(params map[string]any) *mcplib.CallToolResult {
		res, err := handleCodeOrchExecute(context.Background(), mcplib.CallToolRequest{Params: mcplib.CallToolParams{Arguments: map[string]any{
			"endpoint_id": "tickets.list", "params": params}}})
		if err != nil {
			t.Fatalf("execute: %v", err)
		}
		return res
	}
	if res := call(map[string]any{"pageSize": float64(5)}); res.IsError || len(queries) != 1 || !strings.Contains(queries[0], "pageNum=1") || !strings.Contains(queries[0], "pageSize=5") {
		t.Fatalf("lone pageSize: result=%+v queries=%q, want pageNum=1&pageSize=5", res, queries)
	}
	queries = nil
	if res := call(map[string]any{"pageNum": float64(2)}); !res.IsError || len(queries) != 0 {
		t.Fatalf("lone pageNum: result=%+v queries=%q, want a refusal and no request", res, queries)
	}
}
