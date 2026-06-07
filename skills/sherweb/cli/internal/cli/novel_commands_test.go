// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"testing"

	"github.com/spf13/cobra"
)

// Each novel command must honor the verify-friendly dry-run guard: with
// --dry-run set, RunE returns nil without opening the store or hitting the
// network. The substantive join logic is unit-tested in internal/insights.
func TestNovelCommandsDryRun(t *testing.T) {
	cases := []struct {
		use   string
		ctor  func(*rootFlags) *cobra.Command
		flags []string
	}{
		{"margin", newNovelMarginCmd, []string{"month", "customer"}},
		{"drift", newNovelDriftCmd, []string{"since"}},
		{"orphans", newNovelOrphansCmd, nil},
		{"fleet-subs", newNovelFleetSubsCmd, []string{"product", "sku"}},
		{"right-size", newNovelRightSizeCmd, []string{"customer"}},
		{"amend-preview", newNovelAmendPreviewCmd, []string{"customer", "sub", "qty"}},
		{"margin-trend", newNovelMarginTrendCmd, []string{"customer", "last"}},
		{"sub-changes", newNovelSubChangesCmd, []string{"since"}},
		{"usage-leak", newNovelUsageLeakCmd, []string{"customer"}},
	}
	for _, c := range cases {
		t.Run(c.use, func(t *testing.T) {
			flags := &rootFlags{dryRun: true}
			cmd := c.ctor(flags)
			if cmd.Use != c.use {
				t.Fatalf("Use: want %q, got %q", c.use, cmd.Use)
			}
			for _, f := range c.flags {
				if cmd.Flags().Lookup(f) == nil {
					t.Errorf("missing --%s flag", f)
				}
			}
			if cmd.RunE == nil {
				t.Fatalf("%s has no RunE", c.use)
			}
			if err := cmd.RunE(cmd, nil); err != nil {
				t.Errorf("dry-run RunE returned error: %v", err)
			}
		})
	}
}
