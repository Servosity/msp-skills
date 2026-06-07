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
func newNovelRiskDriftCmd(flags *rootFlags) *cobra.Command {
	var window string
	var entity string
	var worsened bool
	var top int
	var dbPath string

	cmd := &cobra.Command{
		Use:   "risk-drift",
		Short: "Rank users/groups by how much their risk score moved",
		Long: strings.TrimSpace(`
Rank users and groups by how much their risk score moved between the latest local
snapshot and the snapshot nearest (latest - window). The API returns a raw series
per entity; it never returns a ranked delta across all entities.

Risk snapshots accumulate every time you run 'knowbe4-cli sync' — drift needs at
least two snapshots over the window to compute a delta.`),
		Example:     "  knowbe4-cli risk-drift --window 90d --worsened --top 20 --agent",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			switch strings.ToLower(strings.TrimSpace(entity)) {
			case "", "user", "group", "account":
			default:
				return fmt.Errorf("--entity must be one of: user, group, account (or omit for all); got %q", entity)
			}
			db, st, closeFn, err := openInsightsDB(cmd, dbPath)
			if err != nil {
				return err
			}
			defer closeFn()
			insightsSyncHints(cmd, st, "users", flags)
			rows, err := insights.RiskDrift(cmd.Context(), db, window, entity, worsened, top)
			if err != nil {
				return err
			}
			return flags.printJSON(cmd, rows)
		},
	}
	cmd.Flags().StringVar(&window, "window", "90d", "Lookback window for the baseline snapshot (e.g. 90d, 6mo, 1y)")
	cmd.Flags().StringVar(&entity, "entity", "", "Limit to user, group, or account (omit for all)")
	cmd.Flags().BoolVar(&worsened, "worsened", false, "Only entities whose risk went up (positive delta)")
	cmd.Flags().IntVar(&top, "top", 20, "Limit to the top N by absolute movement (0 = no limit)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: standard location)")
	return cmd
}
