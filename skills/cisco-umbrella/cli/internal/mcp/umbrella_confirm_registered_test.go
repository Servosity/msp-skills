// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package mcp

import (
	"testing"

	"github.com/mark3labs/mcp-go/server"
)

// The mirrored commands that change Umbrella state carry the confirm gate in
// the real registration (umbrella handfix: mcp-mirror-write-confirm), and the
// read-only novels and local-only commands do not.
func TestRegisteredMirroredWritesNeedConfirm(t *testing.T) {
	s := server.NewMCPServer("mirror-confirm", "test")
	RegisterTools(s)
	tools := s.ListTools()
	for _, name := range []string{"import", "destination_lists_diff", "block_one_domain"} {
		entry, ok := tools[name]
		if !ok {
			t.Fatalf("%s not registered", name)
		}
		if _, ok := entry.Tool.InputSchema.Properties["confirm"]; !ok {
			t.Errorf("%s: no confirm gate", name)
		}
		if _, ok := entry.Tool.InputSchema.Properties["yes"]; ok {
			t.Errorf("%s: --yes must never be a tool argument", name)
		}
	}
	for _, name := range []string{"sync", "export", "workflow_archive", "verdict", "fleet_health", "hygiene", "report_card", "allowlist_audit"} {
		entry, ok := tools[name]
		if !ok {
			t.Fatalf("%s not registered", name)
		}
		if _, gated := entry.Tool.InputSchema.Properties["confirm"]; gated {
			t.Errorf("%s: read or local-only tool should not need confirm", name)
		}
	}
}
