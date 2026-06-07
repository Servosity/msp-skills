// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-built novel feature: SKU change detection via Cove's built-in
// previous-month SKU column.
// pp:data-source live
package cli

import (
	"fmt"
	"sort"
	"strings"

	"cove-pp-cli/internal/coverpc"

	"github.com/spf13/cobra"
)

type billingChangeItem struct {
	AccountID  int64  `json:"account_id"`
	DeviceName string `json:"device_name"`
	Customer   string `json:"customer"`
	PrevSKU    string `json:"prev_sku"`
	SKU        string `json:"sku"`
}

type billingChangesView struct {
	PartnerID    int64               `json:"partner_id"`
	Items        []billingChangeItem `json:"items"`
	TotalChanges int                 `json:"total_changes"`
	scanCapFields
}

func newNovelBillingChangesCmd(flags *rootFlags) *cobra.Command {
	var partnerID int64
	var maxScanPages int
	cmd := &cobra.Command{
		Use:   "changes",
		Short: "Devices whose SKU changed since last month",
		Long: strings.Trim(`
Compares the current SKU (I57) against Cove's purpose-built previous-month
SKU column (I58) and lists every device whose plan changed — the upgrades,
downgrades, and product switches that affect this month's invoices. Replaces
the analyst's manual two-CSV diff.

Use this command to find plan CHANGES since last month. Do NOT use it for
the full current billing snapshot; use 'billing usage' instead.
`, "\n"),
		Example: strings.Trim(`
  cove-cli billing changes --json
  cove-cli billing changes --partner-id 1234 --csv`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only":          "true",
			"pp:no-error-path-probe": "true",
			"pp:requires-tier":       "credentials",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would list devices whose SKU differs from last month")
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
					coverpc.ColDeviceID, coverpc.ColDeviceName, coverpc.ColCustomer,
					coverpc.ColSKU, coverpc.ColPrevSKU,
				},
				MaxPages: maxScanPages,
			})
			if err != nil {
				return apiErr(err)
			}
			items := make([]billingChangeItem, 0)
			for _, d := range devices {
				s := d.Settings
				sku, prev := s[coverpc.ColSKU], s[coverpc.ColPrevSKU]
				// Both sides must be present: a blank prev-SKU is a brand-new
				// device, not a plan change.
				if sku == "" || prev == "" || sku == prev {
					continue
				}
				items = append(items, billingChangeItem{
					AccountID:  d.AccountID,
					DeviceName: s[coverpc.ColDeviceName],
					Customer:   s[coverpc.ColCustomer],
					PrevSKU:    prev,
					SKU:        sku,
				})
			}
			sort.Slice(items, func(i, j int) bool {
				if items[i].Customer != items[j].Customer {
					return items[i].Customer < items[j].Customer
				}
				return items[i].DeviceName < items[j].DeviceName
			})
			view := billingChangesView{
				PartnerID:    root,
				Items:        items,
				TotalChanges: len(items),
				scanCapFields: scanCapFields{
					ScannedDevices: scanned,
					MaxScanPages:   effectiveMaxPages(maxScanPages),
					Note:           scanCapNote(capHit, scanned, effectiveMaxPages(maxScanPages), "SKU-change"),
				},
			}
			if len(items) == 0 && view.Note == "" {
				view.Note = fmt.Sprintf("no SKU changes among %d scanned devices", scanned)
			}
			return emitCoveJSON(cmd, flags, view, view.Items)
		},
	}
	cmd.Flags().Int64Var(&partnerID, "partner-id", 0, "Root partner to scan (default: the logged-in user's partner)")
	cmd.Flags().IntVar(&maxScanPages, "max-scan-pages", 10, "Maximum statistics pages to scan")
	return cmd
}
