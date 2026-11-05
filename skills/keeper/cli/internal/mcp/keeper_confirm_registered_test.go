// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package mcp

import (
	"testing"

	"github.com/mark3labs/mcp-go/server"
)

// No registered tool can change state without confirm (keeper handfix:
// mcp-mirror-write-confirm). Every endpoint tool is a GET marked read-only;
// every other tool is read-only or carries the confirm gate (posture report's
// local store). The learning writers (teach, learnings, playbook amend) are no
// tools at all: the MCP server writes no learning.
func TestRegisteredMirroredWritesNeedConfirm(t *testing.T) {
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
		if _, gated := entry.Tool.InputSchema.Properties["confirm"]; !gated {
			t.Errorf("%s may change state and has no confirm gate", name)
		}
	}
	if _, gated := tools["posture_report"].Tool.InputSchema.Properties["confirm"]; !gated {
		t.Errorf("posture_report: a local write must need confirm")
	}
	for _, name := range []string{"teach", "teach_pattern", "teach_lookup", "teach_playbook", "playbook_amend", "learnings_confirm", "learnings_reject", "learnings_forget"} {
		if _, ok := tools[name]; ok {
			t.Errorf("%s is an MCP tool; the MCP server must write no learning", name)
		}
	}
	for _, name := range []string{"companies_list", "companies_get", "users_list", "users_risk", "security_audit_report", "breachwatch_summary", "audit_trail_events", "events_list", "queue_get-status", "health_get"} {
		entry, ok := tools[name]
		if !ok {
			t.Fatalf("%s not registered", name)
		}
		if ro := entry.Tool.Annotations.ReadOnlyHint; ro == nil || !*ro {
			t.Errorf("%s: a Keeper read must be marked read-only", name)
		}
		if _, gated := entry.Tool.InputSchema.Properties["confirm"]; gated {
			t.Errorf("%s: read tool should not need confirm", name)
		}
	}
}
