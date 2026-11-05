// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-carried: emitted by cli-printing-press 4.28.0 (project-management workflow template); the 4.32.5 profiler classes Zammad as the "content" archetype and no longer emits it. Kept so the shipped `orphans` surface survives the reprint (see handfixes.json: pm-workflow-commands-carried).
//
// Rewritten on Zammad's ticket model (Codex round 1 on the 4.34.0 reprint,
// 2026-10-24). The template demanded a "project" and "labels" field that a
// Zammad ticket never carries, so every ticket (and every user, group and
// state) was reported orphaned. It now reports active tickets with no owner
// (Zammad's owner id 1 is "unassigned"), no priority or no group.

package cli

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"zammad-pp-cli/internal/store"
)

func newOrphansCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	var limit int

	cmd := &cobra.Command{
		Use:   "orphans",
		Short: "Find open tickets missing an owner, priority or group",
		Long: `Scan the locally synced active tickets (new, open or pending) for ones
with no owner (unassigned), no priority or no group. Useful for triaging
unowned work.`,
		Example: `  # Find orphaned tickets
  zammad-cli orphans

  # Output as JSON
  zammad-cli orphans --json`,
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

			items, err := buildOrphanTickets(db)
			if err != nil {
				return err
			}
			totalCount := len(items)
			if limit > 0 && len(items) > limit {
				items = items[:limit]
			}

			if wantsMachineOutput(flags) {
				if flags.csv || flags.plain || flags.quiet {
					return printJSONFiltered(cmd.OutOrStdout(), items, flags)
				}
				result := map[string]any{
					"total_count": totalCount,
					"showing":     len(items),
					"items":       items,
				}
				return printJSONFiltered(cmd.OutOrStdout(), result, flags)
			}

			if len(items) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No orphaned tickets found. Every active ticket has an owner, priority and group.")
				return nil
			}

			if totalCount > len(items) {
				fmt.Fprintf(cmd.OutOrStdout(), "Found %d orphaned tickets (showing first %d):\n\n", totalCount, len(items))
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "Found %d orphaned tickets:\n\n", len(items))
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%-20s %-15s %-30s %s\n", "ID", "TYPE", "MISSING", "TITLE")
			fmt.Fprintf(cmd.OutOrStdout(), "%-20s %-15s %-30s %s\n", "----", "----", "-------", "-----")
			for _, item := range items {
				titleDisplay := item.Title
				if len(titleDisplay) > 40 {
					titleDisplay = titleDisplay[:37] + "..."
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%-20s %-15s %-30s %s\n", item.ID, item.ResourceType, strings.Join(item.MissingFields, ", "), titleDisplay)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&dbPath, "db", "", "SQLite database file path (default: resolved data directory data.db)")
	cmd.Flags().IntVar(&limit, "limit", 50, "Maximum tickets to show (0 means all)")

	return cmd
}

type orphanItem struct {
	ID            string   `json:"id"`
	ResourceType  string   `json:"resource_type"`
	Title         string   `json:"title,omitempty"`
	MissingFields []string `json:"missing_fields"`
}

// buildOrphanTickets returns the active tickets missing an owner ("assignee";
// Zammad's owner id 1 means unassigned), a priority or a group, by ticket id.
func buildOrphanTickets(db *store.Store) ([]orphanItem, error) {
	states, err := loadZammadStateCatalog(db)
	if err != nil {
		return nil, err
	}
	rows, err := db.Query(`SELECT id, title, CAST(state_id AS TEXT), CAST(owner_id AS TEXT), CAST(priority_id AS TEXT), CAST(group_id AS TEXT) FROM tickets`)
	if err != nil {
		return nil, fmt.Errorf("querying tickets: %w", err)
	}
	defer rows.Close()
	items := make([]orphanItem, 0)
	for rows.Next() {
		var id, title, stateID, ownerID, priorityID, groupID sql.NullString
		if err := rows.Scan(&id, &title, &stateID, &ownerID, &priorityID, &groupID); err != nil {
			return nil, fmt.Errorf("scanning tickets: %w", err)
		}
		if !id.Valid || strings.TrimSpace(id.String) == "" {
			continue
		}
		if !zammadStateActive(zammadStateForID(states, stateID).Kind) {
			continue
		}
		missing := make([]string, 0, 3)
		if zammadOwnerID(ownerID) == "1" {
			missing = append(missing, "assignee")
		}
		if strings.TrimSpace(priorityID.String) == "" {
			missing = append(missing, "priority")
		}
		if strings.TrimSpace(groupID.String) == "" {
			missing = append(missing, "group")
		}
		if len(missing) == 0 {
			continue
		}
		items = append(items, orphanItem{ID: id.String, ResourceType: "tickets", Title: title.String, MissingFields: missing})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading tickets: %w", err)
	}
	sort.SliceStable(items, func(i, j int) bool { return zammadNumericLess(items[i].ID, items[j].ID) })
	return items, nil
}
