// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel-feature command (Printing Press transcendence).

package cli

import (
	"time"

	"github.com/spf13/cobra"
)

// pp:data-source local
func newNovelFleetStaleCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	var days int
	cmd := &cobra.Command{
		Use:   "stale",
		Short: "Find hosts across every tenant whose sensor has not checked in within N days",
		Long: "Surface coverage gaps: hosts whose last_seen is older than --days across all " +
			"synced CIDs. Run 'fleet sync' first. Reads no live API.",
		Example:     "  crowdstrike-cli fleet stale --days 14 --json",
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
			ents, err := loadFleetEntitiesFrom(cmd.Context(), st, kindHost, false)
			if err != nil {
				return configErr(err)
			}
			return flags.printJSON(cmd, staleHosts(ents, days, time.Now()))
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Local SQLite store path (default: standard data dir)")
	cmd.Flags().IntVar(&days, "days", 14, "Sensor-silence threshold in days")
	return cmd
}
