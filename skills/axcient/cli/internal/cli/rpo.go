// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel command: RPO breach report.

// pp:data-source local

package cli

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

type rpoBreachView struct {
	ClientID     string  `json:"client_id"`
	ClientName   string  `json:"client_name"`
	DeviceID     string  `json:"device_id"`
	Name         string  `json:"name"`
	Type         string  `json:"type"`
	D2C          bool    `json:"d2c"`
	Target       string  `json:"target"`
	LatestRP     string  `json:"latest_rp,omitempty"`
	HoursSinceRP float64 `json:"hours_since_rp"`
	NeverRun     bool    `json:"never_run"`
}

type rpoView struct {
	ThresholdHours int             `json:"threshold_hours"`
	Target         string          `json:"target"`
	TotalDevices   int             `json:"total_devices"`
	Breaches       []rpoBreachView `json:"breaches"`
	Note           string          `json:"note,omitempty"`
}

func newNovelRpoCmd(flags *rootFlags) *cobra.Command {
	var hours int
	var target string
	var clientID int64
	var dbPath string
	cmd := &cobra.Command{
		Use:   "rpo",
		Short: "Devices whose newest restore point breaches an RPO target",
		Long: strings.Trim(`
Use this command to find devices breaching a restore-point-age (RPO) target.
It reads the synced device rows and compares each device's newest restore
point against --hours, grouped by client. --target narrows the check to one
replication tier: local (appliance), cloud (Axcient vault), vault (private
vault), or any (the newest across all three, the default).

Do NOT use it for job-run failures (a job can succeed yet RPO still slip);
use 'health' for last-job status instead.

Run 'axcient-cli sync' first.
`, "\n"),
		Example: strings.Trim(`
  # Everything without any restore point in the last 24 hours
  axcient-cli rpo --hours 24 --agent

  # Cloud-replication SLA check: nothing replicated offsite in 48h
  axcient-cli rpo --hours 48 --target cloud --json
`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would scan local store for devices breaching the restore-point age target")
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			switch target {
			case "any", "local", "cloud", "vault":
			default:
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--target must be one of any, local, cloud, vault (got %q)", target))
			}
			db, err := openFleetStore(dbPath)
			if err != nil {
				return fmt.Errorf("opening local store: %w", err)
			}
			defer db.Close()
			if !hintIfUnsynced(cmd, db, "device") {
				hintIfStale(cmd, db, "device", flags.maxAge)
			}

			devices, err := loadFleetDevices(db, clientID)
			if err != nil {
				return err
			}
			names := loadClientNames(db)
			devClients := loadDeviceClientMap(db)
			now := time.Now().UTC()
			threshold := time.Duration(hours) * time.Hour

			breaches := make([]rpoBreachView, 0)
			for _, d := range devices {
				rpTime, rpSource, hasRP := d.restorePointFor(target)
				if hasRP && now.Sub(rpTime) <= threshold {
					continue
				}
				cid := d.resolveClientID(devClients)
				v := rpoBreachView{
					ClientID:     cid,
					ClientName:   fleetClientName(names, json.Number(cid)),
					DeviceID:     d.deviceID(),
					Name:         d.Name,
					Type:         d.Type,
					D2C:          d.D2C,
					Target:       target,
					HoursSinceRP: hoursSince(now, rpTime),
					NeverRun:     !hasRP,
				}
				if hasRP {
					v.LatestRP = rpTime.Format(time.RFC3339)
					if rpSource != "" {
						v.Target = rpSource
					}
				}
				breaches = append(breaches, v)
			}

			view := rpoView{
				ThresholdHours: hours,
				Target:         target,
				TotalDevices:   len(devices),
				Breaches:       breaches,
			}
			if len(devices) == 0 {
				view.Note = "no devices in the local store; run 'axcient-cli sync' first"
			}

			if wantsHumanTable(cmd.OutOrStdout(), flags) {
				fmt.Fprintf(cmd.OutOrStdout(), "RPO check (%s > %dh): %d of %d devices breaching\n",
					target, hours, len(breaches), len(devices))
				for _, b := range breaches {
					age := fmt.Sprintf("%.1fh", b.HoursSinceRP)
					if b.NeverRun {
						age = "never"
					}
					fmt.Fprintf(cmd.OutOrStdout(), "  %-24s %-30s %s since last %s restore point\n", b.ClientName, b.Name, age, b.Target)
				}
				return nil
			}
			return printJSONFiltered(cmd.OutOrStdout(), view, flags)
		},
	}
	cmd.Flags().IntVar(&hours, "hours", 24, "RPO threshold in hours")
	cmd.Flags().StringVar(&target, "target", "any", "Replication tier to check: any, local, cloud, or vault")
	cmd.Flags().Int64Var(&clientID, "client", 0, "Limit the check to one client ID (0 = all clients)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (defaults to the standard local store)")
	return cmd
}
