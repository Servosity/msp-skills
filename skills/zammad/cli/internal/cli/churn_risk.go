// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Novel command scaffold. Implement the RunE body before shipping.
// generate --force preserves implemented bodies; untouched TODO scaffolds may refresh.

package cli

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"zammad-pp-cli/internal/store"
)

func newNovelChurnRiskCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	var limit int
	var days int
	var tierField string
	var highOnly bool
	var includeUnassigned bool

	cmd := &cobra.Command{
		Use:         "churn-risk",
		Short:       "Score each organization's churn risk from current workload pressure, overdue work, and negative customer sentiment",
		Example:     "  zammad-cli churn-risk --days 7 --high-only --json",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if days < 0 {
				return usageErr(fmt.Errorf("--days must be non-negative"))
			}
			if !customFieldNameRE.MatchString(tierField) {
				return usageErr(fmt.Errorf("--tier-field must be a simple JSON field name"))
			}
			db, err := openNovelStore(cmd.Context(), dbPath)
			if err != nil {
				return err
			}
			defer db.Close()
			maybeEmitSyncHints(cmd, db, "articles", flags.maxAge)
			empty, err := articlesTableEmpty(db)
			if err != nil {
				return err
			}
			if empty {
				fmt.Fprintln(cmd.ErrOrStderr(), "run 'zammad-cli articles sync' first")
				return printJSONFiltered(cmd.OutOrStdout(), struct {
					Note    string         `json:"note"`
					Results []churnRiskRow `json:"results"`
				}{Note: "run 'zammad-cli articles sync' first", Results: make([]churnRiskRow, 0)}, flags)
			}

			out, err := buildChurnRiskRows(db, days, tierField, highOnly, includeUnassigned, limit, time.Now())
			if err != nil {
				return err
			}
			return printJSONFiltered(cmd.OutOrStdout(), out, flags)
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "SQLite database file path (default: resolved data directory data.db)")
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum organizations to return (0 means all)")
	cmd.Flags().IntVar(&days, "days", 7, "Overdue threshold in days")
	cmd.Flags().StringVar(&tierField, "tier-field", "support_tier", "Custom JSON field to use for support tier")
	cmd.Flags().BoolVar(&highOnly, "high-only", false, "Show only organizations classified as high risk")
	cmd.Flags().BoolVar(&includeUnassigned, "include-unassigned", false, "Include tickets with no organization as a separate bucket")
	return cmd
}

type churnRiskRow struct {
	Organization    string   `json:"organization"`
	OrganizationID  string   `json:"organization_id"`
	Backlog         int      `json:"backlog"`
	Overdue         int      `json:"overdue"`
	NegativeTickets int      `json:"negative_tickets"`
	Pending         int      `json:"pending"`
	Score           int      `json:"score"`
	Risk            string   `json:"risk"`
	Factors         []string `json:"factors"`
	Tier            string   `json:"tier,omitempty"`
}

