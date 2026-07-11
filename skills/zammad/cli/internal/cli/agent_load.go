// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Novel command scaffold. Implement the RunE body before shipping.
// generate --force preserves implemented bodies; untouched TODO scaffolds may refresh.

package cli

import (
	"database/sql"
	"fmt"
	"sort"

	"github.com/spf13/cobra"
)

func newNovelAgentLoadCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	var limit int

	cmd := &cobra.Command{
		Use:         "agent-load",
		Short:       "See each agent's current ticket load broken out by state (open/pending/backlog)",
		Example:     "  zammad-cli agent-load --limit 10 --json",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			db, err := openNovelStore(cmd.Context(), dbPath)
			if err != nil {
				return err
			}
			defer db.Close()
			maybeEmitSyncHints(cmd, db, "tickets", flags.maxAge)

			states, err := loadZammadStateCatalog(db)
			if err != nil {
				return err
			}
			agentNames, err := loadZammadAgentNames(db)
			if err != nil {
				return err
			}

			type agentLoadRow struct {
				Agent    string `json:"agent"`
				OwnerID  string `json:"owner_id"`
				Open     int    `json:"open"`
				Pending  int    `json:"pending"`
				Resolved int    `json:"resolved"`
				Closed   int    `json:"closed"`
				Merged   int    `json:"merged"`
				Total    int    `json:"total"`
				Backlog  int    `json:"backlog"`
			}

			rows, err := db.Query(`SELECT CAST(owner_id AS TEXT), CAST(state_id AS TEXT) FROM tickets`)
			if err != nil {
				return fmt.Errorf("querying tickets: %w", err)
			}
			defer rows.Close()

			byOwner := make(map[string]*agentLoadRow)
			for rows.Next() {
				var ownerID, stateID sql.NullString
				if err := rows.Scan(&ownerID, &stateID); err != nil {
					return fmt.Errorf("scanning tickets: %w", err)
				}
				key := zammadOwnerID(ownerID)
				row := byOwner[key]
				if row == nil {
					row = &agentLoadRow{
						Agent:   zammadAgentName(ownerID, agentNames),
						OwnerID: key,
					}
					byOwner[key] = row
				}
				state := zammadStateForID(states, stateID)
				switch state.Kind {
				case zammadStateClosed:
					row.Closed++
				case zammadStateMerged:
					row.Merged++
				case zammadStateResolved:
					row.Resolved++
				case zammadStatePending:
					row.Pending++
				default:
					row.Open++
				}
				row.Total++
			}
			if err := rows.Err(); err != nil {
				return fmt.Errorf("reading tickets: %w", err)
			}

			out := make([]agentLoadRow, 0, len(byOwner))
			for _, row := range byOwner {
				row.Backlog = row.Open + row.Pending + row.Resolved
				out = append(out, *row)
			}
			sort.SliceStable(out, func(i, j int) bool {
				if out[i].Backlog != out[j].Backlog {
					return out[i].Backlog > out[j].Backlog
				}
				return out[i].Agent < out[j].Agent
			})
			if limit > 0 && len(out) > limit {
				out = out[:limit]
			}
			return printJSONFiltered(cmd.OutOrStdout(), out, flags)
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "SQLite database file path (default: resolved data directory data.db)")
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum agents to return (0 means all)")
	return cmd
}
