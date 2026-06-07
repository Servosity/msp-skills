// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-built novel feature: month-end billing usage export.
// pp:data-source live
package cli

import (
	"fmt"
	"sort"
	"strings"

	"cove-pp-cli/internal/coverpc"

	"github.com/spf13/cobra"
)

type billingUsageItem struct {
	AccountID        int64   `json:"account_id"`
	DeviceName       string  `json:"device_name"`
	Customer         string  `json:"customer"`
	Product          string  `json:"product"`
	SKU              string  `json:"sku"`
	PrevSKU          string  `json:"prev_sku,omitempty"`
	UsedStorageBytes int64   `json:"used_storage_bytes"`
	UsedStorageGB    float64 `json:"used_storage_gb"`
	M365ExchangeSeat int64   `json:"m365_exchange_licenses,omitempty"`
	M365ExchangeBox  int64   `json:"m365_exchange_mailboxes,omitempty"`
	M365OneDriveSeat int64   `json:"m365_onedrive_licenses,omitempty"`
}

type billingUsageView struct {
	PartnerID      int64              `json:"partner_id"`
	Items          []billingUsageItem `json:"items"`
	TotalDevices   int                `json:"total_devices"`
	TotalStorageGB float64            `json:"total_storage_gb"`
	TotalM365Seats int64              `json:"total_m365_licenses"`
	scanCapFields
}

func newNovelBillingUsageCmd(flags *rootFlags) *cobra.Command {
	var partnerID int64
	var maxScanPages int
	cmd := &cobra.Command{
		Use:   "usage",
		Short: "Per-device SKU, used storage, and M365 seats — the month-end billing export",
		Long: strings.Trim(`
Pulls the billing column preset — SKU (I57), previous-month SKU (I58), used
storage (I14), and Microsoft 365 seat counts (D19F13/D19F20/D20F13) — for
every device under the partner tree, decoded to human names. Replaces the
per-partner PowerShell report scripts with one cross-tree call.

Use this command for the CURRENT-period billing snapshot. Do NOT use it to
find what changed since last month; use 'billing changes' instead.
`, "\n"),
		Example: strings.Trim(`
  cove-cli billing usage --csv > usage.csv
  cove-cli billing usage --json --select items.customer,items.sku,items.used_storage_gb`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only":          "true",
			"pp:no-error-path-probe": "true",
			"pp:requires-tier":       "credentials",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would export per-device SKU, storage, and M365 seat usage")
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
				Columns: []string{
					coverpc.ColDeviceID, coverpc.ColDeviceName, coverpc.ColCustomer, coverpc.ColProduct,
					coverpc.ColSKU, coverpc.ColPrevSKU, coverpc.ColUsedStorage,
					coverpc.ColM365ExchangeLicenses, coverpc.ColM365ExchangeMailbox, coverpc.ColM365OneDriveLicenses,
				},
				MaxPages: maxScanPages,
			})
			if err != nil {
				return apiErr(err)
			}
			view := billingUsageView{PartnerID: root, Items: make([]billingUsageItem, 0, len(devices))}
			for _, d := range devices {
				s := d.Settings
				item := billingUsageItem{
					AccountID:  d.AccountID,
					DeviceName: s[coverpc.ColDeviceName],
					Customer:   s[coverpc.ColCustomer],
					Product:    s[coverpc.ColProduct],
					SKU:        s[coverpc.ColSKU],
					PrevSKU:    s[coverpc.ColPrevSKU],
				}
				if v, ok := coverpc.SettingInt(s, coverpc.ColUsedStorage); ok {
					item.UsedStorageBytes = v
					item.UsedStorageGB = float64(int(float64(v)/1e9*100)) / 100
				}
				if v, ok := coverpc.SettingInt(s, coverpc.ColM365ExchangeLicenses); ok {
					item.M365ExchangeSeat = v
				}
				if v, ok := coverpc.SettingInt(s, coverpc.ColM365ExchangeMailbox); ok {
					item.M365ExchangeBox = v
				}
				if v, ok := coverpc.SettingInt(s, coverpc.ColM365OneDriveLicenses); ok {
					item.M365OneDriveSeat = v
				}
				view.Items = append(view.Items, item)
				view.TotalStorageGB += item.UsedStorageGB
				view.TotalM365Seats += item.M365ExchangeSeat + item.M365OneDriveSeat
			}
			sort.Slice(view.Items, func(i, j int) bool {
				if view.Items[i].Customer != view.Items[j].Customer {
					return view.Items[i].Customer < view.Items[j].Customer
				}
				return view.Items[i].DeviceName < view.Items[j].DeviceName
			})
			view.TotalDevices = len(view.Items)
			view.TotalStorageGB = float64(int(view.TotalStorageGB*100)) / 100
			view.scanCapFields = scanCapFields{
				ScannedDevices: scanned,
				MaxScanPages:   effectiveMaxPages(maxScanPages),
				Note:           scanCapNote(capHit, scanned, effectiveMaxPages(maxScanPages), "billing"),
			}
			return emitCoveJSON(cmd, flags, view, view.Items)
		},
	}
	cmd.Flags().Int64Var(&partnerID, "partner-id", 0, "Root partner to export (default: the logged-in user's partner)")
	cmd.Flags().IntVar(&maxScanPages, "max-scan-pages", 10, "Maximum statistics pages to scan")
	return cmd
}
