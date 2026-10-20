// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/synology/handfixes.json
// under the "mcp-read-only-drops-endpoint-writes" entry.

package mcp

import (
	"testing"
)

var dsmWriteTools = []string{"abb_backup", "abb_cancel", "session_logout", "teach", "teach_playbook", "learnings_confirm", "playbook_amend", "fleet_sync", "sync"}

// TestReadOnlyEnvDropsEndpointWrites: SYNOLOGY_MCP_READ_ONLY used to reach only
// the Cobra-tree mirror, so abb_backup, abb_cancel and session_logout stayed
// listed and an MCP call started a backup on the NAS.
func TestReadOnlyEnvDropsEndpointWrites(t *testing.T) {
	for _, value := range []string{"1", "true", "yes"} {
		s := sweepMCPServer(t, nil, value)
		tools := s.ListTools()
		for _, name := range dsmWriteTools {
			if tools[name] != nil {
				t.Fatalf("SYNOLOGY_MCP_READ_ONLY=%s still lists %s", value, name)
			}
		}
		for name, st := range tools {
			if hint := st.Tool.Annotations.ReadOnlyHint; hint == nil || !*hint {
				t.Fatalf("SYNOLOGY_MCP_READ_ONLY=%s offers %s, which is not annotated read-only", value, name)
			}
		}
		for _, keep := range []string{"abb_tasks", "fleet_status", "why_did_a_task_fail", "context", "sql"} {
			if tools[keep] == nil {
				t.Fatalf("SYNOLOGY_MCP_READ_ONLY=%s dropped the read tool %s", value, keep)
			}
		}
	}
}

func TestFullModeKeepsEndpointWrites(t *testing.T) {
	s := sweepMCPServer(t, nil, "")
	for _, name := range dsmWriteTools {
		if s.ListTools()[name] == nil {
			t.Fatalf("full mode dropped %s", name)
		}
	}
}

// TestReadOnlyCallTimeBackstop: a write tool registered before the switch was
// turned on refuses at call time and sends nothing.
func TestReadOnlyCallTimeBackstop(t *testing.T) {
	fake := newFakeDSM(t, "Office PCs")
	s := sweepMCPServer(t, fake, "")
	t.Setenv("SYNOLOGY_MCP_READ_ONLY", "1")
	if text, isErr := callTool(t, s, "abb_backup", map[string]any{"task_ids": "[5]"}); !isErr {
		t.Fatalf("abb_backup ran while the switch was on: %s", text)
	}
	if len(fake.Calls()) != 0 {
		t.Fatalf("the refused write reached the NAS: %+v", fake.Calls())
	}
}
