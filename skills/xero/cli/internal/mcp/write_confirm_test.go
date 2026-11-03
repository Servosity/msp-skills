// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written (NOT generated): pinned by handfixes.json under "mcp-write-confirm".

package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registeredServer(t *testing.T) *server.MCPServer {
	t.Helper()
	s := server.NewMCPServer("xero-confirm-test", "0")
	RegisterTools(s)
	return s
}

func toolSchemaHasConfirm(tool mcplib.Tool) bool {
	if len(tool.RawInputSchema) > 0 {
		var schema struct {
			Properties map[string]any `json:"properties"`
		}
		_ = json.Unmarshal(tool.RawInputSchema, &schema)
		_, ok := schema.Properties[confirmArg]
		return ok
	}
	_, ok := tool.InputSchema.Properties[confirmArg]
	return ok
}

// TestEveryNonReadOnlyToolNeedsConfirm walks the real registered tool set:
// every tool that is not annotated read-only must advertise confirm, and
// read-only tools must not (a read needs no ceremony).
func TestEveryNonReadOnlyToolNeedsConfirm(t *testing.T) {
	tools := registeredServer(t).ListTools()
	var writes, missing, extra []string
	for name, entry := range tools {
		readOnly := entry.Tool.Annotations.ReadOnlyHint != nil && *entry.Tool.Annotations.ReadOnlyHint
		has := toolSchemaHasConfirm(entry.Tool)
		switch {
		case !readOnly && !has:
			missing = append(missing, name)
		case readOnly && has:
			extra = append(extra, name)
		}
		if !readOnly {
			writes = append(writes, name)
		}
	}
	if len(writes) == 0 {
		t.Fatal("no write tools registered; this test proves nothing")
	}
	if len(missing) > 0 {
		t.Fatalf("write tools without confirm: %v", missing)
	}
	if len(extra) > 0 {
		t.Fatalf("read-only tools gated by confirm: %v", extra)
	}
}

// TestWriteToolSendsOnlyWithConfirmTrue drives a real write tool against a
// loopback stub: no confirm, confirm:"true" (a string) and confirm:false all
// preview and send nothing; only confirm:true reaches the API, and confirm
// itself never reaches the generated handler's argument check.
func TestWriteToolSendsOnlyWithConfirmTrue(t *testing.T) {
	resetMCPPathEnv(t)
	var hits atomic.Int32
	var lastMethod, lastPath atomic.Value
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		lastMethod.Store(r.Method)
		lastPath.Store(r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"Invoices":[]}`))
	}))
	defer stub.Close()
	t.Setenv("XERO_BASE_URL", stub.URL)
	t.Setenv("XERO_ACCESS_TOKEN", "not-a-real-token")

	entry := registeredServer(t).ListTools()["invoices_create"]
	if entry == nil {
		t.Fatal("invoices_create is not registered")
	}
	call := func(args map[string]any) *mcplib.CallToolResult {
		t.Helper()
		var req mcplib.CallToolRequest
		req.Params.Name = "invoices_create"
		req.Params.Arguments = args
		res, err := entry.Handler(context.Background(), req)
		if err != nil {
			t.Fatalf("handler error: %v", err)
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

	body := []any{map[string]any{"Type": "ACCREC"}}
	for _, args := range []map[string]any{
		{"Invoices": body},
		{"Invoices": body, "confirm": "true"},
		{"Invoices": body, "confirm": false},
		{"Invoices": body, "confirm": 1.0},
	} {
		res := call(args)
		if got := hits.Load(); got != 0 {
			t.Fatalf("args %v reached the API (%d requests) without confirm:true", args, got)
		}
		if out := text(res); res.IsError || !strings.Contains(out, `"confirmation_required"`) || !strings.Contains(out, `"sent": false`) && !strings.Contains(out, `"sent":false`) {
			t.Fatalf("args %v: want a preview, got error=%v %s", args, res.IsError, out)
		}
	}

	res := call(map[string]any{"Invoices": body, "confirm": true})
	if res.IsError {
		t.Fatalf("confirm:true call failed: %s", text(res))
	}
	if got := hits.Load(); got != 1 {
		t.Fatalf("confirm:true sent %d requests, want 1", got)
	}
	if m, p := lastMethod.Load(), lastPath.Load(); m != "PUT" || p != "/Invoices" {
		t.Fatalf("confirm:true sent %v %v, want PUT /Invoices", m, p)
	}
}
