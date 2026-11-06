// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (handfixes.json mcp-write-preview-confirm). The only write,
// PUT .../bulkSeatChange, must preview over MCP and send only with confirm:
// true. Press 4.34.2's endpoint-mirror surface registered a raw
// seats_bulk-change tool that sent with no confirm; the spec's x-mcp block
// (orchestration: code, endpoint_tools: hidden) removes it and routes the
// write through the confirming datto-saas_execute. This fails if a reprint
// brings the raw tool back or the execute tool stops gating.

package mcp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func TestMCPBulkSeatChangeSendsOnlyWithConfirm(t *testing.T) {
	resetMCPPathEnv(t)
	var mu sync.Mutex
	var methods []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		methods = append(methods, r.Method+" "+r.URL.Path)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"remoteId":"r1","status":"Success"}]`))
	}))
	defer srv.Close()
	t.Setenv("DATTO_SAAS_BASE_URL", srv.URL+"/v1")
	t.Setenv("DATTO_SAAS_PUBLIC_KEY", "pub-test")
	t.Setenv("DATTO_SAAS_SECRET_KEY", "secret-test")
	sent := func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), methods...)
	}

	s := server.NewMCPServer("datto-saas", "test")
	RegisterTools(s)
	tools := s.ListTools()
	for name, entry := range tools {
		if strings.Contains(strings.ReplaceAll(name, "_", "-"), "bulk-change") {
			t.Fatalf("raw write tool %q is registered; it sends without confirm", name)
		}
		if strings.Contains(entry.Tool.Description, "bulkSeatChange") && name != "datto-saas_execute" && name != "datto-saas_search" {
			t.Fatalf("tool %q exposes bulkSeatChange outside datto-saas_execute", name)
		}
	}
	exec, ok := tools["datto-saas_execute"]
	if !ok {
		t.Fatal("datto-saas_execute is not registered")
	}

	call := func(confirm any) *mcplib.CallToolResult {
		t.Helper()
		args := map[string]any{
			"endpoint_id": "seats.bulk-change",
			"params": map[string]any{
				"saasCustomerId":         123456,
				"externalSubscriptionId": "Classic:Office365:654321",
				"seat_type":              "User",
				"action_type":            "License",
				"ids":                    []any{"r1"},
			},
		}
		if confirm != nil {
			args["confirm"] = confirm
		}
		var req mcplib.CallToolRequest
		req.Params.Name = "datto-saas_execute"
		req.Params.Arguments = args
		res, err := exec.Handler(context.Background(), req)
		if err != nil {
			t.Fatalf("execute: %v", err)
		}
		return res
	}
	text := func(res *mcplib.CallToolResult) string {
		var b strings.Builder
		for _, c := range res.Content {
			if tc, ok := c.(mcplib.TextContent); ok {
				b.WriteString(tc.Text)
			}
		}
		return b.String()
	}

	for _, confirm := range []any{nil, false, "true", 1} {
		res := call(confirm)
		if got := sent(); len(got) != 0 {
			t.Fatalf("confirm=%#v sent %v; a write must not leave without confirm: true", confirm, got)
		}
		if confirm == nil && !strings.Contains(text(res), "confirmation_required") {
			t.Fatalf("no confirm: want a preview, got %q", text(res))
		}
	}

	t.Setenv("DATTO_SAAS_MCP_READ_ONLY", "1")
	if res := call(true); !res.IsError || len(sent()) != 0 {
		t.Fatalf("read-only switch: confirm=true must be refused and send nothing; got %q, sent %v", text(res), sent())
	}
	t.Setenv("DATTO_SAAS_MCP_READ_ONLY", "")

	res := call(true)
	if res.IsError {
		t.Fatalf("confirm=true: %s", text(res))
	}
	if got := sent(); len(got) != 1 || got[0] != "PUT /v1/saas/123456/Classic:Office365:654321/bulkSeatChange" {
		t.Fatalf("confirm=true: want exactly one PUT to bulkSeatChange, got %v", got)
	}
}
