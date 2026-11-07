// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package mcp

import (
	"testing"

	"github.com/mark3labs/mcp-go/server"
)

// No registered tool can change Tactical RMM data without confirm
// (tactical-rmm handfix mcp-mirror-write-confirm). A tool must be read-only,
// write only the CLI's own files (local store, learnings, playbooks: listed
// here as local), or carry the confirm gate, so a command a later print adds is
// covered too.
func TestRegisteredMirroredWritesNeedConfirm(t *testing.T) {
	local := map[string]bool{
		"export": true, "sync": true, "workflow_archive": true, "learnings_forget": true, "learnings_reject": true,
		"learnings_confirm": true, "playbook_amend": true, "teach": true, "teach_lookup": true, "teach_pattern": true,
		"teach_playbook": true,
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
			t.Errorf("%s may change Tactical RMM data and has no confirm gate", name)
		}
	}
	// Every Tactical RMM write reachable over MCP: the code-orchestration
	// execute tool, the two cohort writers, import, and services down (it
	// dispatches a services query to every agent it scans).
	for _, name := range []string{"tactical-rmm_execute", "agents_bulk_run", "maintenance_set", "import", "services_down"} {
		entry, ok := tools[name]
		if !ok {
			t.Fatalf("write tool %s not registered", name)
		}
		if _, gated := entry.Tool.InputSchema.Properties["confirm"]; !gated {
			t.Errorf("%s: a Tactical RMM write must carry the confirm gate", name)
		}
	}
}
