// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-built transcendence feature for levelio-cli. Reads the local store.

package cli

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"levelio-pp-cli/internal/store"
)

type alertRecurrenceRow struct {
	Name       string `json:"name"`
	Severity   string `json:"severity"`
	Count      int    `json:"count"`
	Devices    int    `json:"devices"`
	Unresolved int    `json:"unresolved"`
	FirstSeen  string `json:"first_seen,omitempty"`
	LastSeen   string `json:"last_seen,omitempty"`
}

type alertRecurrenceResult struct {
	WindowDays  int                  `json:"window_days"`
	TotalAlerts int                  `json:"total_alerts"`
	Monitors    []alertRecurrenceRow `json:"monitors"`
}

// lvlComputeAlertRecurrence groups the synced alert history by alert name:
// how often each monitor fired, on how many distinct devices, and how many
// firings are still unresolved — the noisiest-monitors ranking.
func lvlComputeAlertRecurrence(alerts []lvlAlert, windowDays, top int, now time.Time) alertRecurrenceResult {
	res := alertRecurrenceResult{WindowDays: windowDays}

	type agg struct {
		count, unresolved int
		sevWeight         int
		severity          string
		devices           map[string]bool
		first, last       time.Time
	}
	rows := map[string]*agg{}
	order := []string{}

	for _, a := range alerts {
		started, ok := lvlParseTime(a.StartedAt)
		if windowDays > 0 {
			if !ok || now.Sub(started).Hours() > float64(windowDays)*24.0 {
				continue
			}
		}
		name := orUnknown(a.Name)
		r, exists := rows[name]
		if !exists {
			r = &agg{devices: map[string]bool{}}
			rows[name] = r
			order = append(order, name)
		}
		r.count++
		res.TotalAlerts++
		if !a.IsResolved {
			r.unresolved++
		}
		if a.DeviceID != "" {
			r.devices[a.DeviceID] = true
		}
		if w := lvlSeverityWeight(a.Severity); w > r.sevWeight {
			r.sevWeight = w
			r.severity = a.Severity
		}
		if ok {
			if r.first.IsZero() || started.Before(r.first) {
				r.first = started
			}
			if started.After(r.last) {
				r.last = started
			}
		}
	}

	for _, name := range order {
		r := rows[name]
		row := alertRecurrenceRow{
			Name: name, Severity: r.severity, Count: r.count,
			Devices: len(r.devices), Unresolved: r.unresolved,
		}
		if !r.first.IsZero() {
			row.FirstSeen = r.first.Format(time.RFC3339)
		}
		if !r.last.IsZero() {
			row.LastSeen = r.last.Format(time.RFC3339)
		}
		res.Monitors = append(res.Monitors, row)
	}

	sort.SliceStable(res.Monitors, func(i, j int) bool {
		a, b := res.Monitors[i], res.Monitors[j]
		if a.Count != b.Count {
			return a.Count > b.Count
		}
		if a.Devices != b.Devices {
			return a.Devices > b.Devices
		}
		return a.Name < b.Name
	})
	if top > 0 && len(res.Monitors) > top {
		res.Monitors = res.Monitors[:top]
	}
	return res
}

// pp:data-source local
func newNovelAlertRecurrenceCmd(flags *rootFlags) *cobra.Command {
	var top int
	var days int
	var dbPath string

	cmd := &cobra.Command{
		Use:         "alert-recurrence",
		Short:       "Rank which alert names fire most often, and on how many distinct devices",
		Annotations: map[string]string{"mcp:read-only": "true"},
		Long: `Rank the chronically noisy monitors: group the synced alert history by
alert name and report how often each fired, on how many distinct devices, how
many firings are still unresolved, and the first/last occurrence. Computed
offline from the local store. Limit the history window with --days (0 = all
synced history) and the row count with --top.

Use this command to rank the chronically noisiest MONITORS/alert-names
fleet-wide. Do NOT use it to cluster current fires by client group; use
'alert-triage' instead.

Run 'levelio-cli sync' first to populate the local store.`,
		Example: strings.Trim(`
  # The 15 noisiest monitors across all synced history
  levelio-cli alert-recurrence

  # Last 30 days only, JSON for agents
  levelio-cli alert-recurrence --days 30 --agent
`, "\n"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			out := cmd.OutOrStdout()
			if dbPath == "" {
				dbPath = defaultDBPath("levelio-cli")
			}
			db, err := store.OpenWithContext(cmd.Context(), dbPath)
			if err != nil {
				return fmt.Errorf("opening local store: %w\nRun 'levelio-cli sync' first.", err)
			}
			defer db.Close()
			if !hintIfUnsynced(cmd, db, "alerts") {
				hintIfStale(cmd, db, "alerts", flags.maxAge)
			}

			alerts, err := lvlAlerts(db)
			if err != nil {
				return fmt.Errorf("loading alerts: %w", err)
			}
			res := lvlComputeAlertRecurrence(alerts, days, top, time.Now().UTC())

			if flags.asJSON {
				return printJSONFiltered(out, res, flags)
			}
			window := "all synced history"
			if res.WindowDays > 0 {
				window = fmt.Sprintf("last %dd", res.WindowDays)
			}
			fmt.Fprintf(out, "%d monitor(s) over %d alert(s) (%s)\n", len(res.Monitors), res.TotalAlerts, window)
			if len(res.Monitors) == 0 {
				return nil
			}
			fmt.Fprintln(out, "COUNT\tDEVICES\tUNRESOLVED\tSEVERITY\tMONITOR")
			for _, m := range res.Monitors {
				fmt.Fprintf(out, "%d\t%d\t%d\t%s\t%s\n", m.Count, m.Devices, m.Unresolved, orUnknown(m.Severity), m.Name)
			}
			return nil
		},
	}
	cmd.Flags().IntVar(&top, "top", 15, "Maximum monitors to return (0 = all)")
	cmd.Flags().IntVar(&days, "days", 0, "Only count alerts that started in the last N days (0 = all synced history)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	return cmd
}
