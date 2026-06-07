// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Transcendence command — hand-built over the local insights store.

package cli

import (
	"strings"

	"github.com/spf13/cobra"

	"knowbe4-pp-cli/internal/insights"
)

// pp:data-source local
func newNovelUntrainedClickersCmd(flags *rootFlags) *cobra.Command {
	var since string
	var top int
	var dbPath string

	cmd := &cobra.Command{
		Use:   "untrained-clickers",
		Short: "Users who clicked a phish but have no passed training",
		Long: strings.TrimSpace(`
Anti-join: users who clicked a phishing test but have no Passed/Completed training
enrollment. Phishing results and training records are separate KnowBe4 endpoints
the console never correlates — this is the most actionable remediation list a
vCISO owns.

Reads the local store. Run 'knowbe4-cli sync' first.`),
		Example:     "  knowbe4-cli untrained-clickers --since 180d --agent",
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
			insightsSyncHints(cmd, st, "training-enrollments", flags)
			rows, err := insights.UntrainedClickers(cmd.Context(), db, top, since)
			if err != nil {
				return err
			}
			return flags.printJSON(cmd, rows)
		},
	}
	cmd.Flags().StringVar(&since, "since", "", "Only count clicks within this window (e.g. 90d, 6mo, 1y)")
	cmd.Flags().IntVar(&top, "top", 0, "Limit to the top N users by risk (0 = no limit)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: standard location)")
	return cmd
}
