// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Transcendence command — hand-built over the local insights store.

package cli

import (
	"strings"
	"time"

	"github.com/spf13/cobra"

	"knowbe4-pp-cli/internal/insights"
)

// pp:data-source local
func newNovelFreshnessCmd(flags *rootFlags) *cobra.Command {
	var staleAfter time.Duration
	var dbPath string

	cmd := &cobra.Command{
		Use:   "freshness",
		Short: "Per-table sync freshness and input-readiness for every insight command",
		Long: strings.TrimSpace(`
See per-table sync freshness and whether the cross-entity commands have the data
they need — before trusting a clicker hunt. Reads per-table sync watermarks and
row counts from the local SQLite store, including the insights-owned
pst_recipients fan-out and risk_snapshots tables, and flags each transcendence
command whose inputs are stale or never synced, naming the sync command that
fixes the gap.

Run this first when results look empty or stale.`),
		Example: strings.Trim(`
  # Full freshness and readiness report
  knowbe4-cli freshness --agent

  # Treat anything older than a day as stale
  knowbe4-cli freshness --stale-after 24h --agent`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			// No insightsSyncHints here: this command IS the freshness
			// reporter — its stdout carries the same signal the stderr
			// hints would duplicate.
			db, _, closeFn, err := openInsightsDB(cmd, dbPath)
			if err != nil {
				return err
			}
			defer closeFn()
			rep, err := insights.Freshness(cmd.Context(), db, staleAfter, time.Now())
			if err != nil {
				return err
			}
			return flags.printJSON(cmd, rep)
		},
	}
	cmd.Flags().DurationVar(&staleAfter, "stale-after", 168*time.Hour, "Age beyond which a table is flagged stale (0 disables staleness flagging)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: standard location)")
	return cmd
}
