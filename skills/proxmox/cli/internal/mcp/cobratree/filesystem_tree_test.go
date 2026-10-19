// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/proxmox/handfixes.json
// under the "mcp-filesystem-destination-gate" entry. External test package so it
// can import internal/cli.

package cobratree_test

import (
	"testing"

	"proxmox-pp-cli/internal/cli"
	"proxmox-pp-cli/internal/mcp/cobratree"
)

// TestNoShellOutToolCanDirectAFilesystemPath walks the real command tree the
// MCP server mirrors and fails when any shell-out tool still forwards a flag
// whose own usage text says it names a filesystem location, so a regenerated
// local-path flag fails the build instead of becoming a file-read primitive.
func TestNoShellOutToolCanDirectAFilesystemPath(t *testing.T) {
	leaks := cobratree.UnblockedFilesystemPathFlags(cli.RootCmd())
	if len(leaks) > 0 {
		t.Fatalf("MCP shell-out tools still forward filesystem-path flags: %v\nadd each flag name to blockedDestinationFlags in internal/mcp/cobratree/shellout.go", leaks)
	}
}
