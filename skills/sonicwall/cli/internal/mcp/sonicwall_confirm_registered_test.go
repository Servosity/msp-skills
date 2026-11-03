// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package mcp

import (
	"testing"

	"github.com/mark3labs/mcp-go/server"
)

// No registered tool can call SonicWall with a side effect, or mint and save a
// token, without confirm (handfix mcp-mirror-write-confirm). A tool must be
// read-only, write only the CLI's own files (listed here as local), or carry the
// confirm gate, so a command a later print adds is covered too.
func TestRegisteredMirroredWritesNeedConfirm(t *testing.T) {
	local := map[string]bool{
		"export": true, "sync": true, "workflow_archive": true, "learnings_forget": true, "learnings_reject": true,
		"learnings_confirm": true, "playbook_amend": true, "teach": true, "teach_lookup": true, "teach_pattern": true,
		"teach_playbook": true,
	}
	s := server.NewMCPServer("mirror-confirm", "test")
	RegisterTools(s)
	tools := s.ListTools()
	for _, must := range []string{"nsm_login", "nsm_get", "import", "sonicwall_execute"} {
		if _, ok := tools[must]; !ok {
			t.Fatalf("%s is not registered", must)
		}
	}
	for name, entry := range tools {
		if _, ok := entry.Tool.InputSchema.Properties["yes"]; ok {
			t.Errorf("%s: --yes must never be a tool argument", name)
		}
		if ro := entry.Tool.Annotations.ReadOnlyHint; (ro != nil && *ro) || local[name] {
			continue
		}
		if _, gated := entry.Tool.InputSchema.Properties["confirm"]; !gated {
			t.Errorf("%s may call SonicWall or change saved credentials and has no confirm gate", name)
		}
	}
}
