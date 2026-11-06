// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// Class local-flag-shadows-root-flag (runzero handfixes local-agent-flags-renamed
// and body-flag-shadows-root-flag): a command-local flag named like a
// persistent root flag replaces it on that command. When exactly one of the two
// is boolean, the token after the flag changes meaning: the generated body flag
// --agent (a string) shadowed the bool --agent, so `org create-scan <site>
// --agent --json` sent "--json" as the explorer ID and turned agent mode off.
// No command may declare such a flag (a same-shape shadow such as scan-watch's
// own --timeout keeps every token's meaning).
func TestNoLocalFlagChangesARootFlagsShape(t *testing.T) {
	root := RootCmd()
	persistent := root.PersistentFlags()
	var walk func(cmd *cobra.Command)
	walk = func(cmd *cobra.Command) {
		cmd.LocalNonPersistentFlags().VisitAll(func(f *pflag.Flag) {
			rf := persistent.Lookup(f.Name)
			if rf == nil {
				return
			}
			if (f.Value.Type() == "bool") != (rf.Value.Type() == "bool") {
				t.Errorf("%s: --%s (%s) shadows the root --%s (%s)", cmd.CommandPath(), f.Name, f.Value.Type(), rf.Name, rf.Value.Type())
			}
		})
		for _, c := range cmd.Commands() {
			walk(c)
		}
	}
	for _, c := range root.Commands() {
		walk(c)
	}
}
