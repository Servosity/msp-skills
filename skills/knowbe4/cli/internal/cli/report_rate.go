// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Transcendence command — hand-built over the local insights store.

package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"knowbe4-pp-cli/internal/insights"
)

// pp:data-source local
func newNovelReportRateCmd(flags *rootFlags) *cobra.Command {
	var bottom int
	var top int
	var byGroup bool
	var since string
	var minDelivered int
	var dbPath string

	cmd := &cobra.Command{
		Use:   "report-rate",
		Short: "Rank users or groups by report-vs-click phishing behavior",
		Long: strings.TrimSpace(`
Use this command to rank users or groups by their phish-report-vs-click behavior —
how often they report simulated phish (the Phish Alert Button) versus falling for
them. Surfaces the people who never report. Ratios reported vs clicked recipient
rows across every synced PST; no KnowBe4 endpoint returns report behavior across
tests. Do NOT use this command to rank overall human risk with full training
context; use 'risk-leaderboard' instead.

Reads the local store. Run 'knowbe4-cli sync' first.`),
		Example: strings.Trim(`
  # The 25 worst reporters (lowest report rate, most clicks)
  knowbe4-cli report-rate --bottom 25 --agent

  # Best reporters this quarter
  knowbe4-cli report-rate --top 10 --since 90d --agent

  # Aggregate by group
  knowbe4-cli report-rate --by-group --bottom 10 --agent`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			if bottom > 0 && top > 0 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--bottom and --top are mutually exclusive"))
			}
			limit := bottom
			worst := true
			if top > 0 {
				limit = top
				worst = false
			}
			db, st, closeFn, err := openInsightsDB(cmd, dbPath)
			if err != nil {
				return err
			}
			defer closeFn()
			insightsSyncHints(cmd, st, "phishing-tests", flags)
			rows, err := insights.ReportRate(cmd.Context(), db, byGroup, limit, worst, since, minDelivered)
			if err != nil {
				return err
			}
			return flags.printJSON(cmd, rows)
		},
	}
	cmd.Flags().IntVar(&bottom, "bottom", 25, "Rank the N worst reporters first (lowest report rate, most clicks)")
	cmd.Flags().IntVar(&top, "top", 0, "Rank the N best reporters first instead (mutually exclusive with --bottom)")
	cmd.Flags().BoolVar(&byGroup, "by-group", false, "Aggregate report/click behavior by group instead of per user")
	cmd.Flags().StringVar(&since, "since", "", "Only count deliveries/clicks/reports within this window (e.g. 90d, 6mo, 1y)")
	cmd.Flags().IntVar(&minDelivered, "min-delivered", 1, "Drop entities with fewer delivered phish than this")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: standard location)")
	return cmd
}
