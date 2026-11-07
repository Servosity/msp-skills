// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// datto-rmm handfix dry-run-changes-nothing (press-traps s6; tests-helper
// finding TestDryRunChangesNoLocalFile): a --dry-run preview of a command
// that writes local files writes nothing. 4.34.2's `auth set-token
// --dry-run` saved the token, `auth logout --dry-run` cleared the credential
// store, and `export --output f --dry-run` replaced f with the preview
// (export: datto_export.go). The commands are wrapped through the
// novel-command hook (root.go registerNovelCommand), so no generated file
// changes and every root the CLI or an MCP child builds carries the guard.

import (
	"fmt"

	"github.com/spf13/cobra"
)

func init() {
	registerNovelCommand(guardDryRunLocalWrites)
}

// dryRunPreviewOnly are the commands whose whole effect is a local
// credential write: under --dry-run they name what they would do and stop.
var dryRunPreviewOnly = []struct {
	path    []string
	preview string
}{
	{[]string{"auth", "set-token"}, "would read a token from stdin and save it to the credential store"},
	{[]string{"auth", "logout"}, "would clear the stored credentials"},
}

func guardDryRunLocalWrites(root *cobra.Command, flags *rootFlags) {
	for _, c := range dryRunPreviewOnly {
		preview := c.preview
		wrapRunUnderDryRun(findExact(root, c.path), flags, func(cmd *cobra.Command) bool {
			fmt.Fprintf(cmd.ErrOrStderr(), "%s\n(dry run - nothing changed)\n", preview)
			return true
		})
	}
}

// findExact is the command at path under root, nil when any part is missing.
func findExact(root *cobra.Command, path []string) *cobra.Command {
	cmd := root
	for _, name := range path {
		var next *cobra.Command
		for _, child := range cmd.Commands() {
			if child.Name() == name {
				next = child
				break
			}
		}
		if next == nil {
			return nil
		}
		cmd = next
	}
	return cmd
}

// wrapRunUnderDryRun runs before(cmd) first under --dry-run; when it reports
// the preview done, the command itself does not run.
func wrapRunUnderDryRun(cmd *cobra.Command, flags *rootFlags, before func(*cobra.Command) bool) {
	if cmd == nil || flags == nil {
		return
	}
	runE, run := cmd.RunE, cmd.Run
	if runE == nil && run == nil {
		return
	}
	cmd.Run = nil
	cmd.RunE = func(c *cobra.Command, args []string) error {
		if flags.dryRun && before(c) {
			return nil
		}
		if runE != nil {
			return runE(c, args)
		}
		run(c, args)
		return nil
	}
}
