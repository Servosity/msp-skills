// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Class test (local-flag-shadows-global-switch): no command-local flag that
// takes a value may carry the name of a persistent switch an ancestor
// declares. Cobra keeps the local flag, so on that one command the documented
// bare --json (or --agent, --quiet, ...) swallows the next argument or stops
// with "flag needs an argument". Failed first on `firebase create --json`.

package cli

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func TestClassNoLocalValueFlagShadowsAPersistentSwitch(t *testing.T) {
	var bad []string
	var walk func(cmd *cobra.Command)
	walk = func(cmd *cobra.Command) {
		cmd.LocalNonPersistentFlags().VisitAll(func(f *pflag.Flag) {
			if f.NoOptDefVal != "" { // a switch itself: a bare use still parses
				return
			}
			for p := cmd.Parent(); p != nil; p = p.Parent() {
				if pf := p.PersistentFlags().Lookup(f.Name); pf != nil && pf.NoOptDefVal != "" {
					bad = append(bad, cmd.CommandPath()+" --"+f.Name+" ("+f.Value.Type()+" over "+pf.Value.Type()+")")
					return
				}
			}
		})
		for _, c := range cmd.Commands() {
			walk(c)
		}
	}
	walk(RootCmd())
	if len(bad) > 0 {
		t.Fatalf("local value flags shadow a persistent switch; rename each (keep its wire name):\n  %s", strings.Join(bad, "\n  "))
	}
}
