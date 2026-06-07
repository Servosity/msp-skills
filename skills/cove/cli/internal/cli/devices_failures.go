// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-built novel feature: fleet-wide failure sweep.
// pp:data-source live
package cli

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"cove-pp-cli/internal/coverpc"

	"github.com/spf13/cobra"
)

type failureItem struct {
	AccountID     int64  `json:"account_id"`
	DeviceName    string `json:"device_name"`
	Customer      string `json:"customer"`
	Product       string `json:"product"`
	Status        int64  `json:"status"`
	StatusName    string `json:"status_name"`
	Errors        int64  `json:"errors,omitempty"`
	LastSessionAt string `json:"last_session_at,omitempty"`
	LastSuccessAt string `json:"last_success_at,omitempty"`
}

type failuresView struct {
	PartnerID     int64         `json:"partner_id"`
	SinceHours    float64       `json:"since_hours,omitempty"`
	Items         []failureItem `json:"items"`
	TotalFailures int           `json:"total_failures"`
	scanCapFields
}

func newNovelDevicesFailuresCmd(flags *rootFlags) *cobra.Command {
	var since string
	var partnerID int64
	var maxScanPages int
	var includeNeverRun bool
	cmd := &cobra.Command{
		Use:   "failures",
		Short: "Fleet-wide sweep of devices whose last backup session failed",
		Long: strings.Trim(`
Lists every device across the whole partner tree whose LAST session ended in
a bad status — Failed, Aborted, Interrupted, CompletedWithErrors, OverQuota,
or NotStarted — with the F00 status enum decoded to names.

Use this command to find devices whose LAST session ended badly. Do NOT use
it to find devices that simply haven't succeeded recently; use
'devices stale' instead.
`, "\n"),
		Example: strings.Trim(`
  cove-cli devices failures --since 24h --json
  cove-cli devices failures --agent --select items.device_name,items.customer,items.status_name
  cove-cli devices failures --include-never-run=false --csv`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only":          "true",
			"pp:no-error-path-probe": "true",
			"pp:requires-tier":       "credentials",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would sweep the partner tree for devices with failed last sessions")
				return nil
			}
			window, err := parseCoveSince(since)
			if err != nil {
				return err
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
					coverpc.ColDeviceID, coverpc.ColDeviceName, coverpc.ColCustomer, coverpc.ColProduct,
					coverpc.ColTotalLastStatus, coverpc.ColTotalLastErrors,
					coverpc.ColTotalLastSessionTS, coverpc.ColTotalLastSuccessTS,
				},
				MaxPages: maxScanPages,
			})
			if err != nil {
				return apiErr(err)
			}
			cutoff := time.Time{}
			if window > 0 {
				cutoff = time.Now().Add(-window)
			}
			items := make([]failureItem, 0)
			for _, d := range devices {
				s := d.Settings
				status, ok := coverpc.SettingInt(s, coverpc.ColTotalLastStatus)
				if !ok {
					continue
				}
				neverRun := status == 7
				if !coverpc.BadSessionStatuses[int(status)] && !neverRun {
					continue
				}
				if neverRun && !includeNeverRun {
					continue
				}
				lastSession, hasSession := coverpc.SettingTime(s, coverpc.ColTotalLastSessionTS)
				// --since keeps failures whose last session ran inside the
				// window; never-run devices carry no timestamp and stay.
				if !cutoff.IsZero() && hasSession && lastSession.Before(cutoff) {
					continue
				}
				item := failureItem{
					AccountID:  d.AccountID,
					DeviceName: s[coverpc.ColDeviceName],
					Customer:   s[coverpc.ColCustomer],
					Product:    s[coverpc.ColProduct],
					Status:     status,
					StatusName: coverpc.StatusName(int(status)),
				}
				if v, ok := coverpc.SettingInt(s, coverpc.ColTotalLastErrors); ok {
					item.Errors = v
				}
				if hasSession {
					item.LastSessionAt = lastSession.Format(time.RFC3339)
				}
				if ts, ok := coverpc.SettingTime(s, coverpc.ColTotalLastSuccessTS); ok {
					item.LastSuccessAt = ts.Format(time.RFC3339)
				}
				items = append(items, item)
			}
			sort.Slice(items, func(i, j int) bool {
				if items[i].Customer != items[j].Customer {
					return items[i].Customer < items[j].Customer
				}
				return items[i].DeviceName < items[j].DeviceName
			})
			view := failuresView{
				PartnerID:     root,
				Items:         items,
				TotalFailures: len(items),
				scanCapFields: scanCapFields{
					ScannedDevices: scanned,
					MaxScanPages:   effectiveMaxPages(maxScanPages),
					Note:           scanCapNote(capHit, scanned, effectiveMaxPages(maxScanPages), "failure"),
				},
			}
			if window > 0 {
				view.SinceHours = window.Hours()
			}
			if len(items) == 0 && view.Note == "" {
				view.Note = fmt.Sprintf("no failed last sessions among %d scanned devices", scanned)
			}
			return emitCoveJSON(cmd, flags, view, view.Items)
		},
	}
	cmd.Flags().StringVar(&since, "since", "", "Only failures whose last session ran within this window (e.g. 24h, 7d); never-run devices always included")
	cmd.Flags().Int64Var(&partnerID, "partner-id", 0, "Root partner to sweep (default: the logged-in user's partner)")
	cmd.Flags().IntVar(&maxScanPages, "max-scan-pages", 10, "Maximum statistics pages to scan")
	cmd.Flags().BoolVar(&includeNeverRun, "include-never-run", true, "Include devices whose backup never started (status NotStarted)")
	return cmd
}
