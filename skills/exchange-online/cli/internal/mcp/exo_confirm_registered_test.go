// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package mcp

import (
	"testing"

	"github.com/mark3labs/mcp-go/server"
)

// The mirrored commands that change Exchange state carry the confirm gate in
// the real registration (exchange-online handfix: mcp-mirror-write-confirm),
// and the read-only novels and local-only commands do not.
func TestRegisteredMirroredWritesNeedConfirm(t *testing.T) {
	s := server.NewMCPServer("mirror-confirm", "test")
	RegisterTools(s)
	tools := s.ListTools()
	for _, name := range []string{"offboard"} {
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
	for _, name := range []string{"ews_posture", "bec_triage", "where_is", "access_map", "quarantine_queue",
		"quarantine_snapshot", "inbox_rules_sweep", "forwarding_audit"} {
		entry, ok := tools[name]
		if !ok {
			t.Fatalf("%s not registered", name)
		}
		if _, gated := entry.Tool.InputSchema.Properties["confirm"]; gated {
			t.Errorf("%s: read or local-only tool should not need confirm", name)
		}
	}
	fa := tools["forwarding_audit"].Tool
	if _, ok := fa.InputSchema.Properties["sweep"]; ok {
		t.Error("forwarding_audit: sweep must not be a tool argument; inbox_rules_sweep sweeps")
	}
	if fa.Annotations.ReadOnlyHint == nil || !*fa.Annotations.ReadOnlyHint {
		t.Error("forwarding_audit: must be marked read-only in every mode")
	}
}
