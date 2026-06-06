// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

// newNovelStaleDevicesCmd implements the device-specific "stale-devices" command.
// It is distinct from the framework-generic "stale" command (which keys off
// updatedAt-style fields); NinjaOne devices expose a lastContact epoch instead,
// so this command keys off last contact and the offline flag.
// pp:data-source local
func newNovelStaleDevicesCmd(flags *rootFlags) *cobra.Command {
	var days int
	var orgFilter string
	var dbPath string

	cmd := &cobra.Command{
		Use:   "stale-devices",
		Short: "List devices whose last contact exceeds a threshold, with days-since and org",
		Long: `Scans synced devices for endpoints whose last contact with NinjaOne is older
than the threshold (or that have never reported in), with the days-since and
owning organization. NinjaOne exposes a last-contact timestamp per device but
no "abandoned devices" list — this computes it across the fleet.

Reads the local store; run 'ninjaone-cli sync' first.`,
		Example: `  # Devices not seen in 14+ days (default)
  ninjaone-cli stale-devices

  # 30-day threshold, one org, as CSV
  ninjaone-cli stale-devices --days 30 --org 42 --csv`,
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

			if !hintIfUnsynced(cmd, db, rtDevices) {
				hintIfStale(cmd, db, rtDevices, flags.maxAge)
			}

			devices, err := buildDeviceIndex(db)
			if err != nil {
				return fmt.Errorf("loading devices: %w", err)
			}
			orgNames, err := buildOrgNames(db)
			if err != nil {
				return fmt.Errorf("loading organizations: %w", err)
			}
			stale := computeStaleDevices(devices, orgNames, days, time.Now().UTC())
			if orgFilter != "" {
				stale = filterStaleByOrg(stale, orgFilter)
			}

			if wantsStructured(flags) {
				return flags.printJSON(cmd, stale)
			}
			if note := emptyStoreNote(devices); note != "" {
				fmt.Fprintln(cmd.ErrOrStderr(), note)
			}
			if len(stale) == 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "No devices stale beyond %d days.\n", days)
				return nil
			}
			rows := make([][]string, 0, len(stale))
			for _, s := range stale {
				since := fmt.Sprintf("%d", s.DaysSince)
				if s.DaysSince < 0 {
					since = "never"
				}
				rows = append(rows, []string{s.Org, s.DeviceName, since, s.LastContact})
			}
			return flags.printTable(cmd, []string{"ORG", "DEVICE", "DAYS SINCE", "LAST CONTACT"}, rows)
		},
	}
	cmd.Flags().IntVar(&days, "days", 14, "Days without contact to consider a device stale")
	cmd.Flags().StringVar(&orgFilter, "org", "", "Limit to one organization id")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: local data store)")
	return cmd
}

// filterStaleByOrg keeps only rows for the given org id. Split out for tests.
func filterStaleByOrg(rows []staleRow, orgID string) []staleRow {
	var out []staleRow
	for _, r := range rows {
		if r.OrgID == orgID {
			out = append(out, r)
		}
	}
	return out
}
