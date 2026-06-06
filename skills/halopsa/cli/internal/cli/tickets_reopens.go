// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"halopsa-pp-cli/internal/store"
)

type reopenRow struct {
	TicketID string `json:"ticket_id"`
	Summary  string `json:"summary"`
	Client   string `json:"client"`
	Agent    string `json:"agent"`
	Reopens  int    `json:"reopen_count"`
}

type reopenView struct {
	Since string      `json:"since"`
	Rows  []reopenRow `json:"rows"`
	Note  string      `json:"note,omitempty"`
}

// pp:data-source local
func newNovelTicketsReopensCmd(flags *rootFlags) *cobra.Command {
	var since string
	var dbPath string

	cmd := &cobra.Command{
		Use:   "reopens",
		Short: "Tickets that bounced from closed back to open (boomerangs), by agent and client",
		Long: `Surface boomerang tickets: tickets whose synced payload carries a non-zero
reopen marker within the window, grouped with their agent and client so
quality reviews can spot patterns.

Detection relies on the tenant's ticket payloads carrying a reopen counter
($.reopened); when the field is absent tenant-wide the command says so
honestly instead of reporting zero boomerangs as fact.

Reads the local sync store. Run 'halopsa-cli sync' first.`,
		Example: `  # Quality audit: boomerangs in the last 30 days
  halopsa-cli tickets reopens --since 30d --json`,
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would list reopened (boomerang) tickets from the local store")
				return nil
			}
			ctx := cmd.Context()
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

			// Field-presence probe: absence of the marker tenant-wide means
			// "cannot detect", not "zero boomerangs".
			var carrier int
			_ = db.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM tickets
				WHERE json_extract(data,'$.reopened') IS NOT NULL`).Scan(&carrier)
			view := reopenView{Since: t.Format(time.RFC3339), Rows: []reopenRow{}}
			if carrier == 0 {
				view.Note = "synced ticket payloads carry no $.reopened marker on this tenant; reopen detection unavailable (not zero boomerangs)"
				fmt.Fprintln(cmd.ErrOrStderr(), "hint: "+view.Note)
				if !wantsHumanTable(cmd.OutOrStdout(), flags) {
					return printJSONFiltered(cmd.OutOrStdout(), view, flags)
				}
				fmt.Fprintln(cmd.OutOrStdout(), view.Note)
				return nil
			}

			rows, err := db.DB().QueryContext(ctx, `SELECT id,
				COALESCE(NULLIF(json_extract(data,'$.summary'),''),'(no summary)'),
				COALESCE(NULLIF(client_name,''),'(no client)'),
				COALESCE(NULLIF(agent_name,''),'(unassigned)'),
				CAST(COALESCE(json_extract(data,'$.reopened'),0) AS INTEGER) AS reopens
			FROM tickets
			WHERE CAST(COALESCE(json_extract(data,'$.reopened'),0) AS INTEGER) > 0
			  AND datetime(COALESCE(NULLIF(json_extract(data,'$.lastactiondate'),''), datecreated)) >= datetime(?)
			ORDER BY reopens DESC, id DESC`, t.Format(time.RFC3339))
			if err != nil {
				return fmt.Errorf("reopens query: %w", err)
			}
			defer rows.Close()
			for rows.Next() {
				var r reopenRow
				if rows.Scan(&r.TicketID, &r.Summary, &r.Client, &r.Agent, &r.Reopens) != nil {
					continue
				}
				view.Rows = append(view.Rows, r)
			}
			if err := rows.Err(); err != nil {
				return fmt.Errorf("reopens rows: %w", err)
			}
			if len(view.Rows) == 0 {
				view.Note = "no reopened tickets in the window"
			}

			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), view, flags)
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Boomerang tickets since %s\n\n", view.Since)
			for _, r := range view.Rows {
				fmt.Fprintf(out, "#%-8s %2dx  %-24s %-20s %s\n", r.TicketID, r.Reopens, r.Client, r.Agent, r.Summary)
			}
			if view.Note != "" {
				fmt.Fprintln(out, "note: "+view.Note)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&since, "since", "30d", "Window: '30d', '7d', '24h', or RFC3339 timestamp")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	return cmd
}
