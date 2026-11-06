// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored (knowbe4 handfix sync-populates-insights-tables): `sync` also
// walks every synced phishing test's recipients into pst_recipients and takes a
// risk snapshot. Without it repeat-clickers, untrained-clickers, report-rate,
// risk-leaderboard, qbr, risk-drift and group-risk-contribution read empty
// tables after a real sync (the 4.24 reprint lost the inline sync.go edit).
// Wired from a novel-command hook, so no generated file carries it.

package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"knowbe4-pp-cli/internal/insights"
	"knowbe4-pp-cli/internal/store"
)

func init() {
	registerNovelCommand(func(root *cobra.Command, flags *rootFlags) {
		for _, c := range root.Commands() {
			if c.Name() == "sync" && c.RunE != nil {
				wrapSyncWithInsights(c, flags)
			}
		}
	})
}

// insightsSyncInputs are the resources the insights walk reads: recipients come
// from synced phishing tests, snapshots from users and groups.
var insightsSyncInputs = map[string]bool{"phishing-tests": true, "users": true, "groups": true}

func wrapSyncWithInsights(sync *cobra.Command, flags *rootFlags) {
	primary := sync.RunE
	sync.RunE = func(cmd *cobra.Command, args []string) error {
		if err := primary(cmd, args); err != nil {
			return err
		}
		if flags.dryRun {
			return nil
		}
		if only, _ := cmd.Flags().GetStringSlice("resources"); len(only) > 0 {
			wanted := false
			for _, r := range only {
				wanted = wanted || insightsSyncInputs[r]
			}
			if !wanted {
				return nil
			}
		}
		return runInsightsSync(cmd, flags)
	}
}

func runInsightsSync(cmd *cobra.Command, flags *rootFlags) error {
	c, err := flags.newClient()
	if err != nil {
		return err
	}
	c.NoCache = true
	dbPath, _ := cmd.Flags().GetString("db")
	if dbPath == "" {
		dbPath = defaultDBPath("knowbe4-cli")
	}
	st, err := store.OpenWithContext(cmd.Context(), dbPath)
	if err != nil {
		return fmt.Errorf("opening local database: %w", err)
	}
	defer st.Close()
	walk, snaps, err := insights.SyncExtras(cmd.Context(), c, st.DB(), 100)
	if err != nil {
		return fmt.Errorf("insights sync (phishing-test recipients, risk snapshots): %w", err)
	}
	msg := fmt.Sprintf("insights: %d recipient rows from %d phishing tests, %d risk snapshots", walk.Rows, walk.Tests, snaps)
	if walk.Failed > 0 {
		msg += fmt.Sprintf("; %d tests failed (first error: %v), so recipient-based answers stay unavailable until a sync walks every test", walk.Failed, walk.FirstErr)
	} else if !walk.Complete {
		msg += "; the phishing tests are not completely synced, so recipient-based answers stay unavailable until they are"
	}
	if walk.AccountErr != nil {
		msg += fmt.Sprintf("; the account risk score (GET /v1/account) was not taken: %v", walk.AccountErr)
	}
	fmt.Fprintln(cmd.ErrOrStderr(), msg)
	return nil
}
