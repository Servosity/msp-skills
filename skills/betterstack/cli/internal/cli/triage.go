// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Novel feature (hand-built): acknowledgement-aware aging of open incidents —
// never-acknowledged first, oldest first.

package cli

import (
	"fmt"
	"sort"
	"time"

	"github.com/spf13/cobra"
)

type triageItem struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Monitor        string `json:"monitor,omitempty"`
	State          string `json:"state"` // never-acknowledged | acknowledged-unresolved
	StartedAt      string `json:"started_at"`
	AcknowledgedAt string `json:"acknowledged_at,omitempty"`
	AgeMinutes     int    `json:"age_minutes"`
	Age            string `json:"age"`
}

// rankTriage filters to open incidents and ranks them never-acknowledged
// first, then oldest first. monitorNames maps monitor id -> display name.
func rankTriage(incidents []incidentRow, monitorNames map[string]string, now time.Time) []triageItem {
	items := make([]triageItem, 0)
	for _, in := range incidents {
		if in.ResolvedAt != "" {
			continue
		}
		state := "never-acknowledged"
		if in.AcknowledgedAt != "" {
			state = "acknowledged-unresolved"
		}
		it := triageItem{
			ID:             in.ID,
			Name:           in.Name,
			Monitor:        monitorNames[in.Source],
			State:          state,
			StartedAt:      in.StartedAt,
			AcknowledgedAt: in.AcknowledgedAt,
		}
		if it.Name == "" {
			it.Name = in.URL
		}
		if started, ok := parseTime(in.StartedAt); ok {
			age := now.Sub(started)
			if age < 0 {
				age = 0
			}
			it.AgeMinutes = int(age.Minutes())
			it.Age = age.Round(time.Minute).String()
		}
		items = append(items, it)
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].State != items[j].State {
			return items[i].State == "never-acknowledged"
		}
		return items[i].AgeMinutes > items[j].AgeMinutes
	})
	return items
}

// pp:data-source local
func newNovelTriageCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	var limit int

	cmd := &cobra.Command{
		Use:   "triage",
		Short: "Open incidents ranked by age and acknowledgement state — never-acknowledged first — joined to the affected monitor.",
		Long: "Use this command to age and prioritize open incidents by acknowledgement state. " +
			"Do NOT use it for the monitor-down board; use 'down' instead. " +
			"Do NOT use it for response-time averages; use 'mttr' instead. " +
			"Reads the local SQLite mirror; run `sync` first.",
		Example:     "  betterstack-cli triage\n  betterstack-cli triage --agent\n  betterstack-cli triage --limit 10",
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
			maybeEmitSyncHints(cmd, s, "incidents", flags.maxAge)

			incidents, err := loadIncidents(cmd.Context(), s)
			if err != nil {
				return fmt.Errorf("reading incidents: %w", err)
			}
			monitors, err := loadMonitors(cmd.Context(), s)
			if err != nil {
				return fmt.Errorf("reading monitors: %w", err)
			}
			names := make(map[string]string, len(monitors))
			for _, m := range monitors {
				n := m.Name
				if n == "" {
					n = m.URL
				}
				names[m.ID] = n
			}

			items := rankTriage(incidents, names, time.Now().UTC())
			if limit > 0 && len(items) > limit {
				items = items[:limit]
			}
			if flags.asJSON {
				return flags.printJSON(cmd, items)
			}
			if len(items) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No open incidents in the local mirror.")
				return nil
			}
			rows := make([][]string, 0, len(items))
			for _, it := range items {
				rows = append(rows, []string{it.ID, truncateField(it.Name, 36), truncateField(it.Monitor, 24), it.State, it.Age})
			}
			return flags.printTable(cmd, []string{"ID", "INCIDENT", "MONITOR", "STATE", "AGE"}, rows)
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/betterstack-cli/data.db)")
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum incidents to return (0 = all open incidents)")
	return cmd
}
