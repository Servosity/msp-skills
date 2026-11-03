// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package mcp

import (
	"testing"

	"github.com/mark3labs/mcp-go/server"
)

// No registered tool can change Betterstack data without confirm (betterstack handfixes:
// mcp-endpoint-write-confirm, mcp-mirror-write-confirm). A tool must either be
// read-only, write only the CLI's own files (local store, learnings,
// playbooks: listed here as local), or carry the confirm gate, or this test
// fails; so an endpoint or command a later print adds is covered too.
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
	// Every Better Stack write reachable over MCP: the typed endpoint writes
	// (monitor, heartbeat, group, policy, status page and section create,
	// update and delete; incident acknowledge, resolve and delete) and import
	// (governance.md's Write tier). Better Stack's novels only read.
	for _, name := range []string{"monitors_create", "monitors_update", "monitors_delete", "monitor-groups_create",
		"monitor-groups_delete", "heartbeats_create", "heartbeats_update", "heartbeats_delete", "heartbeat-groups_create",
		"heartbeat-groups_delete", "incidents_acknowledge", "incidents_resolve", "incidents_delete", "policies_create",
		"policies_delete", "status-pages_create", "status-pages_delete", "status-page-sections_create",
		"status-page-sections_delete", "status-page-resources_delete", "import"} {
		entry, ok := tools[name]
		if !ok {
			t.Fatalf("write tool %s not registered", name)
		}
		if _, gated := entry.Tool.InputSchema.Properties["confirm"]; !gated {
			t.Errorf("%s: a Better Stack write must carry the confirm gate", name)
		}
	}
	for _, name := range []string{"monitors_list", "monitors_get", "heartbeats_list", "incidents_list", "on-calls_list",
		"policies_list", "status-pages_list", "fleet", "coverage", "mttr", "flapping", "oncall_gaps", "heartbeat_risk",
		"down", "triage", "statuspage_audit", "group_health", "sql", "search"} {
		entry, ok := tools[name]
		if !ok {
			t.Fatalf("%s not registered", name)
		}
		if ro := entry.Tool.Annotations.ReadOnlyHint; ro == nil || !*ro {
			t.Errorf("%s: a Better Stack read must be marked read-only", name)
		}
		if _, gated := entry.Tool.InputSchema.Properties["confirm"]; gated {
			t.Errorf("%s: read tool should not need confirm", name)
		}
	}
}
