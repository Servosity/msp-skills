// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel-feature command (Printing Press transcendence).

package cli

import (
	"time"

	"github.com/spf13/cobra"
)

// pp:data-source local
func newNovelFleetScorecardCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	cmd := &cobra.Command{
		Use:   "scorecard",
		Short: "Per-tenant posture board: hosts, sensor coverage, open critical alerts, critical vulns, policies",
		Long: "Roll up the CID-keyed local store into one posture row per tenant. Run " +
			"'fleet sync' first to populate the store. Reads no live API.",
		Example:     "  crowdstrike-cli fleet scorecard --json",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			st, err := openFleetStore(cmd.Context(), resolveFleetDB(dbPath))
			if err != nil {
				return configErr(err)
			}
			defer st.Close()
			if !hintIfFleetUnsynced(cmd, st) {
				hintIfFleetStale(cmd, st, flags.maxAge)
			}
			ents, err := loadFleetEntitiesFrom(cmd.Context(), st, "", false)
			if err != nil {
				return configErr(err)
			}
			return flags.printJSON(cmd, scorecardRollup(ents, time.Now()))
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Local SQLite store path (default: standard data dir)")
	return cmd
}

// resolveFleetDB returns the explicit path or the canonical default store path.
func resolveFleetDB(dbPath string) string {
	if dbPath != "" {
		return dbPath
	}
	return defaultDBPath("crowdstrike-cli")
}
