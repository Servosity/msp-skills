// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// devices/availabilities/changeHistory looks back at most 31 days.
const maxAvailabilityLookback = 31 * 24 * time.Hour

type availabilityChange struct {
	TS     string `json:"ts"`
	Device struct {
		Serial      string `json:"serial"`
		Name        string `json:"name"`
		ProductType string `json:"productType"`
		Model       string `json:"model"`
	} `json:"device"`
	Details struct {
		Old []struct{ Name, Value string } `json:"old"`
		New []struct{ Name, Value string } `json:"new"`
	} `json:"details"`
	Network struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"network"`
}

func statusOf(details []struct{ Name, Value string }) string {
	for _, d := range details {
		if d.Name == "status" {
			return strings.ToLower(d.Value)
		}
	}
	return ""
}

type flapRow struct {
	OrganizationID   string `json:"organization_id"`
	OrganizationName string `json:"organization_name"`
	NetworkID        string `json:"network_id"`
	NetworkName      string `json:"network_name"`
	Serial           string `json:"serial"`
	Name             string `json:"name"`
	Model            string `json:"model"`
	ProductType      string `json:"product_type"`
	Transitions      int    `json:"transitions"`
	OfflineEvents    int    `json:"offline_events"`
	DowntimeMinutes  int64  `json:"downtime_minutes"`
	CurrentlyOffline bool   `json:"currently_offline"`
	LastChangeAt     string `json:"last_change_at"`
	WindowStart      string `json:"window_start"`
}

// computeFlaps pairs each device's transitions in time order. Time between a
// change to offline and the next change away from offline is downtime; a
// device still offline at the end of the window counts until now. A device
// whose first event in the window is a recovery was already offline at the
// window start, so that stretch is counted from windowStart.
func computeFlaps(org fleetOrg, changes []availabilityChange, windowStart, now time.Time) []flapRow {
	type ev struct {
		t        time.Time
		status   string
		previous string
	}
	bySerial := map[string][]ev{}
	type newest struct {
		t  time.Time
		ch availabilityChange
	}
	meta := map[string]newest{}
	for _, ch := range changes {
		t, ok := parseMerakiTime(ch.TS)
		st := statusOf(ch.Details.New)
		if !ok || st == "" || ch.Device.Serial == "" {
			continue
		}
		bySerial[ch.Device.Serial] = append(bySerial[ch.Device.Serial], ev{t, st, statusOf(ch.Details.Old)})
		if m, ok := meta[ch.Device.Serial]; !ok || t.After(m.t) {
			meta[ch.Device.Serial] = newest{t, ch} // newest event names the device and network
		}
	}
	rows := make([]flapRow, 0, len(bySerial))
	for serial, evs := range bySerial {
		sort.SliceStable(evs, func(i, j int) bool { return evs[i].t.Before(evs[j].t) })
		m := meta[serial].ch
		row := flapRow{OrganizationID: org.ID, OrganizationName: org.Name, NetworkID: m.Network.ID, NetworkName: m.Network.Name,
			Serial: serial, Name: m.Device.Name, Model: m.Device.Model, ProductType: m.Device.ProductType,
			Transitions: len(evs), LastChangeAt: evs[len(evs)-1].t.Format(time.RFC3339), WindowStart: windowStart.Format(time.RFC3339)}
		var downSince time.Time
		down := false
		if evs[0].status != "offline" && evs[0].previous == "offline" {
			down, downSince = true, windowStart // offline since before the window
		}
		var total time.Duration
		for _, e := range evs {
			if e.status == "offline" {
				row.OfflineEvents++
				if !down {
					down, downSince = true, e.t
				}
				continue
			}
			if down {
				total += e.t.Sub(downSince)
				down = false
			}
		}
		if down {
			total += now.Sub(downSince)
			row.CurrentlyOffline = true
		}
		row.DowntimeMinutes = int64(total / time.Minute)
		rows = append(rows, row)
	}
	return rows
}

func sortFlaps(rows []flapRow) {
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].DowntimeMinutes != rows[j].DowntimeMinutes {
			return rows[i].DowntimeMinutes > rows[j].DowntimeMinutes
		}
		if rows[i].Transitions != rows[j].Transitions {
			return rows[i].Transitions > rows[j].Transitions
		}
		return rows[i].Serial < rows[j].Serial
	})
}

func newNovelFleetFlapsCmd(flags *rootFlags) *cobra.Command {
	var orgFilter []string
	var since string
	var minTransitions, limit int
	cmd := &cobra.Command{
		Use:   "flaps",
		Short: "Rank devices across every org by minutes offline, then by status changes, over a past window (--since, up to 31d)",
		Long: strings.TrimSpace(`
Reads each organization's device availability change history (up to 31 days
back) and pairs every device's transitions: how many status changes, how many
times it went offline, and total minutes offline in the window. Devices still
offline at the end of the window count until now. Ranked by downtime, then by
transitions.

