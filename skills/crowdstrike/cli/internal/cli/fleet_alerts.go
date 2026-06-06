// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel-feature command (Printing Press transcendence).

package cli

import "github.com/spf13/cobra"

// pp:data-source local
func newNovelFleetAlertsCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	var status string
	cmd := &cobra.Command{
		Use:   "alerts",
		Short: "One severity-sorted alert queue across every tenant, optionally filtered by status",
		Long: "Build a unified, severity-sorted alert queue across all synced CIDs from the " +
			"local store. Default keeps all open alerts; use --status to scope (e.g. new). " +
			"Run 'fleet sync' first. Reads no live API.",
		Example:     "  crowdstrike-cli fleet alerts --status new --json",
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
			ents, err := loadFleetEntitiesFrom(cmd.Context(), st, kindAlert, false)
			if err != nil {
				return configErr(err)
			}
			return flags.printJSON(cmd, alertQueue(ents, status))
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Local SQLite store path (default: standard data dir)")
	cmd.Flags().StringVar(&status, "status", "", "Filter to one Falcon status (e.g. new, in_progress)")
	return cmd
}
