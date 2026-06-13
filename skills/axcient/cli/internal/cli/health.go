// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel command: fleet backup-health sweep.

// pp:data-source local

package cli

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

type healthDeviceView struct {
	DeviceID     string  `json:"device_id"`
	Name         string  `json:"name"`
	Type         string  `json:"type"`
	D2C          bool    `json:"d2c"`
	Status       string  `json:"status"`
	Reason       string  `json:"reason,omitempty"`
	StatusSince  string  `json:"status_since,omitempty"`
	LatestRP     string  `json:"latest_rp,omitempty"`
	RPTarget     string  `json:"rp_target,omitempty"`
	HoursSinceRP float64 `json:"hours_since_rp"`
	Failing      bool    `json:"failing"`
	Stale        bool    `json:"stale"`
}

type healthClientView struct {
	ClientID   string             `json:"client_id"`
	ClientName string             `json:"client_name"`
	Devices    []healthDeviceView `json:"devices"`
}

type healthView struct {
	ThresholdHours int                `json:"threshold_hours"`
	TotalDevices   int                `json:"total_devices"`
	FailingDevices int                `json:"failing_devices"`
	StaleDevices   int                `json:"stale_devices"`
	Clients        []healthClientView `json:"clients"`
	Note           string             `json:"note,omitempty"`
}

func newNovelHealthCmd(flags *rootFlags) *cobra.Command {
	var hours int
	var clientID int64
	var dbPath string
	cmd := &cobra.Command{
		Use:   "health",
		Short: "Fleet-wide failed or stale backups, grouped by client",
		Long: strings.Trim(`
Use this command for the morning fleet-wide failed/stale-backup triage across
ALL clients. It joins synced devices, their health status, and client names in
the local store — answering the question the per-entity API cannot (device
objects carry no client grouping upstream).

A device is reported when its current health status is anything other than
NORMAL (failing) or when its newest restore point — local, cloud, or private
vault — is older than --hours (stale).

Do NOT use it for a single device's full job history; use the generated
'client device job history' commands for one device instead.

Run 'axcient-cli sync' first.
`, "\n"),
		Example: strings.Trim(`
  # Morning sweep: everything failing or stale in the last 24h, agent-shaped
  axcient-cli health --agent

  # Tighter RPO lens: anything without a backup in the last 8 hours
  axcient-cli health --hours 8 --json

  # One client only
  axcient-cli health --client 333 --json
`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would scan local store for failing/stale devices grouped by client")
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

			devices, err := loadFleetDevices(db, clientID)
			if err != nil {
				return err
			}
			names := loadClientNames(db)
			devClients := loadDeviceClientMap(db)
			now := time.Now().UTC()
			threshold := time.Duration(hours) * time.Hour

			grouped := map[string][]healthDeviceView{}
			failing, stale, flagged := 0, 0, 0
			for _, d := range devices {
				rpTime, rpTarget, hasRP := d.newestRestorePoint()
				isStale := !hasRP || now.Sub(rpTime) > threshold
				isFailing := d.isFailing()
				if !isStale && !isFailing {
					continue
				}
				if isFailing {
					failing++
				}
				if isStale {
					stale++
				}
				flagged++
				v := healthDeviceView{
					DeviceID:     d.deviceID(),
					Name:         d.Name,
					Type:         d.Type,
					D2C:          d.D2C,
					Status:       "NORMAL",
					HoursSinceRP: hoursSince(now, rpTime),
					Failing:      isFailing,
					Stale:        isStale,
				}
				if d.Current != nil {
					v.Status = d.Current.Status
					v.Reason = d.Current.Reason
					v.StatusSince = d.Current.Timestamp
				}
				if hasRP {
					v.LatestRP = rpTime.Format(time.RFC3339)
					v.RPTarget = rpTarget
				}
				cid := d.resolveClientID(devClients)
				grouped[cid] = append(grouped[cid], v)
			}

			view := healthView{
				ThresholdHours: hours,
				TotalDevices:   len(devices),
				FailingDevices: failing,
				StaleDevices:   stale,
				Clients:        make([]healthClientView, 0, len(grouped)),
			}
			for _, cid := range sortedClientIDs(grouped) {
				view.Clients = append(view.Clients, healthClientView{
					ClientID:   cid,
					ClientName: fleetClientName(names, json.Number(cid)),
					Devices:    grouped[cid],
				})
			}
			if len(devices) == 0 {
				view.Note = "no devices in the local store; run 'axcient-cli sync' first"
			}

			if wantsHumanTable(cmd.OutOrStdout(), flags) {
				fmt.Fprintf(cmd.OutOrStdout(), "Backup health: %d of %d devices flagged (%d failing, %d stale > %dh)\n",
					flagged, len(devices), failing, stale, hours)
				for _, c := range view.Clients {
					fmt.Fprintf(cmd.OutOrStdout(), "\n%s (client %s)\n", c.ClientName, c.ClientID)
					for _, d := range c.Devices {
						marker := ""
						if d.Failing {
							marker += " [" + d.Status + "]"
						}
						if d.Stale {
							marker += fmt.Sprintf(" [stale %.1fh]", d.HoursSinceRP)
						}
						fmt.Fprintf(cmd.OutOrStdout(), "  %-30s %s%s\n", d.Name, d.Type, marker)
					}
				}
				return nil
			}
			return printJSONFiltered(cmd.OutOrStdout(), view, flags)
		},
	}
	cmd.Flags().IntVar(&hours, "hours", 24, "Restore-point age in hours beyond which a device counts as stale")
	cmd.Flags().Int64Var(&clientID, "client", 0, "Limit the sweep to one client ID (0 = all clients)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (defaults to the standard local store)")
	return cmd
}
