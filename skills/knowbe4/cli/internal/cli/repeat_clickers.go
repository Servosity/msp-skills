// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Transcendence command — hand-built over the local insights store.

package cli

import (
	"strings"

	"github.com/spf13/cobra"

	"knowbe4-pp-cli/internal/insights"
)

// pp:data-source local
func newNovelRepeatClickersCmd(flags *rootFlags) *cobra.Command {
	var minClicks int
	var since string
	var top int
	var dbPath string

	cmd := &cobra.Command{
		Use:   "repeat-clickers",
		Short: "Users who clicked the bait in multiple phishing tests",
		Long: strings.TrimSpace(`
Surface users who clicked the bait in two or more distinct phishing security
tests within a window — the hardest-to-train humans. Counts distinct clicked PSTs
per user across every synced test; no single KnowBe4 API call spans PSTs.

Reads the local store. Run 'knowbe4-cli sync' first (it walks per-PST
recipients into the store).`),
		Example:     "  knowbe4-cli repeat-clickers --min-clicks 2 --since 90d --top 25 --agent",
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
			insightsSyncHints(cmd, st, "phishing-tests", flags)
			rows, err := insights.RepeatClickers(cmd.Context(), db, minClicks, top, since)
			if err != nil {
				return err
			}
			return flags.printJSON(cmd, rows)
		},
	}
	cmd.Flags().IntVar(&minClicks, "min-clicks", 2, "Minimum distinct phishing tests a user must have clicked")
	cmd.Flags().StringVar(&since, "since", "", "Only count clicks within this window (e.g. 90d, 6mo, 1y)")
	cmd.Flags().IntVar(&top, "top", 25, "Limit to the top N users (0 = no limit)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: standard location)")
	return cmd
}