func buildChurnRiskRows(db *store.Store, days int, tierField string, highOnly, includeUnassigned bool, limit int, now time.Time) ([]churnRiskRow, error) {
	states, err := loadZammadStateCatalog(db)
	if err != nil {
		return nil, err
	}
	orgNames, orgData, err := loadZammadOrganizationNames(db)
	if err != nil {
		return nil, err
	}

	negativeTickets := make(map[string]bool)
	articleRows, err := db.Query(`SELECT CAST(ticket_id AS TEXT), subject, body FROM articles WHERE sender_id = 2 AND internal = 0`)
	if err != nil {
		return nil, fmt.Errorf("querying articles: %w", err)
	}
	defer articleRows.Close()
	for articleRows.Next() {
		var ticketID, subject, body sql.NullString
		if err := articleRows.Scan(&ticketID, &subject, &body); err != nil {
			return nil, fmt.Errorf("scanning articles: %w", err)
		}
		if !ticketID.Valid || strings.TrimSpace(ticketID.String) == "" {
			continue
		}
		if len(scanLexicon(subject.String+"\n"+body.String, escalationSignals)) > 0 {
			negativeTickets[strings.TrimSpace(ticketID.String)] = true
		}
	}
	if err := articleRows.Err(); err != nil {
		return nil, fmt.Errorf("reading articles: %w", err)
	}

	aggs := make(map[string]*churnRiskRow)
	ticketRows, err := db.Query(`SELECT id, CAST(organization_id AS TEXT), CAST(state_id AS TEXT), created_at, data FROM tickets`)
	if err != nil {
		return nil, fmt.Errorf("querying tickets: %w", err)
	}
	defer ticketRows.Close()
	for ticketRows.Next() {
		var id, orgID, stateID, createdAt, data sql.NullString
		if err := ticketRows.Scan(&id, &orgID, &stateID, &createdAt, &data); err != nil {
			return nil, fmt.Errorf("scanning tickets: %w", err)
		}
		key := zammadOrganizationID(orgID)
		row := aggs[key]
		if row == nil {
			row = &churnRiskRow{
				Organization:   zammadOrganizationName(orgID, orgNames),
				OrganizationID: key,
				Factors:        make([]string, 0),
			}
			if raw, ok := orgData[key]; ok {
				row.Tier = zammadTierFromData(raw, tierField)
			}
			aggs[key] = row
		}
		if row.Tier == "" && data.Valid {
			row.Tier = zammadTierFromData(data.String, tierField)
		}
		state := zammadStateForID(states, stateID)
		if !zammadStateActive(state.Kind) {
			continue
		}
		row.Backlog++
		if zammadStatePendingish(state.Kind) {
			row.Pending++
		}
		if id.Valid && negativeTickets[strings.TrimSpace(id.String)] {
			row.NegativeTickets++
		}
		if createdAt.Valid {
			if created, ok := parseZammadTime(createdAt.String); ok && zammadAgeDays(now, created) > days {
				row.Overdue++
			}
		}
	}
	if err := ticketRows.Err(); err != nil {
		return nil, fmt.Errorf("reading tickets: %w", err)
	}

	out := make([]churnRiskRow, 0, len(aggs))
	for _, row := range aggs {
		if !includeUnassigned && row.OrganizationID == "" {
			continue
		}
		row.Score = row.Backlog + row.Overdue*2 + row.NegativeTickets*3 + row.Pending
		row.Risk = churnRiskClassification(*row)
		row.Factors = churnRiskFactors(*row, days)
		if highOnly && row.Risk != "high" {
			continue
		}
		out = append(out, *row)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if churnRiskSeverity(out[i].Risk) != churnRiskSeverity(out[j].Risk) {
			return churnRiskSeverity(out[i].Risk) > churnRiskSeverity(out[j].Risk)
		}
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return out[i].Organization < out[j].Organization
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func churnRiskClassification(row churnRiskRow) string {
	switch {
	case row.Score >= 10 && (row.Overdue > 0 || row.NegativeTickets > 0):
		return "high"
	case row.Score >= 4:
		return "watch"
	default:
		return "low"
	}
}

func churnRiskFactors(row churnRiskRow, days int) []string {
	factors := make([]string, 0, 4)
	if row.Backlog > 0 {
		factors = append(factors, fmt.Sprintf("%d active tickets", row.Backlog))
	}
	if row.Overdue > 0 {
		factors = append(factors, fmt.Sprintf("%d tickets overdue >%dd", row.Overdue, days))
	}
	if row.NegativeTickets > 0 {
		factors = append(factors, fmt.Sprintf("%d tickets sound upset", row.NegativeTickets))
	}
	if row.Pending > 0 {
		factors = append(factors, fmt.Sprintf("%d pending tickets", row.Pending))
	}
	return factors
}

func churnRiskSeverity(risk string) int {
	switch risk {
	case "high":
		return 3
	case "watch":
		return 2
	default:
		return 1
	}
}
