// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	mcplib "github.com/mark3labs/mcp-go/mcp"
)

// TestCodeOrchExecuteReportsMerakiNextPage guards the hand-fix
// meraki-mcp-execute-link-next: Meraki pages list endpoints only through the
// Link header, so cisco-meraki_execute must tell the agent a page is partial
// (has_more + next_starting_after) and stay quiet on the last page.
func TestCodeOrchExecuteReportsMerakiNextPage(t *testing.T) {
	resetMCPPathEnv(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/organizations" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("startingAfter") == "" {
			w.Header().Set("Link", "<http://"+r.Host+"/api/v1/organizations?perPage=2&startingAfter=600001>; rel=next")
			_, _ = w.Write([]byte(`[{"id":"600000","name":"Acme"},{"id":"600001","name":"Bayside"}]`))
			return
		}
		_, _ = w.Write([]byte(`[{"id":"600002","name":"Cedar"}]`))
	}))
	defer srv.Close()
	t.Setenv("CISCO_MERAKI_BASE_URL", srv.URL+"/api/v1")
	t.Setenv("MERAKI_DASHBOARD_API_KEY", "test-key")

	call := func(params map[string]any) map[string]any {
		t.Helper()
		res, err := handleCodeOrchExecute(context.Background(), mcplib.CallToolRequest{Params: mcplib.CallToolParams{
			Arguments: map[string]any{"endpoint_id": "organizations.get", "params": params},
		}})
		if err != nil || res == nil || res.IsError {
			t.Fatalf("execute failed: err=%v result=%+v", err, res)
		}
		var out map[string]any
		if err := json.Unmarshal([]byte(res.Content[0].(mcplib.TextContent).Text), &out); err != nil {
			t.Fatalf("result is not a JSON object: %v", err)
		}
		return out
	}

	first := call(map[string]any{})
	if first["has_more"] != true || first["next_starting_after"] != "600001" {
		t.Fatalf("page one must report has_more and the Link cursor, got %v", first)
	}
	last := call(map[string]any{"startingAfter": "600001"})
	if _, ok := last["has_more"]; ok {
		t.Fatalf("last page must not report has_more, got %v", last)
	}
	if last["count"] != float64(1) {
		t.Fatalf("last page count = %v, want 1", last["count"])
	}
}

// TestCodeOrchExecuteTruncatedPageWithholdsCursor: when an upstream page holds
// more items than one MCP result carries, the Link cursor points past items the
// agent never saw, so it must not be offered as the resume point.
func TestCodeOrchExecuteTruncatedPageWithholdsCursor(t *testing.T) {
	resetMCPPathEnv(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Link", "<http://"+r.Host+"/api/v1/organizations?perPage=120&startingAfter=600119>; rel=next")
		items := make([]string, 0, 120)
		for i := 0; i < 120; i++ {
			items = append(items, fmt.Sprintf(`{"id":"%d","name":"Org %d"}`, 600000+i, i))
		}
		_, _ = w.Write([]byte("[" + strings.Join(items, ",") + "]"))
	}))
	defer srv.Close()
	t.Setenv("CISCO_MERAKI_BASE_URL", srv.URL+"/api/v1")
	t.Setenv("MERAKI_DASHBOARD_API_KEY", "test-key")

	res, err := handleCodeOrchExecute(context.Background(), mcplib.CallToolRequest{Params: mcplib.CallToolParams{
		Arguments: map[string]any{"endpoint_id": "organizations.get", "params": map[string]any{"perPage": 120}},
	}})
	if err != nil || res == nil || res.IsError {
		t.Fatalf("execute failed: err=%v result=%+v", err, res)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(res.Content[0].(mcplib.TextContent).Text), &out); err != nil {
		t.Fatalf("result is not a JSON object: %v", err)
	}
	if out["truncated"] != true || out["has_more"] != true {
		t.Fatalf("want a truncated page flagged has_more, got truncated=%v has_more=%v", out["truncated"], out["has_more"])
	}
	if _, ok := out["next_starting_after"]; ok {
		t.Fatalf("a truncated page must not offer the Link cursor (it skips unseen items): %v", out["next_starting_after"])
	}
	if note, _ := out["pagination_note"].(string); !strings.Contains(note, "perPage") {
		t.Fatalf("note must tell the agent to shrink perPage, got %q", note)
	}
}
