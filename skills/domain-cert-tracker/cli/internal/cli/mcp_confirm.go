// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"domain-cert-tracker-pp-cli/internal/cliutil"
)

// Hand-fix (handfixes.json mcp-write-confirm): one gate for every command the
// MCP server mirrors as a write. Each gets a --confirm flag (the MCP
// confirm: true argument); under MCP it previews and exits 2 until confirm
// is true. The command line is unchanged. A command that already declares
// --confirm (clients set, clients remove) keeps its own gate.
func requireMCPConfirmOnWrites(root *cobra.Command) {
	var walk func(*cobra.Command)
	walk = func(c *cobra.Command) {
		for _, sub := range c.Commands() {
			walk(sub)
		}
		if c == root || !c.Runnable() || !mirroredAsWrite(c) || c.Flags().Lookup("confirm") != nil {
			return
		}
		confirm := new(bool)
		c.Flags().BoolVar(confirm, "confirm", false, "Confirm the change (the MCP confirm: true argument)")
		gate := func(cmd *cobra.Command, args []string) error {
			if !cliutil.IsMCPChildEnv() || *confirm || !writesWith(cmd) {
				return nil
			}
			flags := map[string]string{}
			cmd.Flags().Visit(func(f *pflag.Flag) { flags[f.Name] = f.Value.String() })
			preview, _ := json.MarshalIndent(map[string]any{"command": cmd.CommandPath(), "args": args, "flags": flags, "confirm_required": true}, "", "  ")
			cmd.OutOrStdout().Write(append(preview, '\n'))
			return usageErr(errors.New("not run: pass confirm: true to make this change"))
		}
		switch {
		case c.RunE != nil:
			next := c.RunE
			c.RunE = func(cmd *cobra.Command, args []string) error {
				if err := gate(cmd, args); err != nil {
					return err
				}
				return next(cmd, args)
			}
		case c.Run != nil:
			next := c.Run
			c.Run = nil
			c.RunE = func(cmd *cobra.Command, args []string) error {
				if err := gate(cmd, args); err != nil {
					return err
				}
				next(cmd, args)
				return nil
			}
		}
	}
	walk(root)
}

// mcpWriteUnconfirmed: this call is an MCP write without confirm: true, so it
// ends in a preview and must touch no store on the way there (no schema
// migration, no seed rows).
func mcpWriteUnconfirmed(c *cobra.Command) bool {
	if !cliutil.IsMCPChildEnv() || !mirroredAsWrite(c) || !writesWith(c) {
		return false
	}
	confirmed, err := c.Flags().GetBool("confirm")
	return err != nil || !confirmed
}

// mirroredAsWrite matches the MCP walker (internal/mcp/cobratree): a
// mirrored command without mcp:read-only is a write tool. Endpoint commands
// are registered in-process and never run as the companion CLI.
func mirroredAsWrite(c *cobra.Command) bool {
	if c.Hidden || annotationTrue(c, "mcp:hidden") || strings.TrimSpace(c.Annotations["pp:endpoint"]) != "" ||
		annotationTrue(c, "pp:api-resource") || annotationTrue(c, "pp:parent-group") {
		return false
	}
	return !annotationTrue(c, "mcp:read-only")
}

// writesWith: a mcp:read-only-unless-flags command writes only when one of
// those flags is set.
func writesWith(c *cobra.Command) bool {
	raw := c.Annotations["mcp:read-only-unless-flags"]
	if raw == "" {
		return true
	}
	for _, name := range strings.FieldsFunc(raw, func(r rune) bool { return strings.ContainsRune(",; \t\n", r) }) {
		if f := c.Flags().Lookup(name); f != nil && f.Changed {
			return true
		}
	}
	return false
}

func annotationTrue(c *cobra.Command, key string) bool {
	switch strings.ToLower(strings.TrimSpace(c.Annotations[key])) {
	case "true", "1", "yes":
		return true
	}
	return false
}
