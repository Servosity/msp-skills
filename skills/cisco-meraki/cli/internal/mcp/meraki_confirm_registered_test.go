// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package mcp

import (
	"testing"

	"github.com/mark3labs/mcp-go/server"
)

// No registered tool can change Meraki Dashboard data without confirm
// (cisco-meraki handfix meraki-mcp-mirror-write-confirm). A tool must be read-only,
// write only the CLI's own files (local store, learnings, playbooks: listed
// here as local), or carry the confirm gate, so a command a later print adds is
// covered too.
func TestRegisteredMirroredWritesNeedConfirm(t *testing.T) {
	local := map[string]bool{
		"export": true, "sync": true, "workflow_archive": true, "learnings_forget": true, "learnings_reject": true,
		"learnings_confirm": true, "playbook_amend": true, "teach": true, "teach_lookup": true, "teach_pattern": true,
		"teach_playbook": true,
		// recipe: runs `fleet find <ref> --agent`, a read
		"locate_a_mac_from_a_soc_alert": true,
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
			t.Errorf("%s may change Meraki Dashboard data and has no confirm gate", name)
		}
	}
	// Every Meraki write reachable over MCP: the code-orchestration execute
	// tool, fleet push (--apply sends action batches) and import.
	for _, name := range []string{"cisco-meraki_execute", "fleet_push", "import"} {
		entry, ok := tools[name]
		if !ok {
			t.Fatalf("write tool %s not registered", name)
		}
		if _, gated := entry.Tool.InputSchema.Properties["confirm"]; !gated {
			t.Errorf("%s: a Meraki write must carry the confirm gate", name)
		}
	}
}
