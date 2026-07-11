// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Novel command scaffold. Implemented by hand for Zammad feedback mining.

package cli

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"zammad-pp-cli/internal/store"
)

var feedbackBucketLexicons = map[string][]string{
	"feature": {
		"feature request", "would be nice", "can you add", "please add", "wish",
		"missing", "support for", "ability to", "enhancement", "feature",
	},
	"pricing": {
		"price", "pricing", "cost", "expensive", "invoice", "billing", "too much",
		"quote", "discount", "renewal", "per user", "budget",
	},
	"compliance": {
		"compliance", "gdpr", "hipaa", "soc 2", "soc2", "audit", "data retention",
		"privacy", "dpa", "security", "regulation", "iso 27001", "pci",
	},
	"bug": {
		"bug", "broken", "error", "crash", "not working", "fails", "failing",
		"defect", "glitch", "does not work", "regression",
	},
}

var feedbackBucketOrder = []string{"feature", "pricing", "compliance", "bug"}

func newNovelFeedbackScanCmd(flags *rootFlags) *cobra.Command {
	var bucket string
	var dbPath string
	var limit int

	cmd := &cobra.Command{
		Use:   "feedback-scan",
		Short: "Bucket ticket and article text into feature, pricing, compliance, and bug themes",
		Example: `  zammad-cli feedback-scan --bucket pricing --json
  zammad-cli feedback-scan --limit 100 --json`,
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			bucket = strings.ToLower(strings.TrimSpace(bucket))
			if bucket != "" && feedbackBucketLexicons[bucket] == nil {
				return usageErr(fmt.Errorf("--bucket must be one of: feature, pricing, compliance, bug"))
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
					Note    string              `json:"note"`
					Results []feedbackScanMatch `json:"results"`
				}{Note: "run 'zammad-cli articles sync' first", Results: make([]feedbackScanMatch, 0)}, flags)
			}

			out, err := buildFeedbackScanOutput(db, bucket, limit)
			if err != nil {
				return err
			}
			return printJSONFiltered(cmd.OutOrStdout(), out, flags)
		},
	}
	cmd.Flags().StringVar(&bucket, "bucket", "", "Filter to one bucket: feature, pricing, compliance, or bug")
	cmd.Flags().StringVar(&dbPath, "db", "", "SQLite database file path (default: resolved data directory data.db)")
	cmd.Flags().IntVar(&limit, "limit", 50, "Maximum matches to return across all buckets (0 means all)")
	return cmd
}

type feedbackScanMatch struct {
	Bucket       string `json:"bucket"`
	TicketID     string `json:"ticket_id"`
	TicketNumber string `json:"ticket_number"`
	Title        string `json:"title"`
	Snippet      string `json:"snippet"`
	MatchedTerm  string `json:"matched_term"`
}

type feedbackScanOutput struct {
	Buckets map[string][]feedbackScanMatch `json:"buckets"`
	Counts  map[string]int                 `json:"counts"`
}

func buildFeedbackScanOutput(db *store.Store, bucket string, limit int) (feedbackScanOutput, error) {
	out := feedbackScanOutput{
		Buckets: make(map[string][]feedbackScanMatch, len(feedbackBucketOrder)),
		Counts:  make(map[string]int, len(feedbackBucketOrder)),
	}
	for _, name := range feedbackBucketOrder {
		out.Buckets[name] = make([]feedbackScanMatch, 0)
		out.Counts[name] = 0
	}

	type ticketRef struct {
		Number string
		Title  string
	}
	tickets := make(map[string]ticketRef)
	ticketRows, err := db.Query(`SELECT id, number, title FROM tickets`)
	if err != nil {
		return out, fmt.Errorf("querying tickets: %w", err)
	}
	for ticketRows.Next() {
		var id, number, title sql.NullString
		if err := ticketRows.Scan(&id, &number, &title); err != nil {
			_ = ticketRows.Close()
			return out, fmt.Errorf("scanning tickets: %w", err)
		}
		if !id.Valid || strings.TrimSpace(id.String) == "" {
			continue
		}
		tickets[id.String] = ticketRef{Number: stringsOrDefault(number, ""), Title: stringsOrDefault(title, "")}
	}
	if err := ticketRows.Err(); err != nil {
		_ = ticketRows.Close()
		return out, fmt.Errorf("reading tickets: %w", err)
	}
	if err := ticketRows.Close(); err != nil {
		return out, fmt.Errorf("closing tickets: %w", err)
	}

	seen := make(map[string]map[string]bool, len(feedbackBucketOrder))
	for _, name := range feedbackBucketOrder {
		seen[name] = make(map[string]bool)
	}
	resultCount := 0
	addMatches := func(ticketID string, ref ticketRef, text string) {
		for _, bucketName := range feedbackBucketOrder {
			if bucket != "" && bucket != bucketName {
				continue
			}
			matches := scanLexicon(text, feedbackBucketLexicons[bucketName])
			if len(matches) == 0 || seen[bucketName][ticketID] {
				continue
			}
			seen[bucketName][ticketID] = true
			out.Counts[bucketName]++
			if limit > 0 && resultCount >= limit {
				continue
			}
			match := matches[0]
			out.Buckets[bucketName] = append(out.Buckets[bucketName], feedbackScanMatch{
				Bucket:       bucketName,
				TicketID:     ticketID,
				TicketNumber: ref.Number,
				Title:        ref.Title,
				Snippet:      match.Snippet,
				MatchedTerm:  match.Term,
			})
			resultCount++
		}
	}

	ids := make([]string, 0, len(tickets))
	for id := range tickets {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		addMatches(id, tickets[id], tickets[id].Title)
	}

	articleRows, err := db.Query(`SELECT CAST(ticket_id AS TEXT), subject, body FROM articles WHERE sender_id = 2 AND internal = 0 ORDER BY ticket_id, created_at`)
	if err != nil {
		return out, fmt.Errorf("querying articles: %w", err)
	}
	defer articleRows.Close()
	for articleRows.Next() {
		var ticketID, subject, body sql.NullString
		if err := articleRows.Scan(&ticketID, &subject, &body); err != nil {
			return out, fmt.Errorf("scanning articles: %w", err)
		}
		if !ticketID.Valid || strings.TrimSpace(ticketID.String) == "" {
			continue
		}
		id := strings.TrimSpace(ticketID.String)
		ref := tickets[id]
		addMatches(id, ref, subject.String+"\n"+body.String)
	}
	if err := articleRows.Err(); err != nil {
		return out, fmt.Errorf("reading articles: %w", err)
	}
	return out, nil
}
