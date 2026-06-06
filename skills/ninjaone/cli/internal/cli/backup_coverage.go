// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// pp:data-source local
func newNovelBackupCoverageCmd(flags *rootFlags) *cobra.Command {
	var orgFilter string
	var dbPath string

	cmd := &cobra.Command{
		Use:   "backup-coverage",
		Short: "List devices with no/zero backup usage, grouped by organization",
		Long: `Anti-joins the full device inventory against the backup-usage report to
surface devices that have no backup usage record (or zero bytes) — the
unprotected endpoints NinjaOne has no single endpoint to list.

Reads the local store; run 'ninjaone-cli sync' first.`,
		Example: `  # Every unprotected device across the fleet
  ninjaone-cli backup-coverage

  # Just one organization, agent JSON, narrowed fields
  ninjaone-cli backup-coverage --org 42 --agent --select deviceName,org,reason`,
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			db, err := openNovelStore(cmd.Context(), dbPath)
			if err != nil {
				return err
			}
			defer db.Close()

			if !hintIfUnsynced(cmd, db, rtBackupUsage) {
				hintIfStale(cmd, db, rtBackupUsage, flags.maxAge)
			}

			devices, err := buildDeviceIndex(db)
			if err != nil {
				return fmt.Errorf("loading devices: %w", err)
			}
			orgNames, err := buildOrgNames(db)
			if err != nil {
				return fmt.Errorf("loading organizations: %w", err)
			}
			gaps, total, err := computeBackupCoverage(db, devices, orgNames)
			if err != nil {
				return fmt.Errorf("computing backup coverage: %w", err)
			}
			if orgFilter != "" {
				gaps = filterBackupByOrg(gaps, orgFilter)
				// Re-scope the denominator to the filtered org so the
				// "N of M devices" line is not fleet-wide.
				total = 0
				for _, d := range devices {
					if d.OrgID == orgFilter {
						total++
					}
				}
			}

			if wantsStructured(flags) {
				return flags.printJSON(cmd, gaps)
			}
			if note := emptyStoreNote(devices); note != "" {
				fmt.Fprintln(cmd.ErrOrStderr(), note)
			}
			if len(gaps) == 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "All %d device(s) have backup coverage.\n", total)
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%d of %d device(s) have no backup coverage:\n\n", len(gaps), total)
			rows := make([][]string, 0, len(gaps))
			for _, g := range gaps {
				rows = append(rows, []string{g.Org, g.DeviceName, g.DeviceID, g.Reason})
			}
			return flags.printTable(cmd, []string{"ORG", "DEVICE", "ID", "REASON"}, rows)
		},
	}
	cmd.Flags().StringVar(&orgFilter, "org", "", "Limit to one organization id")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: local data store)")
	return cmd
}

// filterBackupByOrg keeps only rows for the given org id. Split out for tests.
func filterBackupByOrg(gaps []backupCoverageRow, orgID string) []backupCoverageRow {
	var out []backupCoverageRow
	for _, g := range gaps {
		if g.OrgID == orgID {
			out = append(out, g)
		}
	}
	return out
}
