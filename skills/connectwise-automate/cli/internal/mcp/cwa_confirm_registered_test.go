// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package mcp

import (
	"testing"

	"github.com/mark3labs/mcp-go/server"
)

// No registered tool can change ConnectWise Automate data without confirm
// (connectwise-automate handfixes: mcp-endpoint-write-confirm,
// mcp-mirror-write-confirm). A tool must either be read-only, write only the
// CLI's own files (local store, learnings, playbooks: listed here as local), or
// carry the confirm gate, or this test fails; so an endpoint or command a later
// print adds is covered too.
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
			t.Errorf("%s may change state and has no confirm gate", name)
		}
	}
	// Every ConnectWise Automate write reachable over MCP: governance.md's
	// Endpoint and fleet actions tier, the token endpoints and bulk import.
	for _, name := range []string{"computers_command-execute", "patching_deploy-approved", "patching_deploy-security",
		"patching_reattempt-failed", "apitoken_mint", "apitoken_refresh", "import"} {
		entry, ok := tools[name]
		if !ok {
			t.Fatalf("write tool %s not registered", name)
		}
		if _, gated := entry.Tool.InputSchema.Properties["confirm"]; !gated {
			t.Errorf("%s: a ConnectWise Automate write must carry the confirm gate", name)
		}
	}
	for _, name := range []string{"computers_list", "computers_get", "alerts_list", "clients_list", "fleet_health",
		"stale_agents", "patch_compliance", "client_rollup", "alert_triage", "os_inventory", "since", "sql", "search"} {
		entry, ok := tools[name]
		if !ok {
			t.Fatalf("%s not registered", name)
		}
		if ro := entry.Tool.Annotations.ReadOnlyHint; ro == nil || !*ro {
			t.Errorf("%s: a ConnectWise Automate read must be marked read-only", name)
		}
		if _, gated := entry.Tool.InputSchema.Properties["confirm"]; gated {
			t.Errorf("%s: read tool should not need confirm", name)
		}
	}
}
