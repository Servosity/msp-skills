// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live
package cli

import (
	"fmt"
	"strings"
	"time"

	"cove-pp-cli/internal/coverpc"
	"cove-pp-cli/internal/store"

	"github.com/spf13/cobra"
)

// newCoveSnapshotCmd captures one timestamped fleet snapshot into local
// SQLite — the history layer behind `storage growth` and `devices changes`.
func newCoveSnapshotCmd(flags *rootFlags) *cobra.Command {
	var partnerID int64
	var pageSize int
	var maxScanPages int
	var dbPath string
	cmd := &cobra.Command{
		Use:   "snapshot",
		Short: "Capture a timestamped fleet statistics snapshot into local SQLite",
		Long: strings.Trim(`
Walks EnumerateAccountStatistics under your root partner (or --partner-id)
and stores one timestamped row per device: name, customer, used storage,
last session status, last success time, SKU, previous-month SKU, M365 seat
counts, plus the full column map. The Cove console keeps no history — two or
more snapshots are what make 'storage growth' and 'devices changes' work.
`, "\n"),
		Example: strings.Trim(`
  cove-cli snapshot
  cove-cli snapshot --partner-id 1234 --json
  cove-cli snapshot --max-scan-pages 20   # fleets beyond ~3000 devices`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only":          "false",
			"pp:no-error-path-probe": "true",
			"pp:requires-tier":       "credentials",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would capture a fleet statistics snapshot into local SQLite")
				return nil
			}
			c, err := newCoveRPC(flags)
			if err != nil {
				return err
			}
			root, err := resolvePartnerID(cmd.Context(), c, partnerID)
			if err != nil {
				return authErr(err)
			}
			devices, scanned, capHit, err := fetchFleetStats(cmd.Context(), c, fleetStatsQuery{
				PartnerID: root,
				Columns:   coverpc.SnapshotColumns,
				PageSize:  pageSize,
				MaxPages:  maxScanPages,
			})
			if err != nil {
				return apiErr(err)
			}
			rows := make([]store.CoveDeviceStat, 0, len(devices))
			for _, d := range devices {
				s := d.Settings
				row := store.CoveDeviceStat{
					AccountID:    d.AccountID,
					PartnerID:    d.PartnerID,
					DeviceName:   s[coverpc.ColDeviceName],
					Customer:     s[coverpc.ColCustomer],
					SKU:          s[coverpc.ColSKU],
					PrevSKU:      s[coverpc.ColPrevSKU],
					SettingsJSON: store.MarshalSettings(s),
				}
				if v, ok := coverpc.SettingInt(s, coverpc.ColUsedStorage); ok {
					row.UsedStorage = v
				}
				if v, ok := coverpc.SettingInt(s, coverpc.ColTotalLastStatus); ok {
					row.LastStatus = v
				}
				if v, ok := coverpc.SettingInt(s, coverpc.ColTotalLastSuccessTS); ok {
					row.LastSuccessAt = v
				}
				rows = append(rows, row)
			}
			if dbPath == "" {
				dbPath = defaultDBPath("cove-cli")
			}
			db, err := store.Open(dbPath)
			if err != nil {
				return fmt.Errorf("opening local store: %w", err)
			}
			defer db.Close()
			takenAt := time.Now().UTC()
			snapID, err := db.InsertCoveSnapshot(cmd.Context(), takenAt, root, rows)
			if err != nil {
				return fmt.Errorf("writing snapshot: %w", err)
			}
			view := struct {
				SnapshotID    int64  `json:"snapshot_id"`
				TakenAt       string `json:"taken_at"`
				RootPartnerID int64  `json:"root_partner_id"`
				Devices       int    `json:"devices"`
				scanCapFields
			}{
				SnapshotID:    snapID,
				TakenAt:       takenAt.Format(time.RFC3339),
				RootPartnerID: root,
				Devices:       len(rows),
				scanCapFields: scanCapFields{
					ScannedDevices: scanned,
					MaxScanPages:   effectiveMaxPages(maxScanPages),
					Note:           scanCapNote(capHit, scanned, effectiveMaxPages(maxScanPages), "snapshot"),
				},
			}
			return flags.printJSON(cmd, view)
		},
	}
	cmd.Flags().Int64Var(&partnerID, "partner-id", 0, "Root partner to snapshot (default: the logged-in user's partner)")
	cmd.Flags().IntVar(&pageSize, "page-size", 300, "Devices per EnumerateAccountStatistics page")
	cmd.Flags().IntVar(&maxScanPages, "max-scan-pages", 10, "Maximum statistics pages to scan before returning a partial snapshot")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: the CLI's local store)")
	return cmd
}

func effectiveMaxPages(requested int) int {
	if requested <= 0 {
		return 10
	}
	return requested
}
