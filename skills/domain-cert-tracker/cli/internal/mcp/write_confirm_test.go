// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package mcp

import (
	"testing"

	"github.com/mark3labs/mcp-go/server"
)

// Every MCP tool that can write (no read-only hint) takes confirm, and the
// companion CLI refuses to run it under MCP until confirm is true (proved
// per command in internal/cli/mcp_confirm_test.go).
func TestEveryMCPWriteToolTakesConfirm(t *testing.T) {
	s := server.NewMCPServer("domain-cert-tracker", "test")
	RegisterTools(s)
	writes := 0
	for name, tool := range s.ListTools() {
		if h := tool.Tool.Annotations.ReadOnlyHint; h != nil && *h {
			continue
		}
		writes++
		if _, ok := tool.Tool.InputSchema.Properties["confirm"]; !ok {
			t.Errorf("MCP write tool %s has no confirm argument", name)
		}
	}
	if writes == 0 {
		t.Fatal("no MCP write tools found; the test is not looking at the real surface")
	}
}
