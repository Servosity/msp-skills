// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source local

package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"backup-radar-pp-cli/internal/radar"
)

type scorecardView struct {
	Query   string              `json:"query,omitempty"`
	Days    int                 `json:"days"`
	Clients []radar.ClientScore `json:"clients"`
	Sources radarSources        `json:"sources"`
}

func newNovelScorecardCmd(flags *rootFlags) *cobra.Command {
	var days, limit int
	var dbPath string

	cmd := &cobra.Command{
		Use:   "scorecard [client]",
		Short: "Score backup SLA compliance from synced client health: one client's rate, weekly trend and worst devices, or all clients ranked",
		Long: `Score backup SLA compliance from synced v3 client health history (each
sync fetches 365 days). The rate is in-compliance backup-days divided by all scored
backup-days; pending days are excluded. With a client (name, unique name
fragment, or ScalePad client id) it adds the weekly trend and the five worst
devices; without one it ranks every client, worst first. Worst devices come
from device SLA history: each sync fetches 90 days and earlier synced days
are kept (up to a year), so each device's "days" shows how much it covers.

Use this command for a compliance trend over weeks or months, for one client or ranked across clients.
Do NOT use this command for today's failing backups; use 'check' instead.`,
		Example: `  backup-radar-cli scorecard --days 90
  backup-radar-cli scorecard "Acme Dental" --days 90 --agent`,
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local", "mcp:write-flags": "csv,plain,quiet"}, // HAND-FIX(radar-machine-modes): refused modes leave the MCP schema
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := refuseTabularModes(flags, "scorecard"); err != nil {
				return err
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "scorecard")
			}
			if days < 1 || days > 365 {
				return usageErr(fmt.Errorf("--days must be between 1 and 365"))
			}
			if len(args) > 1 || (len(args) == 1 && strings.TrimSpace(args[0]) == "") {
				return usageErr(fmt.Errorf("scorecard takes at most one non-empty client; quote names with spaces"))
			}
			snap, ok, err := loadRadarSnapshot(cmd, flags, dbPath)
			if err != nil {
				return err
			}
			view := scorecardView{Days: days, Clients: make([]radar.ClientScore, 0), Sources: sourcesOf(snap, "health", "devices")}
			if len(args) == 1 {
				view.Query = args[0]
			}
			if ok {
				scores := radar.Scorecard(snap, days, len(args) == 1, time.Now().UTC())
				if len(args) == 1 {
					if len(scores) == 0 {
						// An empty mirror is an empty result, not a lookup failure.
						view.Sources.Note = "no client health synced; run 'backup-radar-cli sync --resources health'"
					} else {
						one, err := radar.FindClient(scores, args[0])
						if err != nil {
							return notFoundErr(err)
						}
						scores = []radar.ClientScore{one}
					}
				}
				view.Clients = scores
			}
			if limit > 0 && len(view.Clients) > limit {
				view.Clients = view.Clients[:limit]
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFilteredKeep(cmd.OutOrStdout(), view, flags, radarKeep(radar.ClientScore{})...)
			}
			if len(view.Clients) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No client health in the local store; run 'backup-radar-cli sync --resources health'.")
				printSourcesNote(cmd.OutOrStdout(), view.Sources.Note)
				return nil
			}
			printSourcesNote(cmd.ErrOrStderr(), view.Sources.Note)
			out := cmd.OutOrStdout()
			seen := map[string]int{}
			for _, c := range view.Clients {
				seen[strings.ToLower(c.Client)]++
			}
			for _, c := range view.Clients {
				name := c.Client
				if seen[strings.ToLower(c.Client)] > 1 && c.ClientID != "" {
					name += " (client id " + c.ClientID + ")" // same-named ScalePad clients
				}
				fmt.Fprintf(out, "%-40s %s over %d day(s)\n", scrubRadarText(name), ratePct(c.Rate), c.DaysCovered)
				for _, w := range c.Weekly {
					fmt.Fprintf(out, "    week of %s  %s\n", w.WeekStart, ratePct(w.Rate))
				}
				for _, d := range c.WorstDevices {
					fmt.Fprintf(out, "    worst: %s / %s  %d bad day(s) of %d\n", scrubRadarText(d.Device), scrubRadarText(d.Job), d.BadDays, d.Days)
				}
			}
			return nil
		},
	}
	cmd.Flags().IntVar(&days, "days", 90, "Window in calendar days ending on the newest synced day, the tenant's date at the last sync (1-365)")
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum clients to return when ranking (0 = all)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: the synced local store)")
	return cmd
}

func ratePct(r *float64) string {
	if r == nil {
		return "no scored days"
	}
	return fmt.Sprintf("%.1f%% in compliance", *r)
}
