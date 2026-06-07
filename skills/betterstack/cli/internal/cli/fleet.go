// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Novel feature (hand-built): one-screen fleet health from the local mirror.

package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

type fleetReport struct {
	Monitors struct {
		Total  int `json:"total"`
		Up     int `json:"up"`
		Down   int `json:"down"`
		Paused int `json:"paused"`
	} `json:"monitors"`
	Heartbeats struct {
		Total  int `json:"total"`
		Paused int `json:"paused"`
	} `json:"heartbeats"`
	Incidents struct {
		Total int `json:"total"`
		Open  int `json:"open"`
	} `json:"incidents"`
	OnCall struct {
		Calendars int `json:"calendars"`
		Covered   int `json:"calendars_with_someone_on_call"`
	} `json:"on_call"`
	Hint string `json:"hint,omitempty"`
}

// pp:data-source local
func newNovelFleetCmd(flags *rootFlags) *cobra.Command {
	var dbPath string

	cmd := &cobra.Command{
		Use:         "fleet",
		Short:       "One-screen health of the whole Better Stack account: monitors up/down/paused, heartbeats, open incidents",
		Long:        "Join monitors, heartbeats, incidents, and on-call calendars from the local SQLite mirror into a single health board. Run `sync` first to populate the mirror.",
		Example:     "  betterstack-cli fleet\n  betterstack-cli fleet --agent",
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
			ctx := cmd.Context()

			monitors, err := loadMonitors(ctx, s)
			if err != nil {
				return fmt.Errorf("reading monitors: %w", err)
			}
			heartbeats, err := loadHeartbeats(ctx, s)
			if err != nil {
				return fmt.Errorf("reading heartbeats: %w", err)
			}
			incidents, err := loadIncidents(ctx, s)
			if err != nil {
				return fmt.Errorf("reading incidents: %w", err)
			}
			onCalls, err := loadOnCalls(ctx, s)
			if err != nil {
				return fmt.Errorf("reading on-call calendars: %w", err)
			}

			var rep fleetReport
			rep.Monitors.Total = len(monitors)
			for _, m := range monitors {
				switch {
				case m.Paused:
					rep.Monitors.Paused++
				case monitorDown(m.Status):
					rep.Monitors.Down++
				default:
					rep.Monitors.Up++
				}
			}
			rep.Heartbeats.Total = len(heartbeats)
			for _, h := range heartbeats {
				if h.Paused {
					rep.Heartbeats.Paused++
				}
			}
			rep.Incidents.Total = len(incidents)
			for _, inc := range incidents {
				if inc.ResolvedAt == "" {
					rep.Incidents.Open++
				}
			}
			rep.OnCall.Calendars = len(onCalls)
			for _, oc := range onCalls {
				if oc.OnCallUsers > 0 {
					rep.OnCall.Covered++
				}
			}
			if rep.Monitors.Total == 0 && rep.Heartbeats.Total == 0 && rep.Incidents.Total == 0 && rep.OnCall.Calendars == 0 {
				rep.Hint = "local mirror is empty — run `betterstack-cli sync` first"
			}

			if flags.asJSON {
				return flags.printJSON(cmd, rep)
			}
			rows := [][]string{
				{"Monitors", fmt.Sprintf("%d total · %d up · %d down · %d paused", rep.Monitors.Total, rep.Monitors.Up, rep.Monitors.Down, rep.Monitors.Paused)},
				{"Heartbeats", fmt.Sprintf("%d total · %d paused", rep.Heartbeats.Total, rep.Heartbeats.Paused)},
				{"Incidents", fmt.Sprintf("%d total · %d open", rep.Incidents.Total, rep.Incidents.Open)},
				{"On-call", fmt.Sprintf("%d calendars · %d with someone on call", rep.OnCall.Calendars, rep.OnCall.Covered)},
			}
			if rep.Hint != "" {
				fmt.Fprintln(cmd.ErrOrStderr(), rep.Hint)
			}
			return flags.printTable(cmd, []string{"AREA", "STATUS"}, rows)
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/betterstack-cli/data.db)")
	return cmd
}
