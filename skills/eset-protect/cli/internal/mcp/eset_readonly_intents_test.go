// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package mcp

import (
	"testing"

	"eset-protect-pp-cli/internal/cliutil"
	"github.com/mark3labs/mcp-go/server"
)

// TestReadOnlyModeHidesIsolateIntent guards the hand-fix
// mcp-read-only-hides-intents: isolate_a_machine fronts `respond isolate`, a
// write path, so ESET_PROTECT_MCP_READ_ONLY=1 must not offer it.
func TestReadOnlyModeHidesIsolateIntent(t *testing.T) {
	for _, tc := range []struct {
		env  string
		want bool
	}{{"", true}, {"1", false}} {
		t.Setenv(cliutil.MCPReadOnlyEnvVar, tc.env)
		s := server.NewMCPServer("eset-protect", "test")
		RegisterTools(s)
		if got := s.GetTool("isolate_a_machine") != nil; got != tc.want {
			t.Fatalf("%s=%q: isolate_a_machine registered = %v, want %v", cliutil.MCPReadOnlyEnvVar, tc.env, got, tc.want)
		}
	}
}
