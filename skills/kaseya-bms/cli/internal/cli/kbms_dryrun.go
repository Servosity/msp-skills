// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// kaseya-bms handfix dry-run-changes-nothing (press-traps s6; tests-helper
// finding TestDryRunChangesNoLocalFile): a --dry-run preview of a command
// that writes local files writes nothing. 4.34.2's `auth set-token
// --dry-run` saved the token, `auth logout --dry-run` cleared the credential
// store, and `export --output f --dry-run` replaced f with the preview. The
// commands are wrapped through the novel-command hook (root.go
// registerNovelCommand), so no generated file changes and every root the CLI
// or an MCP child builds carries the guard.
//
// Output files (kaseya-bms tests-helper finding, fixed for the class): under
// --dry-run every command that takes an --output file path (export today,
// any a later print adds) previews to stdout and leaves the file alone; the
// datto export fix this layer was ported from was not carried over, so
// export --output f --dry-run replaced f with the preview again.

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
	forEachCommand(root, func(cmd *cobra.Command) {
		if f := cmd.LocalFlags().Lookup("output"); f != nil && f.Value.Type() == "string" {
			previewOutputUnderDryRun(cmd, flags)
		}
	})
}

// forEachCommand calls fn on root and every command under it.
func forEachCommand(root *cobra.Command, fn func(*cobra.Command)) {
	if root == nil {
		return
	}
	fn(root)
	for _, child := range root.Commands() {
		forEachCommand(child, fn)
	}
}

// previewOutputUnderDryRun sends cmd's --dry-run preview to stdout instead of
// its --output file, which it leaves untouched.
func previewOutputUnderDryRun(cmd *cobra.Command, flags *rootFlags) {
	if cmd == nil || flags == nil || (cmd.RunE == nil && cmd.Run == nil) {
		return
	}
	runE, run := cmd.RunE, cmd.Run
	cmd.Run = nil
	cmd.RunE = func(c *cobra.Command, args []string) error {
		if flags.dryRun {
			if f := c.Flags().Lookup("output"); f != nil && f.Value.String() != "" {
				fmt.Fprintf(c.ErrOrStderr(), "--dry-run: the preview goes to stdout; %s is left alone\n", f.Value.String())
				if err := f.Value.Set(""); err != nil {
					return err
				}
			}
		}
		if runE != nil {
			return runE(c, args)
		}
		run(c, args)
		return nil
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
