// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cobratree

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// mirroredBoolArgRefusal refuses a mirrored tool call that sets a boolean
// flag with anything but a JSON boolean. cliArgsFromMCP joins a string or a
// number onto the flag (--confirm=true), and Cobra parses that as true, so a
// confirm gate opened on the string "true". Hand-fix (handfixes.json
// mcp-bool-args-json-boolean-only).
func mirroredBoolArgRefusal(cmd *cobra.Command, args map[string]any) string {
	if cmd == nil {
		return ""
	}
	names := make([]string, 0, len(args))
	for k := range args {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, k := range names {
		flag := lookupMirroredFlag(cmd, k)
		if flag == nil || flag.Value.Type() != "bool" {
			continue
		}
		if _, ok := args[k].(bool); !ok && args[k] != nil {
			return fmt.Sprintf("argument %q must be a JSON boolean (true or false), got %T", k, args[k])
		}
	}
	return ""
}

func lookupMirroredFlag(cmd *cobra.Command, name string) *pflag.Flag {
	if f := cmd.Flags().Lookup(name); f != nil {
		return f
	}
	return cmd.InheritedFlags().Lookup(name)
}
