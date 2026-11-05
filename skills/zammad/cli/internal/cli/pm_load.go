// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-carried: emitted by cli-printing-press 4.28.0 (project-management workflow template); the 4.32.5 profiler classes Zammad as the "content" archetype and no longer emits it. Kept so the shipped `load` surface survives the reprint (see handfixes.json: pm-workflow-commands-carried).
//
// Rewritten on Zammad's ticket model (Codex round 1 on the 4.34.0 reprint,
// 2026-10-24). The template counted every synced resource as a work item, so
// each user, group and state became "(unassigned)" work, and it read the
// owner as a raw id, so Zammad's unassigned owner (id 1) showed as agent "1".
// It now counts active tickets per owner, named from the synced users.

package cli

import (
	"database/sql"
	"fmt"
	"sort"

	"github.com/spf13/cobra"
	"zammad-pp-cli/internal/store"
)

func newLoadCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	var limit int

	cmd := &cobra.Command{
		Use:   "load",
		Short: "Show open-ticket workload per owner",
		Long: `Count the locally synced active tickets (new, open or pending) owned by
each agent. Helps identify overloaded team members and unbalanced workload.`,
		Example: `  # Show workload distribution
  zammad-cli load

  # Limit results
  zammad-cli load --limit 10

  # Output as JSON
  zammad-cli load --json`,
		// pp:data-source local
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			if dryRunOK(flags) {
				return nil
			}
			db, err := openNovelStore(cmd.Context(), dbPath)
			if err != nil {
				return err
			}
			defer db.Close()
			maybeEmitSyncHints(cmd, db, "tickets", flags.maxAge)
			if err := novelRequireSynced(db, "tickets", false); err != nil {
				return err
			}

			entries, total, err := buildTicketLoad(db)
			if err != nil {
				return err
			}
			shown := entries
			if limit > 0 && len(shown) > limit {
				shown = shown[:limit]
			}

			if wantsMachineOutput(flags) {
				if flags.csv || flags.plain || flags.quiet {
					return printJSONFiltered(cmd.OutOrStdout(), shown, flags)
				}
				result := map[string]any{
					"total_count":  total,
					"showing":      len(shown),
					"distribution": shown,
				}
				return printJSONFiltered(cmd.OutOrStdout(), result, flags)
			}

			if len(entries) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No active tickets found.")
				return nil
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Workload Distribution (%d active tickets, showing %d owners):\n\n", total, len(shown))
			fmt.Fprintf(cmd.OutOrStdout(), "%-40s %s\n", "ASSIGNEE", "COUNT")
			fmt.Fprintf(cmd.OutOrStdout(), "%-40s %s\n", "--------", "-----")
			for _, entry := range shown {
				fmt.Fprintf(cmd.OutOrStdout(), "%-40s %d\n", entry.Assignee, entry.Count)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&dbPath, "db", "", "SQLite database file path (default: resolved data directory data.db)")
	cmd.Flags().IntVar(&limit, "limit", 50, "Maximum entries to show (0 means all)")

	return cmd
}

type loadEntry struct {
	Assignee string `json:"assignee"`
	Count    int    `json:"count"`
}

// buildTicketLoad counts active tickets per owner (Zammad's owner id 1, or no
// owner, is "(unassigned)"), largest first, and returns the total.
func buildTicketLoad(db *store.Store) ([]loadEntry, int, error) {
	states, err := loadZammadStateCatalog(db)
	if err != nil {
		return nil, 0, err
	}
	names, err := loadZammadAgentNames(db)
	if err != nil {
		return nil, 0, err
	}
	rows, err := db.Query(`SELECT CAST(owner_id AS TEXT), CAST(state_id AS TEXT) FROM tickets`)
	if err != nil {
		return nil, 0, fmt.Errorf("querying tickets: %w", err)
	}
	defer rows.Close()
	type agg struct {
		name  string
		count int
	}
	byOwner := make(map[string]*agg)
	total := 0
	for rows.Next() {
		var ownerID, stateID sql.NullString
		if err := rows.Scan(&ownerID, &stateID); err != nil {
			return nil, 0, fmt.Errorf("scanning tickets: %w", err)
		}
		if !zammadStateActive(zammadStateForID(states, stateID).Kind) {
			continue
		}
		key := zammadOwnerID(ownerID)
		a := byOwner[key]
		if a == nil {
			a = &agg{name: zammadAgentName(ownerID, names)}
			byOwner[key] = a
		}
		a.count++
		total++
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("reading tickets: %w", err)
	}
	entries := make([]loadEntry, 0, len(byOwner))
	for _, a := range byOwner {
		entries = append(entries, loadEntry{Assignee: a.name, Count: a.count})
	}
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].Count != entries[j].Count {
			return entries[i].Count > entries[j].Count
		}
		return entries[i].Assignee < entries[j].Assignee
	})
	return entries, total, nil
}