Use this command for device up/down transitions and total downtime over a past window.
Do NOT use this command for devices that are down right now; use 'fleet down' instead.`),
		Example: strings.Trim(`
  cisco-meraki-cli fleet flaps --since 7d --agent
  cisco-meraki-cli fleet flaps --since 24h --min-transitions 4 --json
  cisco-meraki-cli fleet flaps --org "Acme Dental" --limit 20`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "fleet flaps")
			}
			window, err := parseWindow("since", since)
			if err != nil {
				return err
			}
			if minTransitions < 1 || limit < 0 {
				return usageErr(fmt.Errorf("--min-transitions must be >= 1 and --limit >= 0"))
			}
			now := time.Now().UTC()
			windowStart, windowEnd, clamped := fixedWindow(now, window, maxAvailabilityLookback)
			if clamped {
				fmt.Fprintln(cmd.ErrOrStderr(), "warning: availability history looks back at most 31 days; --since was clamped")
			}
			bounds := map[string]string{"t0": windowStart.Format(time.RFC3339), "t1": windowEnd.Format(time.RFC3339)}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			c, orgs, err := fleetClientOrgs(ctx, cmd, flags, orgFilter)
			if err != nil {
				return err
			}
			outcomes := fanOrgs(ctx, orgs, func(ctx context.Context, o fleetOrg) ([]flapRow, error) {
				if !o.APIEnabled {
					return nil, errOrgAPIDisabled
				}
				res, err := merakiGetAll(ctx, c, orgPath("/organizations/{organizationId}/devices/availabilities/changeHistory", o.ID), bounds, 0)
				if err != nil {
					return nil, err
				}
				changes := make([]availabilityChange, 0, len(res.Items))
				for _, raw := range res.Items {
					var ch availabilityChange
					if json.Unmarshal(raw, &ch) == nil {
						changes = append(changes, ch)
					}
				}
				return computeFlaps(o, changes, windowStart, now), nil
			})
			reportFleetFailures(cmd.ErrOrStderr(), outcomes)
			rows := make([]flapRow, 0)
			for _, oc := range outcomes {
				for _, r := range oc.Value {
					if r.Transitions >= minTransitions {
						rows = append(rows, r)
					}
				}
			}
			sortFlaps(rows)
			if limit > 0 && len(rows) > limit {
				rows = rows[:limit]
			}
			return printFleetRows(cmd, flags, rows, []string{"organization_name", "network_name", "serial", "name", "transitions", "downtime_minutes", "currently_offline"}, "No device status changes in the window.")
		},
	}
	addOrgFlag(cmd, &orgFilter)
	cmd.Flags().StringVar(&since, "since", "7d", "Window to analyze, e.g. 24h, 7d (max 31d)")
	cmd.Flags().IntVar(&minTransitions, "min-transitions", 1, "Only devices with at least this many status changes")
	cmd.Flags().IntVar(&limit, "limit", 0, "Return at most N devices (0 = all)")
	return cmd
}
