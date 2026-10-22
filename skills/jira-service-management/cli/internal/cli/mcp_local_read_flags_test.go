// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"testing"

	"jira-service-management-pp-cli/internal/mcp/cobratree"
)

// TestMCPCannotChooseTheReconcileExport pins that an MCP tool call cannot name
// the local export `cmdb reconcile` reads. The MCP server runs this CLI as the
// account that hosts it, so a forwarded --file let a tool call read any
// regular file that account can reach and echo its rows back in the diff. The
// operator still passes --file on the command line; only the MCP surface drops
// it (mcp:write-flags). The second assertion is the other direction: --key,
// an ordinary flag on the same command, must still be forwarded.
func TestMCPCannotChooseTheReconcileExport(t *testing.T) {
	root := RootCmd()
	cmd, _, err := root.Find([]string{"cmdb", "reconcile"})
	if err != nil || cmd == root {
		t.Fatalf("cmdb reconcile not found: %v", err)
	}
	if cmd.Flags().Lookup("file") == nil || cmd.Flags().Lookup("key") == nil {
		t.Fatal("cmdb reconcile: expected --file and --key")
	}
	if !cobratree.DestinationFlagBlocked(cmd, "file") {
		t.Errorf("%s: an MCP tool call can still set --file, a path on this account's disk", cmd.CommandPath())
	}
	if cobratree.DestinationFlagBlocked(cmd, "key") {
		t.Errorf("%s: --key is blocked too; the MCP block must stay specific to --file", cmd.CommandPath())
	}
}
