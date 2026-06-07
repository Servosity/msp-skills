// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Transcendence command — hand-built over the local insights store.

package cli

import (
	"strings"

	"github.com/spf13/cobra"

	"knowbe4-pp-cli/internal/insights"
)

// pp:data-source local
func newNovelGroupRiskContributionCmd(flags *rootFlags) *cobra.Command {
	var window string
	var top int
	var dbPath string

	cmd := &cobra.Command{
		Use:   "group-risk-contribution",
		Short: "Attribute risk movement to the groups driving it",
		Long: strings.TrimSpace(`
Attribute account-level risk movement to the specific groups (departments) driving
it up or down. Diffs each group's risk snapshots over the window and weights by
member count, so a small move in a large department outranks a big move in a tiny
one. Answers "why did our risk go up" by group.

Risk snapshots accumulate on every 'knowbe4-cli sync'.`),
		Example:     "  knowbe4-cli group-risk-contribution --window 90d --top 10 --agent",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			db, st, closeFn, err := openInsightsDB(cmd, dbPath)
			if err != nil {
				return err
			}
			defer closeFn()
			insightsSyncHints(cmd, st, "groups", flags)
			rows, err := insights.GroupRiskContribution(cmd.Context(), db, window, top)
			if err != nil {
				return err
			}
			return flags.printJSON(cmd, rows)
		},
	}
	cmd.Flags().StringVar(&window, "window", "90d", "Lookback window for the baseline snapshot (e.g. 90d, 6mo)")
	cmd.Flags().IntVar(&top, "top", 10, "Limit to the top N groups by weighted contribution (0 = no limit)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: standard location)")
	return cmd
}
