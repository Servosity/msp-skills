// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/synology/handfixes.json under
// the "profile-values-count-as-typed" and "profile-output-mode-groups" entries.

package cli

import (
	"testing"

	"github.com/spf13/cobra"
)

// TestApplyProfileMarksFlagsChanged: commands gate on Changed() (abb backup's
// required-input check and its --task-ids JSON validation,
// hasChangedLocalFlags, --agent's defaults), so a profile value must count as
// typed; before, a profile carrying --task-ids printed help and ran nothing,
// or skipped validation.
func TestApplyProfileMarksFlagsChanged(t *testing.T) {
	var taskIDs string
	var dryRun bool
	cmd := &cobra.Command{Use: "backup"}
	cmd.Flags().StringVar(&taskIDs, "task-ids", "", "")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "")
	if err := ApplyProfileToFlags(cmd, &Profile{Values: map[string]string{"task-ids": "[5]", "dry-run": "true"}}); err != nil {
		t.Fatal(err)
	}
	if taskIDs != "[5]" || !dryRun || !cmd.Flags().Changed("task-ids") || !cmd.Flags().Changed("dry-run") {
		t.Fatalf("task-ids=%q dry-run=%v changed=%v/%v, want both applied and Changed", taskIDs, dryRun, cmd.Flags().Changed("task-ids"), cmd.Flags().Changed("dry-run"))
	}
	if !hasChangedLocalFlags(cmd) {
		t.Fatal("a profile-supplied flag does not count as a changed local flag")
	}
}

// TestApplyProfileTypedOutputModeWins: a typed --csv wins over a profile's
// --json (same group), while a profile's --compact (another group) applies.
func TestApplyProfileTypedOutputModeWins(t *testing.T) {
	var asJSON, asCSV, compact bool
	cmd := &cobra.Command{Use: "list"}
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
