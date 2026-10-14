// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
