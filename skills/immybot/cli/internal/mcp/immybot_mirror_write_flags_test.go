// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): a mirrored novel tool that can write must never
// carry readOnlyHint=true while its write flag is reachable, and with the MCP
// read-only switch on the write flag must be gone from the tool.

package mcp

import (
	"testing"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"immybot-pp-cli/internal/cliutil"
)

// novelWriteFlags lists each hand-written novel command and the flag that turns
// its run into a write (fleet-diff --snapshot records a baseline in the store).
var novelWriteFlags = map[string]string{
	"fleet-diff": "snapshot",
}

func mirroredTool(t *testing.T, s *server.MCPServer, cliCommand string) *mcplib.Tool {
	t.Helper()
	for _, entry := range s.ListTools() {
		if entry == nil || entry.Tool.Meta == nil {
			continue
		}
		if v, _ := entry.Tool.Meta.AdditionalFields["pp:cli-command"].(string); v == cliCommand {
			tool := entry.Tool
			return &tool
		}
	}
	return nil
}

func TestMirroredNovelWriteFlagsAreNeverReadOnly(t *testing.T) {
	for command, flag := range novelWriteFlags {
		t.Setenv(cliutil.MCPReadOnlyEnvVar, "")
		s := server.NewMCPServer("write-flags", "test")
		RegisterTools(s)
		tool := mirroredTool(t, s, command)
		if tool == nil {
			t.Fatalf("%s: no mirrored MCP tool", command)
		}
		_, hasFlag := tool.InputSchema.Properties[flag]
		if hasFlag && tool.Annotations.ReadOnlyHint != nil && *tool.Annotations.ReadOnlyHint {
			t.Errorf("%s: tool exposes write flag %q yet is annotated readOnlyHint=true", command, flag)
		}

		t.Setenv(cliutil.MCPReadOnlyEnvVar, "1")
		ro := server.NewMCPServer("write-flags-ro", "test")
		RegisterTools(ro)
		if tool := mirroredTool(t, ro, command); tool != nil {
			if _, ok := tool.InputSchema.Properties[flag]; ok {
				t.Errorf("%s: with %s=1 the tool still accepts write flag %q", command, cliutil.MCPReadOnlyEnvVar, flag)
			}
		}
	}
}
