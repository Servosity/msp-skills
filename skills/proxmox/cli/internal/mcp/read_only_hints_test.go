// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/proxmox/handfixes.json under
// "mcp-read-only-hints".

package mcp

import (
	"testing"

	"github.com/mark3labs/mcp-go/server"
)

// Every tool reaches Proxmox with GETs only, so none may advertise itself as
// destructive: an MCP client gating on hints would treat reads as writes.
func TestToolsAdvertiseReadOnly(t *testing.T) {
	s := server.NewMCPServer("proxmox", "test")
	RegisterCodeOrchestrationTools(s)
	RegisterIntents(s)
	for name, tool := range s.ListTools() {
		a := tool.Tool.Annotations
		if a.ReadOnlyHint == nil || !*a.ReadOnlyHint || a.DestructiveHint == nil || *a.DestructiveHint {
			t.Errorf("%s: readOnlyHint=%v destructiveHint=%v", name, a.ReadOnlyHint, a.DestructiveHint)
		}
	}
}
