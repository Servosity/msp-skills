// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"halopsa-pp-cli/internal/store"
)

type slaScoreRow struct {
	Group      string  `json:"group"`
	Closed     int     `json:"closed_tickets"`
	WithTarget int     `json:"with_sla_target"`
	MetTarget  int     `json:"met_resolution_sla"`
	MetPct     float64 `json:"met_pct"`
}

type slaScoreView struct {
	Since    string        `json:"since"`
	By       string        `json:"by"`
	Rows     []slaScoreRow `json:"rows"`
	Excluded int           `json:"closed_without_sla_target"`
	Note     string        `json:"note,omitempty"`
}

// pp:data-source local
func newNovelSlaScorecardCmd(flags *rootFlags) *cobra.Command {
	var since string
	var by string
	var dbPath string

	cmd := &cobra.Command{
		Use:   "scorecard",
		Short: "Historical SLA pass-rate for closed tickets, by team or agent",
		Long: `Compute the resolution-SLA hit rate over already-closed tickets: of the
tickets closed in the window that carried an SLA target date, what share
was closed on or before the target. Grouped by team or agent.

Close time uses the last-action timestamp (the same proxy 'standup' uses);
tickets without an SLA target are counted separately, never silently
dropped. Response-SLA scoring is not included: the synced payloads carry
no unambiguous first-response deadline field.

Use this command for historical SLA pass-rate on already-closed tickets.
Do NOT use it for live at-risk tickets; use 'sla breaching' instead.

Reads the local sync store. Run 'halopsa-cli sync' first.`,
		Example: `  # Last 30 days by team — the leadership number
  halopsa-cli sla scorecard --since 30d --by team

  # By agent, JSON
  halopsa-cli sla scorecard --since 7d --by agent --json`,
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would compute the SLA hit-rate scorecard from the local store")
				return nil
			}
			ctx := cmd.Context()
			switch by {
			case "team", "agent":
			default:
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("invalid --by %q (want team|agent)", by))
			}
			t, err := parseSince(since)
			if err != nil {
				_ = cmd.Usage()
				return usageErr(err)
			}
			if dbPath == "" {
				dbPath = defaultDBPath("halopsa-cli")
			}
			db, err := store.OpenWithContext(ctx, dbPath)
			if err != nil {
				return fmt.Errorf("opening local database: %w", err)
			}
			defer func() { _ = db.Close() }()
			if !hintIfUnsynced(cmd, db, "tickets") {
				hintIfStale(cmd, db, "tickets", flags.maxAge)
			}

			groupExpr := "COALESCE(NULLIF(agent_name,''),'(unassigned)')"
			if by == "team" {
				groupExpr = "COALESCE(NULLIF(team,''), NULLIF(json_extract(data,'$.team'),''), '(no team)')"
			}
			q := `SELECT ` + groupExpr + ` AS grp,
				COUNT(*) AS closed,
				SUM(CASE WHEN COALESCE(json_extract(data,'$.targetdate'),'') != '' THEN 1 ELSE 0 END) AS with_target,
				SUM(CASE WHEN COALESCE(json_extract(data,'$.targetdate'),'') != ''
				          AND datetime(COALESCE(NULLIF(json_extract(data,'$.lastactiondate'),''), datecreated)) <= datetime(json_extract(data,'$.targetdate'))
				     THEN 1 ELSE 0 END) AS met
			FROM tickets
			WHERE json_extract(data,'$.status_id') IN (8,9)
			  AND datetime(COALESCE(NULLIF(json_extract(data,'$.lastactiondate'),''), datecreated)) >= datetime(?)
			GROUP BY grp
			ORDER BY closed DESC`
			rows, err := db.DB().QueryContext(ctx, q, t.Format(time.RFC3339))
			if err != nil {
				return fmt.Errorf("sla scorecard query: %w", err)
			}
			defer rows.Close()
			view := slaScoreView{Since: t.Format(time.RFC3339), By: by, Rows: []slaScoreRow{}}
			for rows.Next() {
				var r slaScoreRow
				var withTarget, met sql.NullInt64
				if rows.Scan(&r.Group, &r.Closed, &withTarget, &met) != nil {
					continue
				}
				r.WithTarget = int(withTarget.Int64)
				r.MetTarget = int(met.Int64)
				if r.WithTarget > 0 {
					r.MetPct = float64(r.MetTarget) / float64(r.WithTarget) * 100
				}
				view.Excluded += r.Closed - r.WithTarget
				view.Rows = append(view.Rows, r)
			}
			if err := rows.Err(); err != nil {
				return fmt.Errorf("sla scorecard rows: %w", err)
			}
			if len(view.Rows) == 0 {
				view.Note = "no closed tickets in the window; widen --since or run sync"
			}

			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), view, flags)
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "SLA resolution hit-rate since %s (by %s)\n\n", view.Since, view.By)
			fmt.Fprintf(out, "%-30s %7s %12s %6s\n", view.By, "closed", "with-target", "met%%")
			for _, r := range view.Rows {
				fmt.Fprintf(out, "%-30s %7d %12d %5.1f%%\n", r.Group, r.Closed, r.WithTarget, r.MetPct)
			}
			if view.Excluded > 0 {
				fmt.Fprintf(out, "\n%d closed ticket(s) carried no SLA target and are excluded from met%%.\n", view.Excluded)
			}
			if view.Note != "" {
				fmt.Fprintln(out, "note: "+view.Note)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&since, "since", "30d", "Window: '30d', '7d', '24h', or RFC3339 timestamp")
	cmd.Flags().StringVar(&by, "by", "team", "Group by: team or agent")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	return cmd
}
