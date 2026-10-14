// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/cisco-meraki/handfixes.json
// under "mcp-filesystem-source-gate". External test package so it can import
// internal/cli.

package cobratree_test

import (
	"context"
	"strings"
	"testing"

	"cisco-meraki-pp-cli/internal/cli"
	"cisco-meraki-pp-cli/internal/mcp/cobratree"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// TestNoShellOutToolCanDirectAFilesystemPath fails when any mirrored MCP tool
// still forwards a flag whose usage text names a local file or directory.
func TestNoShellOutToolCanDirectAFilesystemPath(t *testing.T) {
	if leaks := cobratree.UnblockedFilesystemPathFlags(cli.RootCmd()); len(leaks) > 0 {
		t.Fatalf("MCP shell-out tools still forward filesystem-path flags: %v\nadd each to blockedDestinationFlags in internal/mcp/cobratree/shellout.go", leaks)
	}
}

// TestFleetPushBodyIsInlineOnlyOverMCP drives the registered fleet_push tool:
// a file path for --body is refused before the CLI runs, while inline JSON is
// forwarded (the fake CLI path then fails, proving the guard let it through).
func TestFleetPushBodyIsInlineOnlyOverMCP(t *testing.T) {
	s := server.NewMCPServer("t", "0")
	cobratree.RegisterAll(s, cli.RootCmd(), func() (string, error) { return "/nonexistent/cisco-meraki-cli", nil })
	tool := s.GetTool("fleet_push")
	if tool == nil {
		t.Fatal("fleet_push tool not registered")
	}
	call := func(body string) string {
		req := mcplib.CallToolRequest{}
		req.Params.Name = "fleet_push"
		req.Params.Arguments = map[string]any{"path": "/networks/{networkId}/snmp", "body": body}
		res, err := tool.Handler(context.Background(), req)
		if err != nil {
			t.Fatal(err)
		}
		return res.Content[0].(mcplib.TextContent).Text
	}
	if got := call("/etc/hosts"); !strings.Contains(got, "inline JSON object over MCP") {
		t.Fatalf("file path for --body was not refused: %s", got)
	}
	if got := call(`{"access":"none"}`); strings.Contains(got, "inline JSON object over MCP") {
		t.Fatalf("inline JSON body was refused: %s", got)
	}
}
