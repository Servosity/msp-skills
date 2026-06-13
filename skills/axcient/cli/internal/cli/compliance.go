// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel command: compliance evidence export.

// pp:data-source local

package cli

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

type complianceRow struct {
	ClientID            string  `json:"client_id"`
	ClientName          string  `json:"client_name"`
	DeviceID            string  `json:"device_id"`
	DeviceName          string  `json:"device_name"`
	Type                string  `json:"type"`
	D2C                 bool    `json:"d2c"`
	LatestRP            string  `json:"latest_rp,omitempty"`
	RPAgeHours          float64 `json:"rp_age_hours"`
	RPOPass             bool    `json:"rpo_pass"`
	AutoverifyStatus    string  `json:"autoverify_status"`
	AutoverifyHealthy   *bool   `json:"autoverify_healthy,omitempty"`
	AutoverifyTimestamp string  `json:"autoverify_timestamp,omitempty"`
	AutoverifyRP        string  `json:"autoverify_rp,omitempty"`
	ScreenshotURL       string  `json:"screenshot_url,omitempty"`
	AutoverifyPass      bool    `json:"autoverify_pass"`
	Compliant           bool    `json:"compliant"`
}

func newNovelComplianceCmd(flags *rootFlags) *cobra.Command {
	var hours int
	var clientID int64
	var failingOnly bool
	var dbPath string
	cmd := &cobra.Command{
		Use:   "compliance",
		Short: "Exportable per-device backup compliance evidence (RPO + AutoVerify)",
		Long: strings.Trim(`
Use this command to export boot-proof + restore-point-age compliance evidence
for QBR/audit. Each row pairs a device's newest restore-point age (RPO
pass/fail against --hours) with its latest AutoVerify boot-verification
result, including the screenshot URL — evidence no single API endpoint
returns together.

The output is a flat array of rows, so --csv produces a spreadsheet-ready
report and --select narrows columns.

Do NOT use it for raw AutoVerify run detail on one device; use the generated
'device autoverify' instead.

Run 'axcient-cli sync' first.
`, "\n"),
		Example: strings.Trim(`
  # Spreadsheet-ready compliance evidence for one client's QBR
  axcient-cli compliance --client 42 --hours 24 --csv

  # Fleet-wide: only the devices failing compliance, agent-shaped
  axcient-cli compliance --failing-only --agent
`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would export per-device RPO + AutoVerify compliance rows from the local store")
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

			devices, err := loadFleetDevices(db, clientID)
			if err != nil {
				return err
			}
			names := loadClientNames(db)
			autoverify, err := loadLatestAutoverify(db)
			if err != nil {
				return err
			}
			devClients := loadDeviceClientMap(db)
			now := time.Now().UTC()
			threshold := time.Duration(hours) * time.Hour

			rows := make([]complianceRow, 0, len(devices))
			for _, d := range devices {
				rpTime, _, hasRP := d.newestRestorePoint()
				rpoPass := hasRP && now.Sub(rpTime) <= threshold
				av, found := autoverify[d.deviceID()]
				avPass := avPassed(av, found)
				cid := d.resolveClientID(devClients)
				row := complianceRow{
					ClientID:       cid,
					ClientName:     fleetClientName(names, json.Number(cid)),
					DeviceID:       d.deviceID(),
					DeviceName:     d.Name,
					Type:           d.Type,
					D2C:            d.D2C,
					RPAgeHours:     hoursSince(now, rpTime),
					RPOPass:        rpoPass,
					AutoverifyPass: avPass,
					Compliant:      rpoPass && avPass,
				}
				if hasRP {
					row.LatestRP = rpTime.Format(time.RFC3339)
				}
				if found {
					row.AutoverifyStatus = av.Status
					row.AutoverifyHealthy = av.IsHealthy
					row.AutoverifyTimestamp = av.Timestamp
					row.AutoverifyRP = av.RestorePoint
					row.ScreenshotURL = av.ScreenshotURL
				} else {
					row.AutoverifyStatus = "never_run"
				}
				if failingOnly && row.Compliant {
					continue
				}
				rows = append(rows, row)
			}

			if wantsHumanTable(cmd.OutOrStdout(), flags) {
				compliant := 0
				for _, r := range rows {
					if r.Compliant {
						compliant++
					}
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Compliance (RPO %dh + AutoVerify): %d of %d rows compliant\n", hours, compliant, len(rows))
				for _, r := range rows {
					verdict := "PASS"
					if !r.Compliant {
						verdict = "FAIL"
					}
					fmt.Fprintf(cmd.OutOrStdout(), "  %-4s %-24s %-30s rp_age=%.1fh autoverify=%s\n", verdict, r.ClientName, r.DeviceName, r.RPAgeHours, r.AutoverifyStatus)
				}
				if len(devices) == 0 {
					fmt.Fprintln(cmd.OutOrStdout(), "no devices in the local store; run 'axcient-cli sync' first")
				}
				return nil
			}
			return printJSONFiltered(cmd.OutOrStdout(), rows, flags)
		},
	}
	cmd.Flags().IntVar(&hours, "hours", 24, "RPO threshold in hours for the rpo_pass column")
	cmd.Flags().Int64Var(&clientID, "client", 0, "Limit rows to one client ID (0 = all clients)")
	cmd.Flags().BoolVar(&failingOnly, "failing-only", false, "Only emit rows that fail RPO or AutoVerify")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (defaults to the standard local store)")
	return cmd
}
