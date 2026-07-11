// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
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

var escalationSignals = []string{
	"cancel", "cancelling", "canceled", "refund", "unacceptable", "angry", "furious",
	"frustrated", "frustrating", "escalate", "escalation", "lawyer", "legal",
	"terrible", "worst", "disappointed", "ridiculous", "useless", "unhappy",
	"complaint", "urgent", "asap", "immediately", "disgusted", "no longer",
	"fed up", "switching", "competitor",
}

func newNovelEscalateCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	var limit int
	var minHits int

	cmd := &cobra.Command{
		Use:         "escalate",
		Short:       "Surface tickets whose inbound customer articles read as upset, weighted by age and priority",
		Example:     "  zammad-cli escalate --min-hits 1 --limit 25 --json",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if minHits < 0 {
				return usageErr(fmt.Errorf("--min-hits must be non-negative"))
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
					Note    string        `json:"note"`
					Results []escalateRow `json:"results"`
				}{Note: "run 'zammad-cli articles sync' first", Results: make([]escalateRow, 0)}, flags)
			}

			rows, err := buildEscalationRows(db, limit, minHits, time.Now())
			if err != nil {
				return err
			}
			return printJSONFiltered(cmd.OutOrStdout(), rows, flags)
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "SQLite database file path (default: resolved data directory data.db)")
	cmd.Flags().IntVar(&limit, "limit", 25, "Maximum tickets to return")
	cmd.Flags().IntVar(&minHits, "min-hits", 1, "Minimum negative lexicon hits required unless the score floor is met")
	return cmd
}

type escalateRow struct {
	ID           string   `json:"id"`
	Number       string   `json:"number"`
	Title        string   `json:"title"`
	State        string   `json:"state"`
	Priority     string   `json:"priority"`
	Owner        string   `json:"owner"`
	Organization string   `json:"organization"`
	AgeDays      int      `json:"age_days"`
	NegativeHits int      `json:"negative_hits"`
	Score        float64  `json:"score"`
	Snippets     []string `json:"snippets"`
}

func buildEscalationRows(db *store.Store, limit, minHits int, now time.Time) ([]escalateRow, error) {
	states, err := loadZammadStateCatalog(db)
	if err != nil {
		return nil, err
	}
	priorities, err := loadZammadPriorityCatalog(db)
	if err != nil {
		return nil, err
	}
	agentNames, err := loadZammadAgentNames(db)
	if err != nil {
		return nil, err
	}
	orgNames, _, err := loadZammadOrganizationNames(db)
	if err != nil {
		return nil, err
	}

	hitsByTicket := make(map[string]int)
	snippetsByTicket := make(map[string][]string)
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
		matches := scanLexicon(subject.String+"\n"+body.String, escalationSignals)
		if len(matches) == 0 {
			continue
		}
		id := strings.TrimSpace(ticketID.String)
		hitsByTicket[id] += len(matches)
		for _, match := range matches {
			snippetsByTicket[id] = append(snippetsByTicket[id], match.Snippet)
		}
	}
	if err := articleRows.Err(); err != nil {
		return nil, fmt.Errorf("reading articles: %w", err)
	}

	ticketRows, err := db.Query(`SELECT id, number, title, CAST(state_id AS TEXT), CAST(priority_id AS TEXT), CAST(owner_id AS TEXT), CAST(organization_id AS TEXT), created_at FROM tickets`)
	if err != nil {
		return nil, fmt.Errorf("querying tickets: %w", err)
	}
	defer ticketRows.Close()

	out := make([]escalateRow, 0)
	for ticketRows.Next() {
		var id, number, title, stateID, priorityID, ownerID, orgID, createdAt sql.NullString
		if err := ticketRows.Scan(&id, &number, &title, &stateID, &priorityID, &ownerID, &orgID, &createdAt); err != nil {
			return nil, fmt.Errorf("scanning tickets: %w", err)
		}
		if !id.Valid || strings.TrimSpace(id.String) == "" {
			continue
		}
		state := zammadStateForID(states, stateID)
		if !zammadStateActive(state.Kind) {
			continue
		}
		ageDays := 0
		if createdAt.Valid {
			if created, ok := parseZammadTime(createdAt.String); ok {
				ageDays = zammadAgeDays(now, created)
			}
		}
		priority := zammadPriorityForID(priorities, priorityID)
		negativeHits := hitsByTicket[id.String]
		ageBucket := ageDays / 7
		if ageBucket > 4 {
			ageBucket = 4
		}
		score := float64(negativeHits*2+ageBucket) + priority.Weight
		if negativeHits < minHits && score < 5 {
			continue
		}
		snippets := uniqueStrings(snippetsByTicket[id.String])
		if len(snippets) > 5 {
			snippets = snippets[:5]
		}
		out = append(out, escalateRow{
			ID:           id.String,
			Number:       stringsOrDefault(number, ""),
			Title:        stringsOrDefault(title, ""),
			State:        state.Name,
			Priority:     priority.Name,
			Owner:        zammadAgentName(ownerID, agentNames),
			Organization: zammadOrganizationName(orgID, orgNames),
			AgeDays:      ageDays,
			NegativeHits: negativeHits,
			Score:        score,
			Snippets:     snippets,
		})
	}
	if err := ticketRows.Err(); err != nil {
		return nil, fmt.Errorf("reading tickets: %w", err)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return out[i].ID < out[j].ID
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}
