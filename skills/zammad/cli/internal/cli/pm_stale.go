// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-carried: emitted by cli-printing-press 4.28.0 (project-management workflow template); the 4.32.5 profiler classes Zammad as the "content" archetype and no longer emits it. Kept so the shipped `stale` surface survives the reprint (see handfixes.json: pm-workflow-commands-carried).
//
// Rewritten on Zammad's ticket model (Codex round 1 on the 4.34.0 reprint,
// 2026-10-24). The template scanned every synced resource (users, groups,
// states, closed tickets) and compared RFC 3339 timestamps as TEXT, so a value
// written in another UTC offset than the cutoff was misjudged by up to a day.
// It now reads active tickets only and compares instants.

package cli

import (
	"database/sql"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"zammad-pp-cli/internal/store"
)

func newStaleCmd(flags *rootFlags) *cobra.Command {
	var days int
	var team string
	var dbPath string
	var limit int

	cmd := &cobra.Command{
		Use:   "stale",
		Short: "Find open tickets with no updates in N days",
		Long: `Scan the locally synced tickets for active ones (new, open or pending)
that have not been updated within the given number of days. Useful for
finding forgotten or blocked work. --team matches a group id or name.`,
		Example: `  # Find tickets not updated in 30 days (default)
  zammad-cli stale

  # Find tickets not updated in 14 days
  zammad-cli stale --days 14

  # Filter by team (Zammad group id or name)
  zammad-cli stale --days 7 --team "2nd Level"

  # Output as JSON
  zammad-cli stale --days 30 --json`,
		// pp:data-source local
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			if days < 0 {
				return usageErr(fmt.Errorf("--days must be non-negative"))
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

			items, err := buildStaleTickets(db, days, team, time.Now())
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
				fmt.Fprintln(cmd.OutOrStdout(), "No stale tickets found.")
				return nil
			}

			if totalCount > len(items) {
				fmt.Fprintf(cmd.OutOrStdout(), "Found %d stale tickets (showing first %d, no update in %d+ days):\n\n", totalCount, len(items), days)
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "Found %d stale tickets (no update in %d+ days):\n\n", len(items), days)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%-20s %-15s %-8s %s\n", "ID", "TYPE", "DAYS", "TITLE")
			fmt.Fprintf(cmd.OutOrStdout(), "%-20s %-15s %-8s %s\n", "----", "----", "----", "-----")
			for _, item := range items {
				titleDisplay := item.Title
				if len(titleDisplay) > 50 {
					titleDisplay = titleDisplay[:47] + "..."
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%-20s %-15s %-8d %s\n", item.ID, item.ResourceType, item.DaysSince, titleDisplay)
			}
			return nil
		},
	}

	cmd.Flags().IntVar(&days, "days", 30, "Number of days without update to consider stale")
	cmd.Flags().StringVar(&team, "team", "", "Filter by team: a Zammad group id or group name")
	cmd.Flags().StringVar(&dbPath, "db", "", "SQLite database file path (default: resolved data directory data.db)")
	cmd.Flags().IntVar(&limit, "limit", 50, "Maximum tickets to show (0 means all)")

	return cmd
}

type staleItem struct {
	ID           string `json:"id"`
	ResourceType string `json:"resource_type"`
	Title        string `json:"title,omitempty"`
	UpdatedAt    string `json:"updated_at,omitempty"`
	DaysSince    int    `json:"days_since"`
	updated      time.Time
}

// buildStaleTickets returns the active tickets whose last update (or creation,
// when no update time was synced) is more than days before now, oldest first.
// Timestamps are parsed and compared as instants, never as text.
func buildStaleTickets(db *store.Store, days int, team string, now time.Time) ([]staleItem, error) {
	states, err := loadZammadStateCatalog(db)
	if err != nil {
		return nil, err
	}
	groups, err := loadZammadGroupNames(db)
	if err != nil {
		return nil, err
	}
	team = strings.TrimSpace(team)
	rows, err := db.Query(`SELECT id, title, CAST(state_id AS TEXT), CAST(group_id AS TEXT), created_at, updated_at FROM tickets`)
	if err != nil {
		return nil, fmt.Errorf("querying tickets: %w", err)
	}
	defer rows.Close()
	threshold := time.Duration(days) * 24 * time.Hour
	items := make([]staleItem, 0)
	for rows.Next() {
		var id, title, stateID, groupID, createdAt, updatedAt sql.NullString
		if err := rows.Scan(&id, &title, &stateID, &groupID, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("scanning tickets: %w", err)
		}
		if !id.Valid || strings.TrimSpace(id.String) == "" {
			continue
		}
		if !zammadStateActive(zammadStateForID(states, stateID).Kind) {
			continue
		}
		if team != "" && !zammadGroupMatches(groupID, groups, team) {
			continue
		}
		raw := updatedAt.String
		updated, ok := parseZammadTime(raw)
		if !ok {
			raw = createdAt.String
			if updated, ok = parseZammadTime(raw); !ok {
				continue
			}
		}
		if !zammadOlderThan(now, updated, threshold) {
			continue
		}
		items = append(items, staleItem{
			ID:           id.String,
			ResourceType: "tickets",
			Title:        title.String,
			UpdatedAt:    raw,
			DaysSince:    zammadAgeDays(now, updated),
			updated:      updated,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading tickets: %w", err)
	}
	sort.SliceStable(items, func(i, j int) bool {
		if !items[i].updated.Equal(items[j].updated) {
			return items[i].updated.Before(items[j].updated)
		}
		return zammadNumericLess(items[i].ID, items[j].ID)
	})
	return items, nil
}

// loadZammadGroupNames maps group id to name from the synced groups.
func loadZammadGroupNames(db *store.Store) (map[string]string, error) {
	rows, err := db.Query(`SELECT id, name FROM groups`)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return map[string]string{}, nil
		}
		return nil, fmt.Errorf("querying groups: %w", err)
	}
	defer rows.Close()
	names := make(map[string]string)
	for rows.Next() {
		var id, name sql.NullString
		if err := rows.Scan(&id, &name); err != nil {
			return nil, fmt.Errorf("scanning groups: %w", err)
		}
		if id.Valid {
			names[strings.TrimSpace(id.String)] = strings.TrimSpace(name.String)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading groups: %w", err)
	}
	return names, nil
}

// zammadGroupMatches reports whether a ticket's group is team, given as a
// group id or a group name (case-insensitive).
func zammadGroupMatches(groupID sql.NullString, groups map[string]string, team string) bool {
	id := strings.TrimSpace(groupID.String)
	if !groupID.Valid || id == "" {
		return false
	}
	if id == team {
		return true
	}
	return strings.EqualFold(groups[id], team)
}

// zammadNumericLess orders ids numerically when both are numbers.
func zammadNumericLess(left, right string) bool {
	l, lErr := strconv.ParseInt(left, 10, 64)
	r, rErr := strconv.ParseInt(right, 10, 64)
	if lErr == nil && rErr == nil {
		return l < r
	}
	return left < right
}
