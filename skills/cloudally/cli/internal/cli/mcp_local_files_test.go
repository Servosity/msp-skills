// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"regexp"
	"testing"

	"cloudally-pp-cli/internal/mcp/cobratree"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// pathFlagName matches flags whose value names a local file or directory.
var pathFlagName = regexp.MustCompile(`(^|-)(file|path|dir|input|output|out|dest|db)$`)

// A flag that names a local file must never reach an MCP caller: a read
// path echoes file lines back (restore plan --entities-file returned
// credentials.toml tokens), a write path truncates any file the server can
// write. Walks every MCP-visible command so a new flag cannot slip in.
// Root persistent flags (--home, --config, ...) are refused separately by
// cobratree's blockedRootFlags.
func TestMCPLocalFileFlagsBlocked(t *testing.T) {
	root := RootCmd()
	var walk func(c *cobra.Command, hidden bool)
	walk = func(c *cobra.Command, hidden bool) {
		hidden = hidden || c.Annotations[cobratree.HiddenAnnotation] == "true"
		if !hidden && c != root {
			c.LocalNonPersistentFlags().VisitAll(func(f *pflag.Flag) {
				if f.Value.Type() == "bool" || !pathFlagName.MatchString(f.Name) {
					return
				}
				if !cobratree.DestinationFlagBlocked(c, f.Name) {
					t.Errorf("%s --%s names a local file and reaches MCP callers", c.CommandPath(), f.Name)
				}
			})
		}
		for _, s := range c.Commands() {
			walk(s, hidden)
		}
	}
	walk(root, false)

	for _, path := range [][]string{{"download", "fetch"}, {"import"}} {
		if c := cobratree.CommandAtPath(root, path); c == nil || c.Annotations[cobratree.HiddenAnnotation] != "true" {
			t.Errorf("%v must be hidden from MCP", path)
		}
	}
	if c := cobratree.CommandAtPath(root, []string{"download", "link"}); c == nil || c.Annotations[cobratree.ReadOnlyAnnotation] != "" {
		t.Errorf("download link mints a live archive URL and must not be on the read-only MCP surface")
	}
}
