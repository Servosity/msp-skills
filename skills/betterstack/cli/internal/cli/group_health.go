// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Novel feature (hand-built): per-group health rollup — monitors, heartbeats,
// and open incidents aggregated to monitor-group / heartbeat-group level.
// For MSP-style accounts where one group is one client, this is per-client health.

package cli

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

type groupHealthRow struct {
	GroupID       string `json:"group_id"`
	Name          string `json:"name"`
	Kind          string `json:"kind"` // monitor-group | heartbeat-group
	Total         int    `json:"total"`
	Up            int    `json:"up"`
	Down          int    `json:"down"`
	Paused        int    `json:"paused"`
	OpenIncidents int    `json:"open_incidents"`
}

// buildGroupHealth rolls monitor and heartbeat status up to group level.
// Ungrouped members aggregate under the synthetic "(ungrouped)" row of their
// kind so totals always reconcile with the flat resource lists.
func buildGroupHealth(monitorGroups, heartbeatGroups []groupRow, monitors []monitorRow, heartbeats []heartbeatRow, hbGroupIDs map[string]string, incidents []incidentRow) []groupHealthRow {
	openBySource := openIncidentsBySource(incidents)

	mg := make(map[string]*groupHealthRow)
	for _, g := range monitorGroups {
		name := g.Name
		if name == "" {
			name = g.ID
		}
		mg[g.ID] = &groupHealthRow{GroupID: g.ID, Name: name, Kind: "monitor-group"}
	}
	ungroupedMon := &groupHealthRow{GroupID: "", Name: "(ungrouped)", Kind: "monitor-group"}
	for _, m := range monitors {
		row, ok := mg[m.GroupID]
		if !ok {
			row = ungroupedMon
		}
		row.Total++
		switch {
		case m.Paused:
			row.Paused++
		case monitorDown(m.Status):
			row.Down++
		default:
			row.Up++
		}
		row.OpenIncidents += len(openBySource[m.ID])
	}

	hg := make(map[string]*groupHealthRow)
	for _, g := range heartbeatGroups {
		name := g.Name
		if name == "" {
			name = g.ID
		}
		hg[g.ID] = &groupHealthRow{GroupID: g.ID, Name: name, Kind: "heartbeat-group"}
	}
	ungroupedHb := &groupHealthRow{GroupID: "", Name: "(ungrouped)", Kind: "heartbeat-group"}
	for _, h := range heartbeats {
		row, ok := hg[hbGroupIDs[h.ID]]
		if !ok {
			row = ungroupedHb
		}
		row.Total++
		switch {
		case h.Paused:
			row.Paused++
		case h.Status != "" && !strings.EqualFold(h.Status, "up") && !strings.EqualFold(h.Status, "paused"):
			row.Down++
		default:
			row.Up++
		}
	}

	out := make([]groupHealthRow, 0, len(mg)+len(hg)+2)
	for _, r := range mg {
		out = append(out, *r)
	}
	if ungroupedMon.Total > 0 {
		out = append(out, *ungroupedMon)
	}
	for _, r := range hg {
		out = append(out, *r)
	}
	if ungroupedHb.Total > 0 {
		out = append(out, *ungroupedHb)
	}
	// Worst first: open incidents desc, then down desc, then name.
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].OpenIncidents != out[j].OpenIncidents {
			return out[i].OpenIncidents > out[j].OpenIncidents
		}
		if out[i].Down != out[j].Down {
			return out[i].Down > out[j].Down
		}
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return out[i].GroupID < out[j].GroupID
	})
	return out
}

// pp:data-source local
func newNovelGroupHealthCmd(flags *rootFlags) *cobra.Command {
	var dbPath string

	cmd := &cobra.Command{
		Use:   "group-health",
		Short: "Per-group health rollup: monitor and heartbeat up/down counts plus open incidents for every monitor group and heartbeat group.",
		Long: "Use this command for per-group (per-client) health rollups. " +
			"Do NOT use it for the account-wide one-screen board; use 'fleet' instead. " +
			"Reads the local SQLite mirror; run `sync` first.",
		Example:     "  betterstack-cli group-health\n  betterstack-cli group-health --agent",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			if dryRunOK(flags) {
				return nil
			}
			s, err := openAnalyticsStore(cmd, dbPath)
			if err != nil {
				return err
			}
			defer s.Close()
			maybeEmitSyncHints(cmd, s, "", flags.maxAge)

			monitorGroups, err := loadGroupRows(cmd.Context(), s, "monitor-groups")
			if err != nil {
				return fmt.Errorf("reading monitor groups: %w", err)
			}
			heartbeatGroups, err := loadGroupRows(cmd.Context(), s, "heartbeat-groups")
			if err != nil {
				return fmt.Errorf("reading heartbeat groups: %w", err)
			}
			monitors, err := loadMonitors(cmd.Context(), s)
			if err != nil {
				return fmt.Errorf("reading monitors: %w", err)
			}
			heartbeats, err := loadHeartbeats(cmd.Context(), s)
			if err != nil {
				return fmt.Errorf("reading heartbeats: %w", err)
			}
			hbGroupIDs, err := loadHeartbeatGroupIDs(cmd.Context(), s)
			if err != nil {
				return fmt.Errorf("reading heartbeat group ids: %w", err)
			}
			incidents, err := loadIncidents(cmd.Context(), s)
			if err != nil {
				return fmt.Errorf("reading incidents: %w", err)
			}

			rows := buildGroupHealth(monitorGroups, heartbeatGroups, monitors, heartbeats, hbGroupIDs, incidents)
			if flags.asJSON {
				return flags.printJSON(cmd, rows)
			}
			if len(rows) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No groups, monitors, or heartbeats in the local mirror.")
				return nil
			}
			tbl := make([][]string, 0, len(rows))
			for _, r := range rows {
				tbl = append(tbl, []string{truncateField(r.Name, 36), r.Kind, strconv.Itoa(r.Total), strconv.Itoa(r.Up), strconv.Itoa(r.Down), strconv.Itoa(r.Paused), strconv.Itoa(r.OpenIncidents)})
			}
			return flags.printTable(cmd, []string{"GROUP", "KIND", "TOTAL", "UP", "DOWN", "PAUSED", "OPEN-INC"}, tbl)
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/betterstack-cli/data.db)")
	return cmd
}
