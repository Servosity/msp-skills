// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-built novel feature: fleet health rollup.
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

type fleetPartnerHealth struct {
	Customer string `json:"customer"`
	Total    int    `json:"total"`
	Healthy  int    `json:"healthy"`
	Failed   int    `json:"failed"`
	Stale    int    `json:"stale"`
	NeverRun int    `json:"never_run"`
}

type fleetHealthView struct {
	PartnerID    int64                `json:"partner_id"`
	TotalDevices int                  `json:"total_devices"`
	Healthy      int                  `json:"healthy"`
	Failed       int                  `json:"failed"`
	Stale        int                  `json:"stale"`
	NeverRun     int                  `json:"never_run"`
	InProgress   int                  `json:"in_progress"`
	StaleDays    float64              `json:"stale_threshold_days"`
	ByStatus     map[string]int       `json:"by_status"`
	ByPartner    []fleetPartnerHealth `json:"by_partner,omitempty"`
	scanCapFields
}

func newNovelFleetHealthCmd(flags *rootFlags) *cobra.Command {
	var by string
	var partnerID int64
	var staleDays float64
	var maxScanPages int
	cmd := &cobra.Command{
		Use:   "health",
		Short: "One-screen fleet rollup: healthy, failed, stale, never-run",
		Long: strings.Trim(`
Aggregates device statistics across every customer into the single-pane view
the console structurally withholds: totals for healthy, failed, stale, and
never-run devices, a by-status breakdown, and (with --by partner) a
per-customer table.

Every device lands in exactly one primary bucket (healthy, failed, stale,
never-run), so the counts always sum to the total. Failures dominate: a
failed-and-stale device counts as failed. A completed or in-progress device
counts as stale when its last successful session is older than --stale-days.
in_progress is reported as a separate overlay metric.
`, "\n"),
		Example: strings.Trim(`
  cove-cli fleet health --json
  cove-cli fleet health --by partner --json
  cove-cli fleet health --stale-days 7 --partner-id 1234`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only":          "true",
			"pp:no-error-path-probe": "true",
			"pp:requires-tier":       "credentials",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would aggregate fleet health across the partner tree")
				return nil
			}
			if by != "" && by != "partner" {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--by supports only 'partner', got %q", by))
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
					coverpc.ColTotalLastStatus, coverpc.ColTotalLastSuccessTS,
				},
				MaxPages: maxScanPages,
			})
			if err != nil {
				return apiErr(err)
			}
			now := time.Now()
			staleCutoff := time.Duration(staleDays * 24 * float64(time.Hour))
			view := fleetHealthView{
				PartnerID: root,
				StaleDays: staleDays,
				ByStatus:  map[string]int{},
			}
			perPartner := map[string]*fleetPartnerHealth{}
			for _, d := range devices {
				s := d.Settings
				view.TotalDevices++
				customer := s[coverpc.ColCustomer]
				ph := perPartner[customer]
				if ph == nil {
					ph = &fleetPartnerHealth{Customer: customer}
					perPartner[customer] = ph
				}
				ph.Total++
				status, hasStatus := coverpc.SettingInt(s, coverpc.ColTotalLastStatus)
				if hasStatus {
					view.ByStatus[coverpc.StatusName(int(status))]++
				}
				lastSuccess, hasSuccess := coverpc.SettingTime(s, coverpc.ColTotalLastSuccessTS)
				isStale := !hasSuccess || now.Sub(lastSuccess) > staleCutoff
				switch {
				case hasStatus && status == 7:
					view.NeverRun++
					ph.NeverRun++
				case hasStatus && coverpc.BadSessionStatuses[int(status)]:
					view.Failed++
					ph.Failed++
				case hasStatus && (status == 1 || status == 9 || status == 12):
					// In-progress is an overlay metric; the device still lands
					// in exactly one primary bucket so totals reconcile.
					view.InProgress++
					if isStale {
						view.Stale++
						ph.Stale++
					} else {
						view.Healthy++
						ph.Healthy++
					}
				case isStale:
					view.Stale++
					ph.Stale++
				default:
					view.Healthy++
					ph.Healthy++
				}
			}
			if by == "partner" {
				for _, k := range sortedKeys(perPartner) {
					view.ByPartner = append(view.ByPartner, *perPartner[k])
				}
				sort.Slice(view.ByPartner, func(i, j int) bool {
					if view.ByPartner[i].Failed != view.ByPartner[j].Failed {
						return view.ByPartner[i].Failed > view.ByPartner[j].Failed
					}
					return view.ByPartner[i].Customer < view.ByPartner[j].Customer
				})
			}
			view.scanCapFields = scanCapFields{
				ScannedDevices: scanned,
				MaxScanPages:   effectiveMaxPages(maxScanPages),
				Note:           scanCapNote(capHit, scanned, effectiveMaxPages(maxScanPages), "health"),
			}
			return emitCoveJSON(cmd, flags, view, view.ByPartner)
		},
	}
	cmd.Flags().StringVar(&by, "by", "", "Breakdown dimension: 'partner' adds a per-customer table")
	cmd.Flags().Int64Var(&partnerID, "partner-id", 0, "Root partner to roll up (default: the logged-in user's partner)")
	cmd.Flags().Float64Var(&staleDays, "stale-days", 3, "Days without a successful session before a device counts as stale")
	cmd.Flags().IntVar(&maxScanPages, "max-scan-pages", 10, "Maximum statistics pages to scan")
	return cmd
}
