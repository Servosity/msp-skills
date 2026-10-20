// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: an MCP true is only a value for a boolean flag.

package cobratree

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"
)

// boolForNonBoolFlag refuses a JSON true for a flag that is not boolean. The
// shell-out passes true as a bare --flag, which a non-boolean flag reads as
// taking the next token, the first positional: subscriptions_set with
// seats:true ordered accountId seats (adversarial round 31).
func boolForNonBoolFlag(cmd *cobra.Command, args map[string]any) string {
	if cmd == nil {
		return ""
	}
	names := make([]string, 0, len(args))
	for name := range args {
		names = append(names, name)
	}
	sort.Strings(names) // one message for one call, run to run
	for _, name := range names {
		if b, ok := args[name].(bool); !ok || !b {
			continue
		}
		f := cmd.Flags().Lookup(name)
		if f == nil {
			f = cmd.InheritedFlags().Lookup(name)
		}
		if f != nil && f.Value.Type() != "bool" && f.Value.Type() != "count" {
			return fmt.Sprintf("MCP parameter %q takes a value of type %s, not true; nothing was sent", name, f.Value.Type())
		}
	}
	return ""
}
