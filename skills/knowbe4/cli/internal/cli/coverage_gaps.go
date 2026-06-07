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
func newNovelCoverageGapsCmd(flags *rootFlags) *cobra.Command {
	var gapType string
	var joinedWithin string
	var top int
	var dbPath string

	cmd := &cobra.Command{
		Use:   "coverage-gaps",
		Short: "Active users covered by no training or no phishing",
		Long: strings.TrimSpace(`
Find active users enrolled in zero training and/or zero phishing campaigns — the
people your program silently misses. Anti-join of users against campaign coverage.
Use --joined-within to focus on unenrolled new hires.

Reads the local store. Run 'knowbe4-cli sync' first.`),
		Example:     "  knowbe4-cli coverage-gaps --type training --joined-within 30d --agent",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			switch strings.ToLower(strings.TrimSpace(gapType)) {
			case "", "training", "phishing":
			default:
				return fmt.Errorf("--type must be one of: training, phishing (or omit for both); got %q", gapType)
			}
			db, st, closeFn, err := openInsightsDB(cmd, dbPath)
			if err != nil {
				return err
			}
			defer closeFn()
			insightsSyncHints(cmd, st, "users", flags)
			rows, err := insights.CoverageGaps(cmd.Context(), db, gapType, joinedWithin, top)
			if err != nil {
				return err
			}
			return flags.printJSON(cmd, rows)
		},
	}
	cmd.Flags().StringVar(&gapType, "type", "", "Gap kind: training, phishing, or omit for both")
	cmd.Flags().StringVar(&joinedWithin, "joined-within", "", "Only users who joined within this window (e.g. 30d, 6mo)")
	cmd.Flags().IntVar(&top, "top", 0, "Limit to the top N users by risk (0 = no limit)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: standard location)")
	return cmd
}
