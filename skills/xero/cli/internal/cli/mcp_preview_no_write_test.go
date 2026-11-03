// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written (NOT generated): pinned by handfixes.json under "mcp-preview-no-local-write".

package cli

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"xero-pp-cli/internal/cliutil"
	"xero-pp-cli/internal/cliutil/testenv"
)

func filesUnder(t *testing.T, root string) []string {
	t.Helper()
	var out []string
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			out = append(out, p)
		}
		return nil
	})
	sort.Strings(out)
	return out
}

// runProduction drives the production wrapper (Execute), the path the MCP
// server's child process takes, not RootCmd().Execute().
func runProduction(t *testing.T, args ...string) {
	t.Helper()
	old := os.Args
	t.Cleanup(func() { os.Args = old })
	os.Args = append([]string{"xero-cli"}, args...)
	_ = Execute()
}

// An MCP call that is not confirmed must leave no local state: a read-only
// tool runs in a child process the MCP server marks with XERO_MCP_CHILD, and
// an unconfirmed write previews as the command's own --dry-run. Neither may
// create the learn journal, the store, or any other file.
func TestUnconfirmedMCPCallsWriteNoLocalState(t *testing.T) {
	for _, argv := range [][]string{
		{"aging", "--json"},
		{"reconcile", "--json"},
		{"since", "7d", "--json"},
		{"sync", "--dry-run", "--json"},
		{"sync", "--dry-run", "--full", "--json"},
	} {
		home := testenv.Isolate(t, cliutil.ConfigDir, cliutil.DataDir, cliutil.StateDir, cliutil.CacheDir)
		t.Setenv(cliutil.MCPChildEnvVar, "1")
		t.Setenv("XERO_ACCESS_TOKEN", "test-token")
		t.Setenv("XERO_TENANT_ID", "tenant")
		before := filesUnder(t, home)
		runProduction(t, argv...)
		after := filesUnder(t, home)
		if len(after) != len(before) {
			t.Errorf("%v wrote local state: before %v, after %v", argv, before, after)
		}
	}
}
