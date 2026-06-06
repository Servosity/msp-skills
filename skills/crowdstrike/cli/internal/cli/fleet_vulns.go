// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel-feature command (Printing Press transcendence).

package cli

import "github.com/spf13/cobra"

// pp:data-source local
func newNovelFleetVulnsCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	var severity string
	cmd := &cobra.Command{
		Use:   "vulns",
		Short: "Rank Spotlight vulnerabilities across every tenant, optionally filtered by severity",
		Long: "Filter and rank synced Spotlight vulnerabilities across all CIDs from the " +
			"local store. Run 'fleet sync' first. Reads no live API.",
		Example:     "  crowdstrike-cli fleet vulns --severity critical --json",
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
			ents, err := loadFleetEntitiesFrom(cmd.Context(), st, kindVuln, false)
			if err != nil {
				return configErr(err)
			}
			return flags.printJSON(cmd, rankVulns(ents, severity))
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Local SQLite store path (default: standard data dir)")
	cmd.Flags().StringVar(&severity, "severity", "", "Filter to one severity: critical|high|medium|low")
	return cmd
}
