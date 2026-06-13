// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel command: per-client backup posture rollup.

// pp:data-source local

package cli

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

type clientRollupRow struct {
	ClientID          string `json:"client_id"`
	ClientName        string `json:"client_name"`
	DevicesTotal      int    `json:"devices_total"`
	Failing           int    `json:"failing"`
	RPOBreaches       int    `json:"rpo_breaches"`
	AutoverifyFailing int    `json:"autoverify_failing"`
	D2CDevices        int    `json:"d2c_devices"`
	CloudUsageBytes   int64  `json:"cloud_usage_bytes"`
	Healthy           bool   `json:"healthy"`
}

func newNovelClientRollupCmd(flags *rootFlags) *cobra.Command {
	var hours int
	var dbPath string
	cmd := &cobra.Command{
		Use:   "client-rollup",
		Short: "One-line-per-client backup posture across the whole fleet",
		Long: strings.Trim(`
Use this command for a one-line-per-client fleet posture summary (the
Hudu-style dashboard rollup): device totals, failing health statuses, RPO
breaches against --hours, AutoVerify failures, and cloud usage — the deepest
join in the local store (clients + devices + AutoVerify), with no upstream
endpoint equivalent.

The autoverify_failing column counts completed-but-failed verifications
only; devices that have never been verified do not count against it. Use
'compliance' for the strict evidence view where never-verified fails.

Do NOT use it for the device-level failure list; use 'health' for the
per-device breakdown instead.

Run 'axcient-cli sync' first.
`, "\n"),
		Example: strings.Trim(`
  # The fleet dashboard, one row per client
  axcient-cli client-rollup --agent

  # Spreadsheet for the weekly service review
  axcient-cli client-rollup --csv
`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would roll up per-client backup posture from the local store")
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
			hintIfUnsynced(cmd, db, "autoverify")

			devices, err := loadFleetDevices(db, 0)
			if err != nil {
				return err
			}
			names := loadClientNames(db)
			autoverify, err := loadLatestAutoverify(db)
			if err != nil {
				return err
			}
			now := time.Now().UTC()
			threshold := time.Duration(hours) * time.Hour

			devClients := loadDeviceClientMap(db)

			byClient := map[string]*clientRollupRow{}
			for _, d := range devices {
				cid := d.resolveClientID(devClients)
				row, ok := byClient[cid]
				if !ok {
					row = &clientRollupRow{
						ClientID:   cid,
						ClientName: fleetClientName(names, json.Number(cid)),
					}
					byClient[cid] = row
				}
				row.DevicesTotal++
				if d.D2C {
					row.D2CDevices++
				}
				row.CloudUsageBytes += d.CloudUsage
				if d.isFailing() {
					row.Failing++
				}
				rpTime, _, hasRP := d.newestRestorePoint()
				if !hasRP || now.Sub(rpTime) > threshold {
					row.RPOBreaches++
				}
				av, found := autoverify[d.deviceID()]
				if found && !avPassed(av, found) {
					row.AutoverifyFailing++
				}
			}

			rows := make([]clientRollupRow, 0, len(byClient))
			for _, cid := range sortedClientIDs(byClient) {
				r := *byClient[cid]
				r.Healthy = r.Failing == 0 && r.RPOBreaches == 0 && r.AutoverifyFailing == 0
				rows = append(rows, r)
			}

			if wantsHumanTable(cmd.OutOrStdout(), flags) {
				fmt.Fprintf(cmd.OutOrStdout(), "Client rollup (%d clients, RPO threshold %dh)\n", len(rows), hours)
				for _, r := range rows {
					badge := "OK  "
					if !r.Healthy {
						badge = "WARN"
					}
					fmt.Fprintf(cmd.OutOrStdout(), "  %s %-28s devices=%-4d failing=%-3d rpo_breach=%-3d av_fail=%-3d cloud=%.1fGB\n",
						badge, r.ClientName, r.DevicesTotal, r.Failing, r.RPOBreaches, r.AutoverifyFailing, float64(r.CloudUsageBytes)/1e9)
				}
				if len(devices) == 0 {
					fmt.Fprintln(cmd.OutOrStdout(), "no devices in the local store; run 'axcient-cli sync' first")
				}
				return nil
			}
			return printJSONFiltered(cmd.OutOrStdout(), rows, flags)
		},
	}
	cmd.Flags().IntVar(&hours, "hours", 24, "RPO threshold in hours for the rpo_breaches column")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (defaults to the standard local store)")
	return cmd
}
