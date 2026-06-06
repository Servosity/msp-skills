// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"

	"sherweb-pp-cli/internal/insights"

	"github.com/spf13/cobra"
)

// pp:data-source local
func newNovelDriftCmd(flags *rootFlags) *cobra.Command {
	var flagSince string

	cmd := &cobra.Command{
		Use:   "drift",
		Short: "Diff the latest payable-charges snapshot against the prior one to flag charges that appeared, vanished, or changed.",
		Long: `Diff the two most recent payable-charges snapshots in the local store. Each
'sherweb-cli deep-sync' run appends a snapshot, so drift needs at least two
syncs to compare. Charges are keyed by customer + SKU + charge name; entries
sort by absolute dollar delta descending.`,
		Example:     "  sherweb-cli drift --since 2026-03-01 --agent",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			s, err := openInsightStore(cmd.Context())
			if err != nil {
				return err
			}
			defer s.Close()

			if !hintIfUnsynced(cmd, s, "") {
				hintIfStale(cmd, s, "", flags.maxAge)
			}

			ids, byID, err := loadPayableSnapshots(s)
			if err != nil {
				return fmt.Errorf("loading payable snapshots: %w", err)
			}
			// Optional --since lower bound on snapshot ids (timestamps).
			if flagSince != "" {
				kept := ids[:0]
				for _, id := range ids {
					if id >= flagSince {
						kept = append(kept, id)
					}
				}
				ids = kept
			}
			if len(ids) < 2 {
				fmt.Fprintln(cmd.ErrOrStderr(), "need at least 2 payable-charge snapshots to compute drift — run 'sherweb-cli deep-sync' across two or more billing periods")
				return flags.printJSON(cmd, []insights.DriftEntry{})
			}
			prior := byID[ids[len(ids)-2]]
			current := byID[ids[len(ids)-1]]
			result := insights.ComputeDrift(current, prior)
			return flags.printJSON(cmd, result)
		},
	}
	cmd.Flags().StringVar(&flagSince, "since", "", "Only compare snapshots taken on or after this timestamp (YYYY-MM-DD)")
	return cmd
}
