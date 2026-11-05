// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/zammad/handfixes.json under the
// "mcp-vendor-writes-need-confirm" entry. External test package so it can
// import internal/cli even where internal/cli imports internal/mcp.

package cobratree_test

import (
	"sort"
	"testing"

	"github.com/mark3labs/mcp-go/server"

	"zammad-pp-cli/internal/cli"
	"zammad-pp-cli/internal/mcp/cobratree"
)

// mcpStateChangingAllowed is every mirrored tool that is not marked
// read-only. None sends a write to Zammad: the press's local-store tools
// (sync, articles_sync, export, import, workflow_archive) and learn-store
// tools. A vendor write goes through zammad_execute, which previews until
// confirm is true; that tool is registered elsewhere, not by this walker.
var mcpStateChangingAllowed = map[string]bool{
	"articles_sync": true, "export": true, "import": true, "sync": true, "workflow_archive": true,
	"learnings_confirm": true, "learnings_forget": true, "learnings_reject": true, "playbook_amend": true,
	"playbook_list": true, "recall": true, "teach": true, "teach_lookup": true, "teach_pattern": true,
	"teach_playbook": true,
}

// mcpConfirmGatedWrites send a write to Zammad only when the call carries
// confirm: true (the MCP form of --yes, which the mirror blocks); without it
// they print the plan and write nothing (webhooks.go).
var mcpConfirmGatedWrites = map[string]bool{"webhooks_register": true, "webhooks_unregister": true}

// TestNoMirroredToolWritesWithoutConfirm: ticket note posted an article over
// MCP with no confirm step (Codex r1 [P1] on the 4.34.0 publish). The class:
// every mirrored tool that is not read-only must be on the allowlist above,
// so a new vendor write fails the build instead of shipping without confirm.
func TestNoMirroredToolWritesWithoutConfirm(t *testing.T) {
	s := server.NewMCPServer("t", "0")
	cobratree.RegisterAll(s, cli.RootCmd(), func() (string, error) { return "zammad-cli", nil })
	var bad []string
	for name, tool := range s.ListTools() {
		ro := tool.Tool.Annotations.ReadOnlyHint
		if mcpConfirmGatedWrites[name] {
			if _, ok := tool.Tool.InputSchema.Properties["confirm"]; !ok {
				bad = append(bad, name+" (no confirm argument)")
			}
			continue
		}
		if (ro == nil || !*ro) && !mcpStateChangingAllowed[name] {
			bad = append(bad, name)
		}
	}
	sort.Strings(bad)
	if len(bad) > 0 {
		t.Fatalf("MCP mirrors tools that may change state and are not on the allowlist: %v\nmark a vendor write mcp:hidden (zammad_execute carries the confirm step), or add a local-only tool to mcpStateChangingAllowed", bad)
	}
	if _, ok := s.ListTools()["ticket_note"]; ok {
		t.Fatal("ticket_note is exposed over MCP")
	}
}
