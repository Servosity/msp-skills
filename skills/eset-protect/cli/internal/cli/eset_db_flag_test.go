// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// TestDBFlagHelpNamesTheScopedMirror (handfixes.json:
// db-flag-help-names-scoped-mirror): every --db flag in the command tree
// describes the credential-scoped mirror file (data-<hash>.db) and where to
// see it, never the generated "resolved data directory data.db" or
// "standard cache location".
func TestDBFlagHelpNamesTheScopedMirror(t *testing.T) {
	seen := 0
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		check := func(f *pflag.Flag) {
			if f.Name != "db" {
				return
			}
			seen++
			u := f.Usage
			if !strings.Contains(u, "data-<hash>.db") || !strings.Contains(u, "'doctor' prints the path") {
				t.Errorf("%s --db usage does not name the scoped mirror: %q", c.CommandPath(), u)
			}
			for _, bad := range []string{"resolved data directory data.db", "standard cache location", "the CLI's data directory)", "the CLI data directory)", "data store)"} {
				if strings.Contains(u, bad) {
					t.Errorf("%s --db usage still says %q: %q", c.CommandPath(), bad, u)
				}
			}
		}
		c.LocalFlags().VisitAll(check)
		for _, sub := range c.Commands() {
			walk(sub)
		}
	}
	walk(RootCmd())
	if seen < 30 {
		t.Fatalf("only %d --db flags found; the walk did not reach the command tree", seen)
	}
}
