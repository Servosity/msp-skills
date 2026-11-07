// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored (not generator output); pinned by skills/eset-protect/handfixes.json
// under "mcp-mirror-write-confirm".

package cli

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// The parse-only MCP preview. When the MCP server previews a mirrored write
// that the caller has not confirmed, it starts this CLI with cliutil.MCPChildEnvVar
// set to cliutil.MCPChildPreview (a normal child gets "1"). The CLI then
// parses its command line as usual (unknown flags, bad values, the argument
// count, required flags and flag groups all fail the way a real call would),
// prints one line naming what it parsed, and runs nothing: the root
// PersistentPreRunE returns before it touches the home override, a profile, a
// --deliver sink, the platform session or the learn store; the command's own
// hooks and RunE are replaced; and Execute writes no learn journal. A
// command's --dry-run is not used for this: it is not proven read-only (sync
// opens and migrates its store before it checks --dry-run).

// mcpPreviewOnlyRootPreRun keeps the root PersistentPreRunE reachable for a
// preview. Cobra runs only the nearest PersistentPreRun(E) above the executed
// command, so one on a subcommand would bypass the root's preview return and
// run the command; a preview runs none of them.
func mcpPreviewOnlyRootPreRun(root *cobra.Command) {
	var walk func(*cobra.Command)
	walk = func(c *cobra.Command) {
		if c != root {
			c.PersistentPreRun, c.PersistentPreRunE = nil, nil
		}
		for _, child := range c.Commands() {
			walk(child)
		}
	}
	walk(root)
}

// mcpParseOnlyPreview runs from the root PersistentPreRunE, after flag parse
// and argument validation: every later hook of cmd and its parents becomes a
// no-op, and RunE only reports what was parsed. Cobra still checks required
// flags and flag groups before RunE, so those failures reach the caller too.
func mcpParseOnlyPreview(cmd *cobra.Command) error {
	cmd.PreRun, cmd.PreRunE = nil, nil
	cmd.PostRun, cmd.PostRunE = nil, nil
	for c := cmd; c != nil; c = c.Parent() {
		c.PersistentPostRun, c.PersistentPostRunE = nil, nil
	}
	cmd.Run = nil
	cmd.RunE = func(c *cobra.Command, args []string) error {
		_, err := fmt.Fprintln(c.OutOrStdout(), mcpPreviewLine(c, args))
		return err
	}
	return nil
}

// mcpPreviewLine names the command, every flag the command line set and the
// positional arguments, then says nothing ran.
func mcpPreviewLine(cmd *cobra.Command, args []string) string {
	var b strings.Builder
	b.WriteString("preview: ")
	b.WriteString(cmd.CommandPath())
	cmd.Flags().Visit(func(f *pflag.Flag) {
		b.WriteString(" --" + f.Name + "=" + mcpPreviewQuote(f.Value.String()))
	})
	for _, a := range args {
		b.WriteString(" " + mcpPreviewQuote(a))
	}
	b.WriteString(" parsed; nothing was run or written")
	return b.String()
}

func mcpPreviewQuote(v string) string {
	if v == "" || strings.ContainsAny(v, " \t\r\n\"'\\") || !strconv.CanBackquote(v) {
		return strconv.Quote(v)
	}
	return v
}
