// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-built novel feature: stale-device triage.
// pp:data-source live
package cli

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"cove-pp-cli/internal/coverpc"

	"github.com/spf13/cobra"
)

type staleItem struct {
	AccountID      int64   `json:"account_id"`
	DeviceName     string  `json:"device_name"`
	Customer       string  `json:"customer"`
	DaysStale      float64 `json:"days_stale,omitempty"`
	LastSuccessAt  string  `json:"last_success_at,omitempty"`
	NeverSucceeded bool    `json:"never_succeeded,omitempty"`
	LastStatusName string  `json:"last_status_name,omitempty"`
}

type staleView struct {
	PartnerID     int64       `json:"partner_id"`
	ThresholdDays float64     `json:"threshold_days"`
	Items         []staleItem `json:"items"`
	TotalStale    int         `json:"total_stale"`
	scanCapFields
}

func newNovelDevicesStaleCmd(flags *rootFlags) *cobra.Command {
	var days float64
	var partnerID int64
	var maxScanPages int
	cmd := &cobra.Command{
		Use:   "stale",
		Short: "Devices with no successful backup in N days, ranked worst-first",
		Long: strings.Trim(`
Computes staleness from the last-successful-session timestamp (column F09)
across every customer in the partner tree and ranks the gaps worst-first.
A device can be stale while its last-status looks fine — this command
catches the silent gaps.

Use this command to find devices with no SUCCESSFUL backup in N days. Do NOT
use it to find devices whose last status was an error; use
'devices failures' instead.
`, "\n"),
		Example: strings.Trim(`
  cove-cli devices stale --days 3 --json
  cove-cli devices stale --days 7 --partner-id 1234 --csv`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only":          "true",
			"pp:no-error-path-probe": "true",
			"pp:requires-tier":       "credentials",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would rank devices by days since last successful backup")
				return nil
			}
			if days <= 0 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--days must be positive, e.g. --days 3"))
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
				Columns: []string{
					coverpc.ColDeviceID, coverpc.ColDeviceName, coverpc.ColCustomer,
					coverpc.ColTotalLastSuccessTS, coverpc.ColTotalLastStatus,
				},
				MaxPages: maxScanPages,
			})
			if err != nil {
				return apiErr(err)
			}
			now := time.Now()
			threshold := time.Duration(days * 24 * float64(time.Hour))
			items := make([]staleItem, 0)
			for _, d := range devices {
				s := d.Settings
				item := staleItem{
					AccountID:  d.AccountID,
					DeviceName: s[coverpc.ColDeviceName],
					Customer:   s[coverpc.ColCustomer],
				}
				if status, ok := coverpc.SettingInt(s, coverpc.ColTotalLastStatus); ok {
					item.LastStatusName = coverpc.StatusName(int(status))
				}
				lastSuccess, ok := coverpc.SettingTime(s, coverpc.ColTotalLastSuccessTS)
				if !ok {
					// No successful session on record — stale by definition.
					item.NeverSucceeded = true
					items = append(items, item)
					continue
				}
				gap := now.Sub(lastSuccess)
				if gap < threshold {
					continue
				}
				item.DaysStale = math.Round(gap.Hours()/24*10) / 10
				item.LastSuccessAt = lastSuccess.Format(time.RFC3339)
				items = append(items, item)
			}
			sort.Slice(items, func(i, j int) bool {
				// Never-succeeded first, then largest gap.
				if items[i].NeverSucceeded != items[j].NeverSucceeded {
					return items[i].NeverSucceeded
				}
				return items[i].DaysStale > items[j].DaysStale
			})
			view := staleView{
				PartnerID:     root,
				ThresholdDays: days,
				Items:         items,
				TotalStale:    len(items),
				scanCapFields: scanCapFields{
					ScannedDevices: scanned,
					MaxScanPages:   effectiveMaxPages(maxScanPages),
					Note:           scanCapNote(capHit, scanned, effectiveMaxPages(maxScanPages), "staleness"),
				},
			}
			if len(items) == 0 && view.Note == "" {
				view.Note = fmt.Sprintf("all %d scanned devices have a successful session within %.1f days", scanned, days)
			}
			return emitCoveJSON(cmd, flags, view, view.Items)
		},
	}
	cmd.Flags().Float64Var(&days, "days", 3, "Staleness threshold in days since the last successful session")
	cmd.Flags().Int64Var(&partnerID, "partner-id", 0, "Root partner to sweep (default: the logged-in user's partner)")
	cmd.Flags().IntVar(&maxScanPages, "max-scan-pages", 10, "Maximum statistics pages to scan")
	return cmd
}
