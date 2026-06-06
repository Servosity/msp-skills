// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel-feature command (Printing Press transcendence): the
// tenant exposure trend. Diffs the current sync's per-tenant posture snapshot
// against the prior one retained in the local store — degradation detection
// the live API cannot do because it only ever shows current state.

package cli

import (
	"sort"
	"time"

	"github.com/spf13/cobra"
)

// fleetTrendRow is one tenant's week-over-week posture delta.
type fleetTrendRow struct {
	CID                 string     `json:"cid"`
	Current             time.Time  `json:"current_snapshot"`
	Prior               *time.Time `json:"prior_snapshot,omitempty"`
	Hosts               int        `json:"hosts"`
	HostsDelta          int        `json:"hosts_delta"`
	CriticalAlerts      int        `json:"critical_alerts"`
	CriticalAlertsDelta int        `json:"critical_alerts_delta"`
	CriticalVulns       int        `json:"critical_vulns"`
	CriticalVulnsDelta  int        `json:"critical_vulns_delta"`
	Direction           string     `json:"direction"` // worse | better | flat | baseline
}

// fleetTrendView is the `fleet trend` response envelope.
type fleetTrendView struct {
	Tenants []fleetTrendRow `json:"tenants"`
	Note    string          `json:"note,omitempty"`
}

// trendFromSnapshots computes per-CID deltas between each tenant's two most
// recent snapshots. Pure function; expects rows ordered newest-first per CID
// (loadFleetSnapshots guarantees this).
func trendFromSnapshots(snaps []fleetSnapshot) fleetTrendView {
	view := fleetTrendView{}
	byCID := map[string][]fleetSnapshot{}
	order := []string{}
	for _, s := range snaps {
		if _, ok := byCID[s.CID]; !ok {
			order = append(order, s.CID)
		}
		byCID[s.CID] = append(byCID[s.CID], s)
	}

	baselineOnly := 0
	for _, cid := range order {
		gens := byCID[cid]
		cur := gens[0]
		row := fleetTrendRow{
			CID:            cid,
			Current:        cur.TakenAt,
			Hosts:          cur.Hosts,
			CriticalAlerts: cur.CriticalAlerts,
			CriticalVulns:  cur.CriticalVulns,
		}
		if len(gens) < 2 {
			row.Direction = "baseline"
			baselineOnly++
		} else {
			prior := gens[1]
			t := prior.TakenAt
			row.Prior = &t
			row.HostsDelta = cur.Hosts - prior.Hosts
			row.CriticalAlertsDelta = cur.CriticalAlerts - prior.CriticalAlerts
			row.CriticalVulnsDelta = cur.CriticalVulns - prior.CriticalVulns
			switch {
			case row.CriticalAlertsDelta > 0 || row.CriticalVulnsDelta > 0:
				row.Direction = "worse"
			case row.CriticalAlertsDelta < 0 || row.CriticalVulnsDelta < 0:
				row.Direction = "better"
			default:
				row.Direction = "flat"
			}
		}
		view.Tenants = append(view.Tenants, row)
	}

	// Worst tenants first: worse > flat > better > baseline, then by combined
	// degradation magnitude.
	rank := map[string]int{"worse": 0, "flat": 1, "better": 2, "baseline": 3}
	sort.SliceStable(view.Tenants, func(i, j int) bool {
		a, b := view.Tenants[i], view.Tenants[j]
		if rank[a.Direction] != rank[b.Direction] {
			return rank[a.Direction] < rank[b.Direction]
		}
		da := a.CriticalAlertsDelta + a.CriticalVulnsDelta
		db := b.CriticalAlertsDelta + b.CriticalVulnsDelta
		if da != db {
			return da > db
		}
		return a.CID < b.CID
	})

	switch {
	case len(view.Tenants) == 0:
		view.Note = "no posture snapshots in the local store; run 'fleet sync' at least once"
	case baselineOnly == len(view.Tenants):
		view.Note = "only one sync generation recorded; deltas appear after the next 'fleet sync'"
	}
	return view
}

// pp:data-source local
func newNovelFleetTrendCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	cmd := &cobra.Command{
		Use:   "trend",
		Short: "See which tenants got worse since the last sync: deltas in critical alerts, critical vulns, and host counts",
		Long: "Use this command to see which tenants degraded SINCE the last sync " +
			"(week-over-week deltas computed from the posture snapshots each 'fleet sync' " +
			"records). Worst tenants sort first. Run 'fleet sync' at least twice for deltas.\n" +
			"Do NOT use this command for the current-state snapshot; use 'fleet scorecard' " +
			"instead.",
		Example:     "  crowdstrike-cli fleet trend --json",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			st, err := openFleetStore(cmd.Context(), resolveFleetDB(dbPath))
			if err != nil {
				return configErr(err)
			}
			defer st.Close()
			if !hintIfFleetUnsynced(cmd, st) {
				hintIfFleetStale(cmd, st, flags.maxAge)
			}
			snaps, err := loadFleetSnapshots(cmd.Context(), st)
			if err != nil {
				return configErr(err)
			}
			return flags.printJSON(cmd, trendFromSnapshots(snaps))
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Local SQLite store path (default: standard data dir)")
	return cmd
}
