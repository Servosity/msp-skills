// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Novel command scaffold. Implement the RunE body before shipping.
// generate --force preserves implemented bodies; untouched TODO scaffolds may refresh.

package cli

import (
	"database/sql"
	"fmt"
	"sort"
	"time"

	"github.com/spf13/cobra"
)

func newNovelOverdueCmd(flags *rootFlags) *cobra.Command {
	var days int
	var hours int
	var dbPath string
	var limit int

	cmd := &cobra.Command{
		Use:         "overdue",
		Short:       "Find aging tickets still in new/open/pending past a threshold, weighted by priority so the worst rise to the top.",
		Example:     "  zammad-cli overdue --days 3 --limit 50 --json",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if days < 0 || hours < 0 {
				return usageErr(fmt.Errorf("--days and --hours must be non-negative"))
			}
			thresholdHours := days * 24
			if hours > 0 {
				thresholdHours = hours
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
			priorities, err := loadZammadPriorityCatalog(db)
			if err != nil {
				return err
			}
			agentNames, err := loadZammadAgentNames(db)
			if err != nil {
				return err
			}
			orgNames, _, err := loadZammadOrganizationNames(db)
			if err != nil {
				return err
			}

			type overdueRow struct {
				ID             string  `json:"id"`
				Number         string  `json:"number"`
				Title          string  `json:"title"`
				State          string  `json:"state"`
				Priority       string  `json:"priority"`
				Owner          string  `json:"owner"`
				Organization   string  `json:"organization"`
				AgeDays        int     `json:"age_days"`
				AgeHours       int     `json:"age_hours,omitempty"`
				PriorityWeight float64 `json:"priority_weight"`
				OverdueScore   float64 `json:"overdue_score"`
			}

			now := time.Now()
			rows, err := db.Query(`SELECT id, number, title, CAST(state_id AS TEXT), CAST(priority_id AS TEXT), CAST(owner_id AS TEXT), CAST(organization_id AS TEXT), created_at FROM tickets`)
			if err != nil {
				return fmt.Errorf("querying tickets: %w", err)
			}
			defer rows.Close()
			out := make([]overdueRow, 0)
			for rows.Next() {
				var id, number, title, stateID, priorityID, ownerID, orgID, createdAt sql.NullString
				if err := rows.Scan(&id, &number, &title, &stateID, &priorityID, &ownerID, &orgID, &createdAt); err != nil {
					return fmt.Errorf("scanning tickets: %w", err)
				}
				state := zammadStateForID(states, stateID)
				if !zammadStateActive(state.Kind) || !createdAt.Valid {
					continue
				}
				created, ok := parseZammadTime(createdAt.String)
				if !ok {
					continue
				}
				ageHours := zammadAgeHours(now, created)
				if ageHours < thresholdHours {
					continue
				}
				priority := zammadPriorityForID(priorities, priorityID)
				ageDays := zammadAgeDays(now, created)
				row := overdueRow{
					ID:             stringsOrDefault(id, ""),
					Number:         stringsOrDefault(number, ""),
					Title:          stringsOrDefault(title, ""),
					State:          state.Name,
					Priority:       priority.Name,
					Owner:          zammadAgentName(ownerID, agentNames),
					Organization:   zammadOrganizationName(orgID, orgNames),
					AgeDays:        ageDays,
					PriorityWeight: priority.Weight,
					OverdueScore:   zammadOverdueScore(ageHours, priority.Weight),
				}
				if hours > 0 {
					row.AgeHours = ageHours
				}
				out = append(out, row)
			}
			if err := rows.Err(); err != nil {
				return fmt.Errorf("reading tickets: %w", err)
			}
			sort.SliceStable(out, func(i, j int) bool {
				if out[i].OverdueScore != out[j].OverdueScore {
					return out[i].OverdueScore > out[j].OverdueScore
				}
				return out[i].ID < out[j].ID
			})
			if limit > 0 && len(out) > limit {
				out = out[:limit]
			}
			return printJSONFiltered(cmd.OutOrStdout(), out, flags)
		},
	}
	cmd.Flags().IntVar(&days, "days", 3, "Ticket age threshold in days")
	cmd.Flags().IntVar(&hours, "hours", 0, "Ticket age threshold in hours; overrides --days when set")
	cmd.Flags().StringVar(&dbPath, "db", "", "SQLite database file path (default: resolved data directory data.db)")
	cmd.Flags().IntVar(&limit, "limit", 50, "Maximum tickets to return")
	return cmd
}

func zammadOverdueScore(ageHours int, priorityWeight float64) float64 {
	return float64(ageHours) / 24 * priorityWeight
}

func stringsOrDefault(value sql.NullString, fallback string) string {
	if value.Valid {
		return value.String
	}
	return fallback
}
