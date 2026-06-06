// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"time"

	"sherweb-pp-cli/internal/cliutil"
	"sherweb-pp-cli/internal/insights"

	"github.com/spf13/cobra"
)

// pp:data-source local
func newNovelSubChangesCmd(flags *rootFlags) *cobra.Command {
	var flagSince string

	cmd := &cobra.Command{
		Use:   "sub-changes",
		Short: "Flag subscriptions added, cancelled, or quantity-changed between two syncs across the whole book.",
		Long: `Use this command for subscriptions added, cancelled, or quantity-changed between
two syncs across all customers. Do NOT use this command for payable-charge changes;
use 'drift' instead. Do NOT use this command for current total seats by product;
use 'fleet-subs' instead.

Diffs the latest subscription snapshot (written by 'sherweb-cli deep-sync')
against the prior one — or, with --since, against the most recent snapshot at
least that old (e.g. --since 30d). The biggest seat swings sort first.`,
		Example:     "  sherweb-cli sub-changes --agent\n  sherweb-cli sub-changes --since 30d",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			var sinceAge time.Duration
			if flagSince != "" {
				d, err := cliutil.ParseDurationLoose(flagSince)
				if err != nil {
					_ = cmd.Usage()
					return usageErr(fmt.Errorf("invalid --since %q: %w (use durations like 24h, 7d, 4w)", flagSince, err))
				}
				sinceAge = d
			}
			s, err := openInsightStore(cmd.Context())
			if err != nil {
				return err
			}
			defer s.Close()

			if !hintIfUnsynced(cmd, s, "") {
				hintIfStale(cmd, s, "", flags.maxAge)
			}

			ids, byID, err := loadSubscriptionSnapshots(s)
			if err != nil {
				return fmt.Errorf("loading subscription snapshots: %w", err)
			}
			if len(ids) < 2 {
				fmt.Fprintln(cmd.ErrOrStderr(), "need at least two subscription snapshots to diff — each 'sherweb-cli deep-sync' run appends one")
				return flags.printJSON(cmd, []insights.SubChange{})
			}

			currentID := ids[len(ids)-1]
			priorID := ids[len(ids)-2]
			if sinceAge > 0 {
				cutoff := time.Now().UTC().Add(-sinceAge)
				priorID = ""
				for _, id := range ids[:len(ids)-1] { // ascending; keep the newest snapshot at or before the cutoff
					if ts, err := time.Parse(time.RFC3339, id); err == nil && !ts.After(cutoff) {
						priorID = id
					}
				}
				if priorID == "" {
					fmt.Fprintf(cmd.ErrOrStderr(), "no snapshot older than %s — falling back to the previous snapshot\n", flagSince)
					priorID = ids[len(ids)-2]
				}
			}

			changes := insights.DiffSubscriptions(byID[currentID], byID[priorID])
			if len(changes) == 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "no subscription changes between %s and %s\n", priorID, currentID)
			}
			return flags.printJSON(cmd, changes)
		},
	}
	cmd.Flags().StringVar(&flagSince, "since", "", "Diff against the most recent snapshot at least this old (e.g. 24h, 7d, 4w); default: the previous snapshot")
	return cmd
}
