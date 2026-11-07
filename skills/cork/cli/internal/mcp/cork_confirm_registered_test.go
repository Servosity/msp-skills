// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package mcp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// No registered tool can change Cork data without confirm (cork handfixes
// mcp-endpoint-write-confirm, mcp-mirror-write-confirm). A tool must be
// read-only, write only the CLI's own files (listed here as local), or carry
// the confirm gate, so an endpoint or command a later print adds is covered.
func TestRegisteredWritesNeedConfirm(t *testing.T) {
	local := map[string]bool{
		"export": true, "sync": true, "workflow_archive": true, "learnings_forget": true, "learnings_reject": true,
		"learnings_confirm": true, "playbook_amend": true, "teach": true, "teach_lookup": true, "teach_pattern": true,
		"teach_playbook": true, "recall": true, "playbook_list": true, // mcp:local-write (learn store only)
	}
	s := server.NewMCPServer("cork-confirm", "test")
	RegisterTools(s)
	tools := s.ListTools()
	for _, name := range []string{"integrations_connect", "integrations_delete", "integrations_update", "integrations_resync_integration",
		"distributor_provision-partner", "software_install", "import"} {
		entry, ok := tools[name]
		if !ok {
			t.Fatalf("write tool %s not registered", name)
		}
		if _, gated := entry.Tool.InputSchema.Properties["confirm"]; !gated {
			t.Errorf("%s: a Cork write must carry the confirm gate", name)
		}
	}
	for name, entry := range tools {
		if _, ok := entry.Tool.InputSchema.Properties["yes"]; ok {
			t.Errorf("%s: --yes must never be a tool argument", name)
		}
		if ro := entry.Tool.Annotations.ReadOnlyHint; (ro != nil && *ro) || local[name] {
			if _, gated := entry.Tool.InputSchema.Properties["confirm"]; gated && ro != nil && *ro {
				t.Errorf("%s: a read tool should not need confirm", name)
			}
			continue
		}
		if _, gated := entry.Tool.InputSchema.Properties["confirm"]; !gated {
			t.Errorf("%s may change Cork data and has no confirm gate", name)
		}
	}
}

// The registered integrations_delete tool sends nothing to Cork until confirm
// is boolean true, then sends exactly one DELETE.
func TestEndpointWriteSendsOnlyWithConfirm(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.Method != http.MethodDelete {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("CORK_BASE_URL", srv.URL)
	t.Setenv("CORK_API_KEY", "dummy-key")
	s := server.NewMCPServer("cork-confirm", "test")
	RegisterTools(s)
	call := func(args map[string]any) *mcplib.CallToolResult {
		req := mcplib.CallToolRequest{}
		req.Params.Name, req.Params.Arguments = "integrations_delete", args
		res, err := s.GetTool("integrations_delete").Handler(context.Background(), req)
		if err != nil {
			t.Fatal(err)
		}
		return res
	}
	const id = "550e8400-e29b-41d4-a716-446655440000"
	for _, confirm := range []any{nil, false, "true", 1.0} {
		args := map[string]any{"integration-uuid": id}
		if confirm != nil {
			args["confirm"] = confirm
		}
		if res := call(args); res.IsError {
			t.Fatalf("confirm=%v: the preview must not be an error: %+v", confirm, res.Content)
		}
	}
	if n := hits.Load(); n != 0 {
		t.Fatalf("%d request(s) reached Cork without confirm", n)
	}
	call(map[string]any{"integration-uuid": id, "confirm": true})
	if n := hits.Load(); n != 1 {
		t.Fatalf("confirm=true: want exactly 1 DELETE, got %d", n)
	}
}
