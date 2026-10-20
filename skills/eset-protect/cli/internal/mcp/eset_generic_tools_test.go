// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package mcp

import (
	"testing"

	"eset-protect-pp-cli/internal/cliutil"
	"github.com/mark3labs/mcp-go/server"
)

// TestGenericProjectManagementToolsAreNotOffered (adversarial round 5, P2):
// the MCP tool list offered the generator's 'load' ("workload per assignee"),
// 'orphans' ("missing assignee or project") and 'stale' tools in both modes,
// each answering confidently and wrongly on ESET data, 'stale' beside the real
// fleet_stale. None is offered in normal or read-only mode.
func TestGenericProjectManagementToolsAreNotOffered(t *testing.T) {
	for _, mode := range []string{"", "1"} {
		t.Setenv(cliutil.MCPReadOnlyEnvVar, mode)
		s := server.NewMCPServer("eset-protect", "test")
		RegisterTools(s)
		for _, name := range []string{"load", "orphans", "stale"} {
			if s.GetTool(name) != nil {
				t.Errorf("%s=%q: the generic %q tool is offered", cliutil.MCPReadOnlyEnvVar, mode, name)
			}
		}
		if s.GetTool("fleet_stale") == nil || s.GetTool("analytics") == nil {
			t.Errorf("%s=%q: fleet_stale or analytics went missing", cliutil.MCPReadOnlyEnvVar, mode)
		}
	}
}
