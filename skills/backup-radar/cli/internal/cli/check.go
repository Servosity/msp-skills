// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source local

package cli

import (
	"fmt"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"backup-radar-pp-cli/internal/radar"
)

type checkView struct {
	Rows    []radar.CheckRow `json:"rows"`
	Counts  map[string]int   `json:"counts"`
	Sources radarSources     `json:"sources"`
}

func newNovelCheckCmd(flags *rootFlags) *cobra.Command {
	var includeAtRisk bool
	var client, dbPath string
	var limit int

	cmd := &cobra.Command{
		Use:   "check",
		Short: "Rank failing and silent backups (add --include-at-risk for at-risk) from the local store",
		Long: `Rank problem backups by joining the latest synced v3 SLA day (normally
yesterday) with current v2 backup status and ticket counts. Each row says whether the problem is new or
ongoing, how many days it has lasted, and how many tickets are open.

Use this command for today's fleet-wide triage of failing, silent or at-risk backups.
Do NOT use this command for backups that are not monitored or have no PSA ticketing; use 'unwatched' instead.
Do NOT use it for one device's history; use 'timeline' instead.`,
		Example: `  backup-radar-cli check
  backup-radar-cli check --include-at-risk --agent
  backup-radar-cli check --client "Acme Dental" --json`,
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local", "mcp:write-flags": "csv,plain,quiet"}, // HAND-FIX(radar-machine-modes): refused modes leave the MCP schema
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := refuseTabularModes(flags, "check"); err != nil {
				return err
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "check")
			}
			snap, ok, err := loadRadarSnapshot(cmd, flags, dbPath)
			if err != nil {
				return err
			}
			view := checkView{Rows: make([]radar.CheckRow, 0), Counts: map[string]int{}}
			view.Sources = sourcesOf(snap, "devices", "backups") // an empty snapshot says "nothing synced yet"
			if ok {
				view.Rows = radar.Check(snap, includeAtRisk, client, time.Now().UTC())
			}
			for _, r := range view.Rows {
				view.Counts[r.State]++
			}
			view.Counts["total"] = len(view.Rows)
			if limit > 0 && len(view.Rows) > limit {
				view.Rows = view.Rows[:limit]
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFilteredKeep(cmd.OutOrStdout(), view, flags, radarKeep(radar.CheckRow{})...)
			}
			if len(view.Rows) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No failing or silent backups in the local store.")
				if view.Sources.Note != "" {
					fmt.Fprintln(cmd.OutOrStdout(), "note: "+scrubRadarText(view.Sources.Note))
				}
				return nil
			}
			table := make([][]string, 0, len(view.Rows))
			for _, r := range view.Rows {
				tickets := "-"
				if r.TicketCount != nil {
					tickets = strconv.Itoa(*r.TicketCount)
				}
				table = append(table, []string{r.State, r.Trend, strconv.FormatFloat(r.DaysInState, 'f', -1, 64), r.Client, r.Device, r.Job, tickets, r.Source})
			}
			if view.Sources.Note != "" {
				fmt.Fprintln(cmd.ErrOrStderr(), "note: "+scrubRadarText(view.Sources.Note))
			}
			return printRadarTable(cmd.OutOrStdout(), []string{"state", "trend", "days", "client", "device", "job", "tickets", "source"}, table)
		},
	}
	cmd.Flags().BoolVar(&includeAtRisk, "include-at-risk", false, "Also include v3 At Risk and v2 Warning backups (default: only failing and no-result)")
	cmd.Flags().StringVar(&client, "client", "", "Only clients whose name contains this text (case-insensitive)")
	cmd.Flags().IntVar(&limit, "limit", 200, "Maximum rows to return after ranking (0 = all)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: the synced local store)")
	return cmd
}
