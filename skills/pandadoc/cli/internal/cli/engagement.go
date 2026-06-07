// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

type recipientEngagement struct {
	Email          string  `json:"email"`
	Name           string  `json:"name,omitempty"`
	Documents      int     `json:"documents"`
	Completed      int     `json:"completed"`
	CompletionRate float64 `json:"completion_rate"`
}

type engagementReport struct {
	Recipients []recipientEngagement `json:"recipients"`
	Note       string                `json:"note,omitempty"`
}

// newNovelEngagementCmd implements the "engagement" transcendence command:
// ranks recipients by how often the documents they receive reach completion.
// It joins each document's embedded recipients against the document's status,
// a recipient-centric rollup the document-centric API cannot return.
// pp:data-source local
func newNovelEngagementCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	var limit int
	cmd := &cobra.Command{
		Use:         "engagement",
		Short:       "Rank recipients by how often they open and sign vs. let documents sit unread.",
		Long:        "Rank recipients by how often the documents they are on reach completion. A recipient on many documents with few completions is a chronic staller.\n\nUse this command to rank RECIPIENTS by completion RATE across documents. Do NOT use it to rank clients by how long since they last signed; use 'cold-clients' instead.\n\nReads the local store — run `sync` first. Joins each document's embedded recipients with the document's status, a recipient-centric rollup the PandaDoc API cannot return in one call. Requires recipient data to be present in synced documents.",
		Example:     "  pandadoc-cli engagement --agent",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			db, _, err := openNovelStore(cmd, flags, dbPath, "documents")
			if err != nil {
				return err
			}
			defer db.Close()
			raws, err := db.List("documents", 1000000)
			if err != nil {
				return err
			}

			type agg struct {
				name      string
				docs      int
				completed int
			}
			byEmail := map[string]*agg{}
			docCount := len(raws)
			for _, raw := range raws {
				var m map[string]json.RawMessage
				if err := json.Unmarshal(raw, &m); err != nil {
					continue
				}
				st := normalizeStatus(jsonStr(m, "status"))
				// "Signed" means completed or paid — the same canonical set
				// cold-clients uses, so the two recipient rollups agree.
				docCompleted := st == "document.completed" || st == "document.paid"
				for _, rec := range parseRecipients(m) {
					a := byEmail[rec.Email]
					if a == nil {
						a = &agg{}
						byEmail[rec.Email] = a
					}
					if a.name == "" {
						a.name = rec.Name
					}
					a.docs++
					// per-recipient has_completed wins; else fall back to doc status
					var hc bool
					if hcRaw, ok := rec.Raw["has_completed"]; ok {
						_ = json.Unmarshal(hcRaw, &hc)
					}
					if hc || docCompleted {
						a.completed++
					}
				}
			}

			report := engagementReport{Recipients: make([]recipientEngagement, 0, len(byEmail))}
			for email, a := range byEmail {
				rate := 0.0
				if a.docs > 0 {
					rate = float64(a.completed) / float64(a.docs)
				}
				report.Recipients = append(report.Recipients, recipientEngagement{
					Email: email, Name: strings.TrimSpace(a.name),
					Documents: a.docs, Completed: a.completed, CompletionRate: rate,
				})
			}
			sort.Slice(report.Recipients, func(i, j int) bool {
				if report.Recipients[i].Documents != report.Recipients[j].Documents {
					return report.Recipients[i].Documents > report.Recipients[j].Documents
				}
				return report.Recipients[i].CompletionRate < report.Recipients[j].CompletionRate
			})
			if limit > 0 && len(report.Recipients) > limit {
				report.Recipients = report.Recipients[:limit]
			}
			if docCount == 0 {
				report.Note = "no documents in local store — run `pandadoc-cli sync` first"
			} else if len(report.Recipients) == 0 {
				report.Note = "synced documents have no embedded recipient data; nothing to rank"
			}

			w := cmd.OutOrStdout()
			if flags.asJSON || flags.agent || !isTerminal(w) {
				return printJSONFiltered(w, report, flags)
			}
			if len(report.Recipients) == 0 {
				fmt.Fprintln(w, report.Note)
				return nil
			}
			fmt.Fprintf(w, "Recipient engagement (by documents received):\n\n")
			for _, r := range report.Recipients {
				label := r.Email
				if r.Name != "" {
					label = r.Name + " <" + r.Email + ">"
				}
				fmt.Fprintf(w, "  %-40s docs=%-4d completed=%-4d rate=%.0f%%\n",
					truncateField(label, 40), r.Documents, r.Completed, r.CompletionRate*100)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (defaults to standard location)")
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum recipients to return (0 = no limit)")
	return cmd
}
