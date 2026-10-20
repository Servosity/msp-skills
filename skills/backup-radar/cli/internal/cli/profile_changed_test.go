// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"testing"

	"github.com/spf13/cobra"
)

// HAND-FIX(profile-values-count-as-typed): a profile value marks its flag
// Changed, so `backups retire --profile preview` (saved with --dry-run)
// sends dry_run=true instead of omitting it and retiring live.
func TestApplyProfileMarksFlagsChanged(t *testing.T) {
	var dryRun bool
	cmd := &cobra.Command{Use: "retire"}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "")
	if err := ApplyProfileToFlags(cmd, &Profile{Values: map[string]string{"dry-run": "true"}}); err != nil {
		t.Fatal(err)
	}
	if !dryRun || !cmd.Flags().Changed("dry-run") {
		t.Fatalf("dry-run=%v changed=%v, want both true", dryRun, cmd.Flags().Changed("dry-run"))
	}
}

// HAND-FIX(profile-output-mode-groups): a typed --csv wins over a profile's
// --json (same group), while a profile's --compact (another group) applies.
func TestApplyProfileTypedOutputModeWins(t *testing.T) {
	var asJSON, asCSV, compact bool
	cmd := &cobra.Command{Use: "devices"}
	cmd.Flags().BoolVar(&asJSON, "json", false, "")
	cmd.Flags().BoolVar(&asCSV, "csv", false, "")
	cmd.Flags().BoolVar(&compact, "compact", false, "")
	if err := cmd.Flags().Set("csv", "true"); err != nil {
		t.Fatal(err)
	}
	if err := ApplyProfileToFlags(cmd, &Profile{Values: map[string]string{"json": "true", "compact": "true"}}); err != nil {
		t.Fatal(err)
	}
	if asJSON || !asCSV || !compact {
		t.Fatalf("json=%v csv=%v compact=%v, want false true true", asJSON, asCSV, compact)
	}
}
