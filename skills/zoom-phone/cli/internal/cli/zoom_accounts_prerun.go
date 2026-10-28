// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source local
// Account isolation at the earliest common point of every command.

package cli

import (
	"github.com/spf13/cobra"

	"zoom-phone-pp-cli/internal/cliutil"
)

// The client hook only covers API use. Local-store commands (e911 audit,
// numbers inventory, footprint --no-live, search, sql, analytics, sync's store
// writes) resolve and open the store in the generated pre-run or in their own
// RunE, before or without any client. So the isolation check also runs as the
// first step of every PersistentPreRunE in the tree: before the generated
// pre-run loads config, scopes or claims the store, auto-refreshes, or seeds
// the learn store. Cobra runs only the nearest PersistentPreRun(E), so every
// command that defines one is wrapped, not just the root (today only the root
// does).
func init() {
	registerNovelCommand(func(root *cobra.Command, flags *rootFlags) {
		acctWrapPreRuns(root, flags)
	})
}

// ZoomCheckAccountIsolation runs the account-isolation check for the current
// process environment (ZOOM_PHONE_CONFIG or the default config, and the data
// directory from ZOOM_PHONE_HOME / ZOOM_PHONE_DATA_DIR). The MCP server calls it
// at startup, before any tool can open the local store. It returns a configErr
// (exit code 10) on refusal.
func ZoomCheckAccountIsolation() error {
	return acctCheckIsolation(acctResolvedConfigPath(nil))
}

// acctPreRunExempt lists commands that never read config or the store.
func acctPreRunExempt(cmd *cobra.Command) bool {
	for c := cmd; c != nil; c = c.Parent() {
		switch c.Name() {
		case "help", "completion", cobra.ShellCompRequestCmd, cobra.ShellCompNoDescRequestCmd:
			if c.Parent() != nil && c.Parent().Parent() == nil {
				return true
			}
		}
	}
	return false
}

func acctWrapPreRuns(root *cobra.Command, flags *rootFlags) {
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		if c == root || c.PersistentPreRunE != nil || c.PersistentPreRun != nil {
			acctWrapPreRun(c, flags)
		}
		for _, sub := range c.Commands() {
			walk(sub)
		}
	}
	walk(root)
}

func acctWrapPreRun(c *cobra.Command, flags *rootFlags) {
	innerE, inner := c.PersistentPreRunE, c.PersistentPreRun
	c.PersistentPreRun = nil
	c.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		if !acctPreRunExempt(cmd) {
			// The generated pre-run applies --home first thing; apply it here
			// (idempotently) so the check sees the same data directory.
			if flags != nil {
				if _, err := cliutil.SetHomeOverride(flags.homePath); err != nil {
					return err
				}
			}
			if err := acctCheckIsolation(acctResolvedConfigPath(flags)); err != nil {
				return err
			}
		}
		if innerE != nil {
			return innerE(cmd, args)
		}
		if inner != nil {
			inner(cmd, args)
		}
		return nil
	}
}
