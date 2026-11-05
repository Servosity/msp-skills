// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package mcp

import (
	"testing"

	"github.com/mark3labs/mcp-go/server"
)

// The mirrored commands that change PRTG state carry the confirm gate in the
// real registration (prtg handfix: mcp-mirror-write-confirm).
func TestRegisteredMirroredWritesNeedConfirm(t *testing.T) {
	s := server.NewMCPServer("mirror-confirm", "test")
	RegisterTools(s)
	tools := s.ListTools()
	for _, name := range []string{"fleet_ack", "fleet_pause", "fleet_resume", "import"} {
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
	for _, name := range []string{"sync", "export", "fleet_sync", "workflow_archive", "fleet_exec", "fleet_alarms", "fleet_status"} {
		if entry, ok := tools[name]; ok {
			if _, gated := entry.Tool.InputSchema.Properties["confirm"]; gated {
				t.Errorf("%s: read or local-only tool should not need confirm", name)
			}
		}
	}
}
