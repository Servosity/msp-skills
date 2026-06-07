// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Transcendence command — hand-built over the local insights store.

package cli

import (
	"strings"

	"github.com/spf13/cobra"

	"knowbe4-pp-cli/internal/insights"
)

// pp:data-source local
func newNovelRiskLeaderboardCmd(flags *rootFlags) *cobra.Command {
	var top int
	var dbPath string

	cmd := &cobra.Command{
		Use:   "risk-leaderboard",
		Short: "Highest-risk users with click/report/training context",
		Long: strings.TrimSpace(`
Rank the highest-risk users, each enriched with their phishing click history,
report count, and number of open (not-passed) trainings. The API exposes the risk
score alone; the why-behind-the-score requires fusing users, phishing recipients,
and training enrollments from the local store.

Run 'knowbe4-cli sync' first.`),
		Example:     "  knowbe4-cli risk-leaderboard --top 25 --agent",
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
			insightsSyncHints(cmd, st, "users", flags)
			rows, err := insights.RiskLeaderboard(cmd.Context(), db, top)
			if err != nil {
				return err
			}
			return flags.printJSON(cmd, rows)
		},
	}
	cmd.Flags().IntVar(&top, "top", 25, "Limit to the top N users (0 = no limit)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: standard location)")
	return cmd
}
