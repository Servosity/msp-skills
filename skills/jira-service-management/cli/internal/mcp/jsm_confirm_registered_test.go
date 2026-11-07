// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package mcp

import (
	"testing"

	"github.com/mark3labs/mcp-go/server"
)

// No registered tool can change Jira Service Management data without confirm
// (handfix jsm-mcp-mirror-write-confirm). A tool must be read-only, write only
// the CLI's own files (local store, learnings, playbooks, saved Assets ids:
// listed here as local), or carry the confirm gate, so a command a later print
// adds is covered too.
func TestRegisteredMirroredWritesNeedConfirm(t *testing.T) {
	local := map[string]bool{
		"export": true, "sync": true, "workflow_archive": true, "learnings_forget": true, "learnings_reject": true,
		"learnings_confirm": true, "playbook_amend": true, "teach": true, "teach_lookup": true, "teach_pattern": true,
		"teach_playbook": true,
		// cmdb discover only reads the API; --save writes the CLI's config file.
		"cmdb_discover": true,
		// cmdb reconcile only reads Assets; its --file is refused over MCP.
		"cmdb_reconcile": true,
	}
	s := server.NewMCPServer("mirror-confirm", "test")
	RegisterTools(s)
	tools := s.ListTools()
	if len(tools) < 30 {
		t.Fatalf("only %d tools registered", len(tools))
	}
	for name, entry := range tools {
		if _, ok := entry.Tool.InputSchema.Properties["yes"]; ok {
			t.Errorf("%s: --yes must never be a tool argument", name)
		}
		if ro := entry.Tool.Annotations.ReadOnlyHint; ro != nil && *ro {
			continue
		}
		if local[name] {
			continue
		}
		if _, gated := entry.Tool.InputSchema.Properties["confirm"]; !gated {
			t.Errorf("%s may change Jira Service Management data and has no confirm gate", name)
		}
	}
	// Every JSM write reachable over MCP: the code-orchestration execute tool
	// and request note (posts an internal comment).
	for _, name := range []string{"jira-service-management_execute", "request_note"} {
		entry, ok := tools[name]
		if !ok {
			t.Fatalf("write tool %s not registered", name)
		}
		if _, gated := entry.Tool.InputSchema.Properties["confirm"]; !gated {
			t.Errorf("%s: a JSM write must carry the confirm gate", name)
		}
	}
}
