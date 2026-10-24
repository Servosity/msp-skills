// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
package mcp

import (
	"testing"

	"cove-pp-cli/internal/cli"
	"cove-pp-cli/internal/mcp/cobratree"
)

// auth token prints the session visa; it must never become an MCP tool.
func TestAuthTokenIsNotAnMCPTool(t *testing.T) {
	root := cli.RootCmd()
	cmd := cobratree.CommandAtPath(root, []string{"auth", "token"})
	if cmd == nil || cmd.Name() != "token" {
		t.Fatalf("auth token command not found")
	}
	if cobratree.CommandExposableOverMCP(cmd) {
		t.Fatalf("auth token is exposed over MCP; it would print the session visa to the agent")
	}
}
