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

func newNovelCustomerHealthCmd(flags *rootFlags) *cobra.Command {
	var atRiskOnly bool
	var dbPath string
	var includeUnassigned bool
	var limit int
	var tierField string

	cmd := &cobra.Command{
		Use:         "customer-health",
		Short:       "Rank organizations by a health signal built from backlog age, pending work, and last activity.",
		Example:     "  zammad-cli customer-health --at-risk --tier-field support_tier --json",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if !customFieldNameRE.MatchString(tierField) {
				return usageErr(fmt.Errorf("--tier-field must be a simple JSON field name"))
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
			orgNames, orgData, err := loadZammadOrganizationNames(db)
			if err != nil {
				return err
			}

			type customerHealthRow struct {
				Organization     string `json:"organization"`
				OrganizationID   string `json:"organization_id"`
				Open             int    `json:"open"`
				Pending          int    `json:"pending"`
				OldestOpenDays   int    `json:"oldest_open_days"`
				AvgOpenAgeDays   int    `json:"avg_open_age_days"`
				LastActivityDays int    `json:"last_activity_days"`
				Total            int    `json:"total"`
				Signal           string `json:"signal"`
				Tier             string `json:"tier,omitempty"`
			}
			type healthAgg struct {
				row              customerHealthRow
				openAgeDaysTotal int
				parsedOpenAges   int
				lastActivity     time.Time
			}

			now := time.Now()
			aggs := make(map[string]*healthAgg)
			rows, err := db.Query(`SELECT CAST(organization_id AS TEXT), CAST(state_id AS TEXT), created_at, updated_at, data FROM tickets`)
			if err != nil {
				return fmt.Errorf("querying tickets: %w", err)
			}
			defer rows.Close()
			for rows.Next() {
				var orgID, stateID, createdAt, updatedAt, data sql.NullString
				if err := rows.Scan(&orgID, &stateID, &createdAt, &updatedAt, &data); err != nil {
					return fmt.Errorf("scanning tickets: %w", err)
				}
				key := zammadOrganizationID(orgID)
				agg := aggs[key]
				if agg == nil {
					agg = &healthAgg{row: customerHealthRow{
						Organization:   zammadOrganizationName(orgID, orgNames),
						OrganizationID: key,
						Signal:         "healthy",
					}}
					if raw, ok := orgData[key]; ok {
						agg.row.Tier = zammadTierFromData(raw, tierField)
					}
					aggs[key] = agg
				}
				agg.row.Total++
				if agg.row.Tier == "" && data.Valid {
					agg.row.Tier = zammadTierFromData(data.String, tierField)
				}
				if updatedAt.Valid {
					if updated, ok := parseZammadTime(updatedAt.String); ok && updated.After(agg.lastActivity) {
						agg.lastActivity = updated
					}
				}
				state := zammadStateForID(states, stateID)
				if !zammadStateActive(state.Kind) {
					continue
				}
				agg.row.Open++
				if zammadStatePendingish(state.Kind) {
					agg.row.Pending++
				}
				if createdAt.Valid {
					if created, ok := parseZammadTime(createdAt.String); ok {
						age := zammadAgeDays(now, created)
						if age > agg.row.OldestOpenDays {
							agg.row.OldestOpenDays = age
						}
						agg.openAgeDaysTotal += age
						agg.parsedOpenAges++
					}
				}
			}
			if err := rows.Err(); err != nil {
				return fmt.Errorf("reading tickets: %w", err)
			}

			out := make([]customerHealthRow, 0, len(aggs))
			for _, agg := range aggs {
				if !includeUnassigned && agg.row.OrganizationID == "" {
					continue
				}
				if agg.parsedOpenAges > 0 {
					agg.row.AvgOpenAgeDays = averageZammadAgeDays(agg.openAgeDaysTotal, agg.parsedOpenAges)
				}
				if !agg.lastActivity.IsZero() {
					agg.row.LastActivityDays = zammadAgeDays(now, agg.lastActivity)
				}
				agg.row.Signal = customerHealthSignal(agg.row.Open, agg.row.OldestOpenDays, agg.row.LastActivityDays)
				if atRiskOnly && agg.row.Signal != "at-risk" {
					continue
				}
				out = append(out, agg.row)
			}
			sort.SliceStable(out, func(i, j int) bool {
				if customerHealthSeverity(out[i].Signal) != customerHealthSeverity(out[j].Signal) {
					return customerHealthSeverity(out[i].Signal) > customerHealthSeverity(out[j].Signal)
				}
				if out[i].Open != out[j].Open {
					return out[i].Open > out[j].Open
				}
				return out[i].Organization < out[j].Organization
			})
			if limit > 0 && len(out) > limit {
				out = out[:limit]
			}
			return printJSONFiltered(cmd.OutOrStdout(), out, flags)
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "SQLite database file path (default: resolved data directory data.db)")
	cmd.Flags().BoolVar(&includeUnassigned, "include-unassigned", false, "Include tickets with no organization as a separate bucket")
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum organizations to return (0 means all)")
	cmd.Flags().BoolVar(&atRiskOnly, "at-risk", false, "Show only organizations classified as at-risk")
	cmd.Flags().StringVar(&tierField, "tier-field", "support_tier", "Custom JSON field to use for support tier")
	return cmd
}

func averageZammadAgeDays(totalAgeDays, parsedAgeCount int) int {
	if parsedAgeCount <= 0 {
		return 0
	}
	return totalAgeDays / parsedAgeCount
}

func customerHealthSignal(open, oldestOpenDays, lastActivityDays int) string {
	if open >= 5 && (oldestOpenDays >= 14 || lastActivityDays >= 7) {
		return "at-risk"
	}
	if open >= 3 || oldestOpenDays >= 7 || lastActivityDays >= 5 {
		return "watch"
	}
	return "healthy"
}

func customerHealthSeverity(signal string) int {
	switch signal {
	case "at-risk":
		return 3
	case "watch":
		return 2
	default:
		return 1
	}
}
