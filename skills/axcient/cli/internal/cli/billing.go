// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel command: per-client usage reconciliation.

// pp:data-source local

package cli

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

type billingRow struct {
	ClientID         string `json:"client_id"`
	ClientName       string `json:"client_name"`
	DevicesTotal     int    `json:"devices_total"`
	Servers          int    `json:"servers"`
	Workstations     int    `json:"workstations"`
	NAS              int    `json:"nas"`
	D2CDevices       int    `json:"d2c_devices"`
	ApplianceDevices int    `json:"appliance_devices"`
	LocalUsageBytes  int64  `json:"local_usage_bytes"`
	CloudUsageBytes  int64  `json:"cloud_usage_bytes"`
	VaultUsageBytes  int64  `json:"vault_usage_bytes"`
	TotalUsageBytes  int64  `json:"total_usage_bytes"`
}

func newNovelBillingCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	cmd := &cobra.Command{
		Use:   "billing",
		Short: "Per-client protected-system counts and storage usage in one table",
		Long: strings.Trim(`
Roll up protected-system counts (by type and by D2C vs appliance-based) and
local/cloud/private-vault storage usage for every client into one exportable
table. The upstream usage endpoint is strictly per-client; this cross-client
rollup only exists in the local store.

The output is a flat array of rows, so --csv produces the month-end invoice
reconciliation sheet directly.

Run 'axcient-cli sync' first.
`, "\n"),
		Example: strings.Trim(`
  # Month-end reconciliation sheet
  axcient-cli billing --csv

  # Agent-shaped, just the headline columns
  axcient-cli billing --agent --select client_name,devices_total,total_usage_bytes
`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would roll up per-client device counts and storage usage from the local store")
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			db, err := openFleetStore(dbPath)
			if err != nil {
				return fmt.Errorf("opening local store: %w", err)
			}
			defer db.Close()
			if !hintIfUnsynced(cmd, db, "device") {
				hintIfStale(cmd, db, "device", flags.maxAge)
			}

			devices, err := loadFleetDevices(db, 0)
			if err != nil {
				return err
			}
			names := loadClientNames(db)
			devClients := loadDeviceClientMap(db)

			byClient := map[string]*billingRow{}
			for _, d := range devices {
				cid := d.resolveClientID(devClients)
				row, ok := byClient[cid]
				if !ok {
					row = &billingRow{
						ClientID:   cid,
						ClientName: fleetClientName(names, json.Number(cid)),
					}
					byClient[cid] = row
				}
				row.DevicesTotal++
				switch strings.ToUpper(d.Type) {
				case "SERVER":
					row.Servers++
				case "WORKSTATION":
					row.Workstations++
				case "NAS":
					row.NAS++
				}
				if d.D2C {
					row.D2CDevices++
				} else {
					row.ApplianceDevices++
				}
				row.LocalUsageBytes += d.LocalUsage
				row.CloudUsageBytes += d.CloudUsage
				row.VaultUsageBytes += d.VaultUsage
				row.TotalUsageBytes += d.LocalUsage + d.CloudUsage + d.VaultUsage
			}

			rows := make([]billingRow, 0, len(byClient))
			for _, cid := range sortedClientIDs(byClient) {
				rows = append(rows, *byClient[cid])
			}
			sort.SliceStable(rows, func(i, j int) bool { return rows[i].TotalUsageBytes > rows[j].TotalUsageBytes })

			if wantsHumanTable(cmd.OutOrStdout(), flags) {
				fmt.Fprintf(cmd.OutOrStdout(), "Usage rollup: %d clients, %d devices\n", len(rows), len(devices))
				for _, r := range rows {
					fmt.Fprintf(cmd.OutOrStdout(), "  %-28s devices=%-4d (srv %d / wks %d / nas %d, d2c %d) cloud=%.1fGB local=%.1fGB vault=%.1fGB\n",
						r.ClientName, r.DevicesTotal, r.Servers, r.Workstations, r.NAS, r.D2CDevices,
						float64(r.CloudUsageBytes)/1e9, float64(r.LocalUsageBytes)/1e9, float64(r.VaultUsageBytes)/1e9)
				}
				if len(devices) == 0 {
					fmt.Fprintln(cmd.OutOrStdout(), "no devices in the local store; run 'axcient-cli sync' first")
				}
				return nil
			}
			return printJSONFiltered(cmd.OutOrStdout(), rows, flags)
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (defaults to the standard local store)")
	return cmd
}
