// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package mcp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"blumira-pp-cli/internal/cliutil/testenv"
)

// An endpoint write (resolving a finding) sends nothing to the
// server until confirm is boolean true (handfix mcp-endpoint-write-confirm).
func TestEndpointWriteSendsOnlyWithConfirm(t *testing.T) {
	testenv.Isolate(t)
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"finding_id":"8d1f2e3c-4b5a-4c6d-9e7f-0a1b2c3d4e5f"}`))
	}))
	defer srv.Close()
	t.Setenv("BLUMIRA_BASE_URL", srv.URL)
	t.Setenv("BLUMIRA_API_TOKEN", "test-token")

	s := server.NewMCPServer("confirm", "test")
	RegisterTools(s)
	entry, ok := s.ListTools()["org_controller-direct-resolve-finding"]
	if !ok {
		t.Fatal("org_controller-direct-resolve-finding not registered")
	}
	call := func(args map[string]any) *mcplib.CallToolResult {
		req := mcplib.CallToolRequest{}
		req.Params.Name = "org_controller-direct-resolve-finding"
		req.Params.Arguments = args
		res, err := entry.Handler(context.Background(), req)
		if err != nil {
			t.Fatalf("handler error: %v", err)
		}
		return res
	}
	base := map[string]any{"finding_id": "8d1f2e3c-4b5a-4c6d-9e7f-0a1b2c3d4e5f", "resolution": float64(20)}
	for _, confirm := range []any{nil, false, "true", float64(1)} {
		args := map[string]any{"finding_id": "8d1f2e3c-4b5a-4c6d-9e7f-0a1b2c3d4e5f", "resolution": float64(20)}
		if confirm != nil {
			args["confirm"] = confirm
		}
		res := call(args)
		text := res.Content[0].(mcplib.TextContent).Text
		if !strings.Contains(text, "confirmation_required") {
			t.Errorf("confirm=%v: want a preview, got %s", confirm, text)
		}
	}
	if n := hits.Load(); n != 0 {
		t.Fatalf("unconfirmed calls reached the server %d time(s)", n)
	}
	base["confirm"] = true
	call(base)
	if n := hits.Load(); n != 1 {
		t.Fatalf("confirmed call reached the server %d time(s), want 1", n)
	}
}
