// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source local

package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"jira-service-management-pp-cli/internal/cliutil"
)

type slaReportView struct {
	Since    string     `json:"since"`
	GroupBy  string     `json:"group_by"`
	Requests int        `json:"requests_scanned"`
	Groups   []slaGroup `json:"groups"`
	Note     string     `json:"note,omitempty"`
	// SLAIncomplete counts synced requests that carry only part of their SLA.
	SLAIncomplete int `json:"sla_incomplete,omitempty"`
}

func newNovelSlaReportCmd(flags *rootFlags) *cobra.Command {
	var since, groupBy, dbPath string

	cmd := &cobra.Command{
		Use:   "sla-report",
		Short: "Breach rate, breached count and median time vs goal per desk, request type or SLA metric, from synced data.",
		Long: strings.Trim(`
Aggregates completed SLA cycles that stopped inside --since from requests in the
local store (sync stores each request with its SLA inline). Groups are sorted by
breach rate. Run 'sync --resources request,servicedesk,requesttype' first.

Use this command for historical SLA breach rates over a period from synced data.
Do NOT use this command for which open tickets are about to breach right now; use 'breaches' instead.`, "\n"),
		Example: strings.Trim(`
  jira-service-management-cli sla-report --since 30d --group-by requesttype --agent
  jira-service-management-cli sla-report --since 7d --group-by metric`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "sla-report")
			}
			window, err := cliutil.ParseDurationLoose(since)
			if err != nil || window <= 0 {
				return usageErr(fmt.Errorf("--since %q: want a duration like 7d, 30d or 24h", since))
			}
			switch groupBy {
			case "desk", "requesttype", "metric":
			default:
				return usageErr(fmt.Errorf("--group-by must be desk, requesttype or metric"))
			}
			view := slaReportView{Since: since, GroupBy: groupBy, Groups: make([]slaGroup, 0)}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			db, hint, err := openMirror(ctx, dbPath, "request,servicedesk,requesttype")
			if err != nil {
				return err
			}
			if db == nil {
				view.Note = hint
				fmt.Fprintln(cmd.ErrOrStderr(), view.Note)
				if !wantsHumanTable(cmd.OutOrStdout(), flags) {
					return printJSONFiltered(cmd.OutOrStdout(), view, flags)
				}
				return nil
			}
			defer db.Close()
			if !hintIfUnsynced(cmd, db, "request") {
				hintIfStale(cmd, db, "request", flags.maxAge)
			}
			reqs, err := localRequests(ctx, db)
			if err != nil {
				return err
			}
			desks, types := nameIndex(ctx, db)
			names := map[string]string{}
			switch groupBy {
			case "desk":
				names = desks
			case "requesttype":
				names = types
			}
			view.Requests = len(reqs)
			for _, r := range reqs {
				if r.slaIncomplete() {
					view.SLAIncomplete++
				}
			}
			if view.SLAIncomplete > 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "note: %d synced requests carry only part of their SLA metrics, so these numbers may undercount; re-run sync --resources request\n", view.SLAIncomplete)
			}
			view.Groups = slaRollup(reqs, time.Now().Add(-window).UnixMilli(), groupBy, names)
			if len(view.Groups) == 0 {
				view.Note = fmt.Sprintf("no completed SLA cycles in the last %s across %d synced requests", since, len(reqs))
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), view, flags)
			}
			w := cmd.OutOrStdout()
			if view.Note != "" {
				fmt.Fprintln(w, view.Note)
				return nil
			}
			fmt.Fprintf(w, "%-28s %7s %8s %7s %12s %12s\n", strings.ToUpper(groupBy), "CYCLES", "BREACHED", "RATE", "MEDIAN", "GOAL")
			for _, g := range view.Groups {
				label := g.Name
				if label == "" {
					label = g.Group
				}
				fmt.Fprintf(w, "%-28s %7d %8d %6.1f%% %12s %12s\n", truncateText(label, 28), g.Cycles, g.Breached, g.BreachRate*100,
					humanDuration(time.Duration(g.MedianElapsedMS)*time.Millisecond), humanDuration(time.Duration(g.MedianGoalMS)*time.Millisecond))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&since, "since", "30d", "Window of completed SLA cycles (e.g. 7d, 30d, 24h)")
	cmd.Flags().StringVar(&groupBy, "group-by", "desk", "Group by desk, requesttype or metric")
	cmd.Flags().StringVar(&dbPath, "db", "", "SQLite database file path (default: resolved data directory data.db)")
	return cmd
}
