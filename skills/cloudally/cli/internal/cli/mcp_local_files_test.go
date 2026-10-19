// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"testing"

	"cloudally-pp-cli/internal/mcp/cobratree"
)

// A flag that names a local file must never reach an MCP caller: a read
// path echoes file lines back as warnings, a write path truncates any file
// the server can write. Both held on read-only tools before this guard.
func TestMCPLocalFileFlagsBlocked(t *testing.T) {
	root := RootCmd()
	for _, tc := range []struct {
		path  []string
		flags []string
	}{
		{[]string{"restore", "plan"}, []string{"plan-out", "entities-file"}},
		{[]string{"restore", "submit"}, []string{"plan-file", "entities-file"}},
		{[]string{"download", "submit"}, []string{"plan-out", "entities-file"}},
	} {
		cmd := cobratree.CommandAtPath(root, tc.path)
		if cmd == nil {
			t.Fatalf("%v: command not found", tc.path)
		}
		for _, f := range tc.flags {
			if cmd.Flags().Lookup(f) == nil {
				t.Fatalf("%v --%s: flag not declared", tc.path, f)
			}
			if !cobratree.DestinationFlagBlocked(cmd, f) {
				t.Errorf("%v --%s reaches MCP callers", tc.path, f)
			}
		}
	}
	if c := cobratree.CommandAtPath(root, []string{"download", "fetch"}); c == nil || c.Annotations[cobratree.HiddenAnnotation] != "true" {
		t.Errorf("download fetch must be hidden from MCP")
	}
	if c := cobratree.CommandAtPath(root, []string{"download", "link"}); c == nil || c.Annotations[cobratree.ReadOnlyAnnotation] != "" {
		t.Errorf("download link mints a live archive URL and must not be on the read-only MCP surface")
	}
}
