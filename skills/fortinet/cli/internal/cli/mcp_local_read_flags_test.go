// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"testing"

	"fortinet-pp-cli/internal/mcp/cobratree"
	"github.com/spf13/cobra"
)

// TestMCPCannotChooseFleetFiles pins that an MCP tool call cannot name the local
// files the fleet commands read. The MCP server runs this CLI as the account
// that hosts it. A forwarded --devices let a tool call point a fleet command at
// any devices file that account can reach, and a devices file names each
// device's API URL and token_file, so the CLI would read that token file and
// send it to that URL. A forwarded `fleet changes --dir` let it read and print
// diffs of files from any directory. The operator still passes both flags on
// the command line (or sets FORTINET_DEVICES); only the MCP surface drops them
// (mcp:write-flags). Every command that carries --devices is checked, so a new
// fleet command inherits the block through addFleetFlags. --device (a list of
// names, not a path) must still be forwarded: the block is not a blanket one.
func TestMCPCannotChooseFleetFiles(t *testing.T) {
	root := RootCmd()
	var checked int
	var walk func(*cobra.Command)
	walk = func(cmd *cobra.Command) {
		if f := cmd.LocalFlags().Lookup("devices"); f != nil {
			checked++
			if !cobratree.DestinationFlagBlocked(cmd, "devices") {
				t.Errorf("%s: an MCP tool call can still set --devices, a path on this account's disk", cmd.CommandPath())
			}
			if cmd.LocalFlags().Lookup("device") != nil && cobratree.DestinationFlagBlocked(cmd, "device") {
				t.Errorf("%s: --device is blocked too; the MCP block must stay specific to --devices", cmd.CommandPath())
			}
		}
		for _, c := range cmd.Commands() {
			walk(c)
		}
	}
	walk(root)
	if checked < 9 {
		t.Fatalf("found %d commands with --devices, want at least the 9 fleet commands", checked)
	}
	changes, _, err := root.Find([]string{"fleet", "changes"})
	if err != nil || changes == root {
		t.Fatalf("fleet changes not found: %v", err)
	}
	if !cobratree.DestinationFlagBlocked(changes, "dir") {
		t.Errorf("%s: an MCP tool call can still set --dir, a directory on this account's disk", changes.CommandPath())
	}
	if cobratree.DestinationFlagBlocked(changes, "since") {
		t.Errorf("%s: --since is blocked too; the MCP block must stay specific to --dir", changes.CommandPath())
	}
}
