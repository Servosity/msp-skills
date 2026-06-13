// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel command: appliance-to-device assignment map.

// pp:data-source local

package cli

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

type applianceMapDevice struct {
	DeviceID     string  `json:"device_id"`
	Name         string  `json:"name"`
	Type         string  `json:"type"`
	Status       string  `json:"status"`
	LatestRP     string  `json:"latest_rp,omitempty"`
	HoursSinceRP float64 `json:"hours_since_rp"`
}

type applianceMapRow struct {
	ApplianceID string               `json:"appliance_id"`
	Alias       string               `json:"alias"`
	ServiceID   string               `json:"service_id"`
	ClientID    string               `json:"client_id"`
	ClientName  string               `json:"client_name"`
	IPAddress   string               `json:"ip_address,omitempty"`
	Active      bool                 `json:"active"`
	DeviceCount int                  `json:"device_count"`
	Devices     []applianceMapDevice `json:"devices"`
}

type applianceMapView struct {
	TotalAppliances int               `json:"total_appliances"`
	AssignedDevices int               `json:"assigned_devices"`
	UnmappedDevices int               `json:"unmapped_devices"`
	Appliances      []applianceMapRow `json:"appliances"`
	Note            string            `json:"note,omitempty"`
}

func newNovelApplianceMapCmd(flags *rootFlags) *cobra.Command {
	var clientID int64
	var dbPath string
	cmd := &cobra.Command{
		Use:   "appliance-map",
		Short: "Which devices each appliance protects, with each device's backup state",
		Long: strings.Trim(`
Join appliance-to-device assignment with each device's current health status
and newest restore point. The upstream appliance endpoint can include its
devices but says nothing about their backup health; this local join adds the
missing column. Appliances and devices pair on their shared service SID.

D2C (Direct-to-Cloud) devices have no appliance and are reported in the
unmapped count, not under an appliance.

Run 'axcient-cli sync' first.
`, "\n"),
		Example: strings.Trim(`
  # Fleet-wide appliance triage view
  axcient-cli appliance-map --agent

  # One client's appliances only
  axcient-cli appliance-map --client 333 --json
`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would join appliances with their protected devices from the local store")
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
			if !hintIfUnsynced(cmd, db, "appliance") {
				hintIfStale(cmd, db, "appliance", flags.maxAge)
			}
			hintIfUnsynced(cmd, db, "device")

			appliances, err := loadFleetAppliances(db)
			if err != nil {
				return err
			}
			devices, err := loadFleetDevices(db, 0)
			if err != nil {
				return err
			}
			names := loadClientNames(db)
			now := time.Now().UTC()

			// Index devices by the service SID they share with their appliance.
			devicesBySID := map[string][]applianceMapDevice{}
			mapped := 0
			applianceSIDs := map[string]bool{}
			for _, a := range appliances {
				if a.ServiceID != "" {
					applianceSIDs[a.ServiceID] = true
				}
			}
			for _, d := range devices {
				if d.D2C || d.ServiceID == "" || !applianceSIDs[d.ServiceID] {
					continue
				}
				rpTime, _, hasRP := d.newestRestorePoint()
				v := applianceMapDevice{
					DeviceID:     d.deviceID(),
					Name:         d.Name,
					Type:         d.Type,
					Status:       "NORMAL",
					HoursSinceRP: hoursSince(now, rpTime),
				}
				if d.Current != nil && d.Current.Status != "" {
					v.Status = d.Current.Status
				}
				if hasRP {
					v.LatestRP = rpTime.Format(time.RFC3339)
				}
				devicesBySID[d.ServiceID] = append(devicesBySID[d.ServiceID], v)
				mapped++
			}

			rows := make([]applianceMapRow, 0, len(appliances))
			for _, a := range appliances {
				if clientID > 0 {
					cid, _ := a.ClientID.Int64()
					if cid != clientID {
						continue
					}
				}
				assigned := devicesBySID[a.ServiceID]
				if assigned == nil {
					assigned = make([]applianceMapDevice, 0)
				}
				rows = append(rows, applianceMapRow{
					ApplianceID: a.applianceID(),
					Alias:       a.Alias,
					ServiceID:   a.ServiceID,
					ClientID:    a.ClientID.String(),
					ClientName:  fleetClientName(names, json.Number(a.ClientID.String())),
					IPAddress:   a.IPAddress,
					Active:      a.Active,
					DeviceCount: len(assigned),
					Devices:     assigned,
				})
			}

			view := applianceMapView{
				TotalAppliances: len(rows),
				AssignedDevices: mapped,
				UnmappedDevices: len(devices) - mapped,
				Appliances:      rows,
			}
			if len(appliances) == 0 {
				view.Note = "no appliances in the local store; run 'axcient-cli sync' first"
			}

			if wantsHumanTable(cmd.OutOrStdout(), flags) {
				fmt.Fprintf(cmd.OutOrStdout(), "Appliance map: %d appliances, %d assigned devices (%d unmapped/D2C)\n",
					view.TotalAppliances, view.AssignedDevices, view.UnmappedDevices)
				for _, r := range rows {
					state := "active"
					if !r.Active {
						state = "inactive"
					}
					fmt.Fprintf(cmd.OutOrStdout(), "\n%s [%s] — %s (%s, %d devices)\n", r.Alias, r.ServiceID, r.ClientName, state, r.DeviceCount)
					for _, d := range r.Devices {
						fmt.Fprintf(cmd.OutOrStdout(), "  %-30s %-12s %s\n", d.Name, d.Type, d.Status)
					}
				}
				return nil
			}
			return printJSONFiltered(cmd.OutOrStdout(), view, flags)
		},
	}
	cmd.Flags().Int64Var(&clientID, "client", 0, "Limit the map to one client ID (0 = all clients)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (defaults to the standard local store)")
	return cmd
}
