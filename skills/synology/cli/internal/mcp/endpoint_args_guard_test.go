// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/synology/handfixes.json
// under the "mcp-writes-refuse-unknown-args" entry.

package mcp

import (
	"strings"
	"testing"
)

func methodCalls(f *fakeDSM, method string) int {
	n := 0
	for _, c := range f.Calls() {
		if c.Method == method {
			n++
		}
	}
	return n
}

// TestEndpointWriteRefusesUndeclaredArgument: an agent that passes the CLI
// spelling dry-run to abb_backup must get a refusal, not a live backup with an
// inert query parameter.
func TestEndpointWriteRefusesUndeclaredArgument(t *testing.T) {
	fake := newFakeDSM(t, "Office PCs")
	s := sweepMCPServer(t, fake, "")
	for _, tool := range []string{"abb_backup", "abb_cancel"} {
		text, isErr := callTool(t, s, tool, map[string]any{"task_ids": "[5]", "dry-run": true})
		if !isErr {
			t.Fatalf("%s with an undeclared dry-run argument ran: %s", tool, text)
		}
	}
	if text, isErr := callTool(t, s, "session_logout", map[string]any{"force": true}); !isErr {
		t.Fatalf("session_logout with an undeclared argument ran: %s", text)
	}
	if n := methodCalls(fake, "backup") + methodCalls(fake, "cancel") + methodCalls(fake, "logout"); n != 0 {
		t.Fatalf("%d DSM write call(s) reached the NAS: %+v", n, fake.Calls())
	}
	if text, isErr := callTool(t, s, "abb_backup", map[string]any{"task_ids": "[5]", "confirm": true}); isErr {
		t.Fatalf("declared abb_backup call refused: %s", text)
	}
	if methodCalls(fake, "backup") != 1 {
		t.Fatalf("declared abb_backup call did not reach the NAS: %+v", fake.Calls())
	}
}

// TestReadToolRefusesOperationOverride: every DSM operation is a GET on one
// CGI path selected by api= and method=, and the client merges extra
// arguments into that query with Set, so {"method":"backup"} on the
// read-only abb_tasks tool used to start a backup.
func TestReadToolRefusesOperationOverride(t *testing.T) {
	fake := newFakeDSM(t, "Office PCs")
	s := sweepMCPServer(t, fake, "1")
	for _, tc := range []struct {
		tool string
		args map[string]any
	}{
		{"abb_tasks", map[string]any{"method": "backup", "task_ids": "[5]", "trigger_type": "1"}},
		{"abb_settings", map[string]any{"api": "SYNO.ActiveBackup.Task", "method": "cancel", "task_ids": "[5]"}},
		{"abb_tasks", map[string]any{"Method": "backup"}},
		{"system_get", map[string]any{"version": "1"}},
		{"abb_tasks", map[string]any{"load_status": "false"}},
		{"abb_tasks", map[string]any{"_sid": "other"}},
	} {
		if text, isErr := callTool(t, s, tc.tool, tc.args); !isErr {
			t.Fatalf("%s %v was not refused: %s", tc.tool, tc.args, text)
		}
	}
	if len(fake.Calls()) != 0 {
		t.Fatalf("a refused call reached the NAS: %+v", fake.Calls())
	}
	// Press 4.34.2 refuses every argument the tool schema does not declare,
	// reads included, so even an extra DSM filter that overrides nothing is
	// refused before any request is sent.
	if text, isErr := callTool(t, s, "abb_tasks", map[string]any{"sort_by": "task_name"}); !isErr || !strings.Contains(text, "sort_by") {
		t.Fatalf("an undeclared read argument was not refused: %s", text)
	}
	if calls := fake.Calls(); len(calls) != 0 {
		t.Fatalf("a refused call reached the NAS: %+v", calls)
	}
}
