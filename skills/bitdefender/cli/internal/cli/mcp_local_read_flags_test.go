// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"testing"

	"bitdefender-pp-cli/internal/mcp/cobratree"
)

// TestMCPCannotChooseLocalInputFiles pins that an MCP tool call cannot name the
// local file these commands read. The MCP server runs this CLI as the account
// that hosts it, so a forwarded --devices or --agreements let a tool call read
// any regular file that account can reach and echo the parsed rows back. The
// operator still passes the flag on the command line; only the MCP surface
// drops it (mcp:write-flags). The second assertion is the other direction: an
// ordinary flag on the same command must still be forwarded, so the block is
// not a blanket one.
func TestMCPCannotChooseLocalInputFiles(t *testing.T) {
	root := RootCmd()
	for _, tc := range []struct {
		path      []string
		localFile string
		ordinary  string
	}{
		{[]string{"fleet", "rmm-gap"}, "devices", "max-pages"},
		{[]string{"usage", "billing"}, "agreements", "month"},
	} {
		cmd, _, err := root.Find(tc.path)
		if err != nil || cmd == root {
			t.Fatalf("%v: command not found: %v", tc.path, err)
		}
		if cmd.Flags().Lookup(tc.localFile) == nil || cmd.Flags().Lookup(tc.ordinary) == nil {
			t.Fatalf("%v: expected flags --%s and --%s", tc.path, tc.localFile, tc.ordinary)
		}
		if !cobratree.DestinationFlagBlocked(cmd, tc.localFile) {
			t.Errorf("%s: an MCP tool call can still set --%s, a path on this account's disk", cmd.CommandPath(), tc.localFile)
		}
		if cobratree.DestinationFlagBlocked(cmd, tc.ordinary) {
			t.Errorf("%s: --%s is blocked too; the MCP block must stay specific to the local-file flag", cmd.CommandPath(), tc.ordinary)
		}
	}
}
