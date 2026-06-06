// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"database/sql"
	"fmt"
	"sort"
	"time"

	"github.com/spf13/cobra"
	"halopsa-pp-cli/internal/store"
)

type timeLeakRow struct {
	Client      string  `json:"client"`
	Agent       string  `json:"agent"`
	UnbilledHrs float64 `json:"unbilled_billable_hours"`
	ActionCount int     `json:"action_count"`
}

type timeLeakView struct {
	From  string        `json:"from"`
	To    string        `json:"to"`
	Rows  []timeLeakRow `json:"rows"`
	Total float64       `json:"total_unbilled_hours"`
	Note  string        `json:"note,omitempty"`
}

// pp:data-source local
func newNovelTimeLeaksCmd(flags *rootFlags) *cobra.Command {
	var month string
	var client string
	var dbPath string

	cmd := &cobra.Command{
		Use:   "leaks",
		Short: "Billable time not yet attached to any invoice, by client and agent",
		Long: `Find revenue leaks: actions carrying billable hours (actionchargehours > 0)
whose invoice linkage fields (action_invoice_line_id / actioninvoicenumber)
are still empty, summed by client and agent for the window.

Use this command for billable time not yet on any invoice (revenue leak).
Do NOT use it for over-bank vs under-bank contract pacing; use
'contracts burn' instead.

Reads the local sync store. Run 'halopsa-cli sync' first.`,
		Example: `  # Monday billing prep: current month's un-invoiced billable time
  halopsa-cli time leaks --month current --json

  # One client
  halopsa-cli time leaks --client "Acme" --month 2026-05`,
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would compute un-invoiced billable hours from the local store")
				return nil
			}
			ctx := cmd.Context()
			start, end, err := parseMonth(month)
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
			if !hintIfUnsynced(cmd, db, "actions") {
				hintIfStale(cmd, db, "actions", flags.maxAge)
			}

			// Probe: do the synced action payloads carry invoice linkage at
			// all? If not, "un-invoiced" cannot be distinguished and the
			// command reports that honestly instead of claiming everything
			// leaks.
			var linked int
			_ = db.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM actions
				WHERE COALESCE(json_extract(data,'$.action_invoice_line_id'),0) != 0
				   OR COALESCE(json_extract(data,'$.actioninvoicenumber'),'') NOT IN ('','0')`).Scan(&linked)

			q := `SELECT
				COALESCE(NULLIF(t.client_name,''),'(no client)') AS client,
				COALESCE(NULLIF(json_extract(a.data,'$.who'),''),'(unassigned)') AS agent,
				SUM(COALESCE(json_extract(a.data,'$.actionchargehours'),0)) AS hrs,
				COUNT(*) AS n
			FROM actions a
			LEFT JOIN tickets t ON t.id = CAST(json_extract(a.data,'$.ticket_id') AS TEXT)
			WHERE COALESCE(json_extract(a.data,'$.actionchargehours'),0) > 0
			  AND COALESCE(json_extract(a.data,'$.action_invoice_line_id'),0) = 0
			  AND COALESCE(json_extract(a.data,'$.actioninvoicenumber'),'') IN ('','0')
			  AND datetime(COALESCE(NULLIF(json_extract(a.data,'$.actiondatecreated'),''), a.actiondatecreated)) BETWEEN datetime(?) AND datetime(?)`
			binds := []any{start.Format(time.RFC3339), end.Format(time.RFC3339)}
			if client != "" {
				q += ` AND t.client_name LIKE ?`
				binds = append(binds, "%"+client+"%")
			}
			q += ` GROUP BY client, agent ORDER BY hrs DESC`

			rows, err := db.DB().QueryContext(ctx, q, binds...)
			if err != nil {
				return fmt.Errorf("time-leaks query: %w", err)
			}
			defer rows.Close()
			view := timeLeakView{From: start.Format("2006-01-02"), To: end.Format("2006-01-02"), Rows: []timeLeakRow{}}
			for rows.Next() {
				var r timeLeakRow
				var hrs sql.NullFloat64
				if rows.Scan(&r.Client, &r.Agent, &hrs, &r.ActionCount) != nil {
					continue
				}
				r.UnbilledHrs = hrs.Float64
				view.Rows = append(view.Rows, r)
				view.Total += r.UnbilledHrs
			}
			if err := rows.Err(); err != nil {
				return fmt.Errorf("time-leaks rows: %w", err)
			}
			sort.SliceStable(view.Rows, func(i, j int) bool { return view.Rows[i].UnbilledHrs > view.Rows[j].UnbilledHrs })
			if linked == 0 && len(view.Rows) > 0 {
				view.Note = "no synced action carries an invoice linkage field on this tenant; rows show ALL billable hours in the window (un-invoiced detection unavailable)"
				fmt.Fprintln(cmd.ErrOrStderr(), "hint: "+view.Note)
			}
			if len(view.Rows) == 0 {
				view.Note = "no un-invoiced billable hours found in the window; widen --month or check sync freshness"
			}

			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), view, flags)
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Un-invoiced billable time %s .. %s (total %.2f h)\n\n", view.From, view.To, view.Total)
			for _, r := range view.Rows {
				fmt.Fprintf(out, "%-32s %-24s %8.2f h  (%d actions)\n", r.Client, r.Agent, r.UnbilledHrs, r.ActionCount)
			}
			if view.Note != "" {
				fmt.Fprintln(out, "\nnote: "+view.Note)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&month, "month", "current", "Month window: 'current', 'last', or YYYY-MM")
	cmd.Flags().StringVar(&client, "client", "", "Filter by client name substring")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	return cmd
}
