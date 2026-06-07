// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

type followupRecipient struct {
	Email string `json:"email"`
	Name  string `json:"name,omitempty"`
}

type followupItem struct {
	ID          string              `json:"id"`
	Name        string              `json:"name"`
	Status      string              `json:"status"`
	DaysStalled int                 `json:"days_stalled"`
	Value       float64             `json:"value,omitempty"`
	Currency    string              `json:"currency,omitempty"`
	Recipients  []followupRecipient `json:"recipients"`
}

type followupReport struct {
	Items []followupItem `json:"items"`
	Note  string         `json:"note,omitempty"`
}

// newNovelFollowupCmd implements the "followup" transcendence command: a
// ranked nudge worklist that joins stalled documents to their recipient
// emails and days-since-activity — the API never returns documents with
// contact emails in one outreach-ready shape.
// pp:data-source local
func newNovelFollowupCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	var days int
	var limit int
	cmd := &cobra.Command{
		Use:         "followup",
		Short:       "A ranked nudge worklist: stalled documents joined to recipient emails and days-since-sent.",
		Long:        "Produce an actionable nudge worklist: open documents idle for N+ days, joined to their recipient emails, ordered coldest-and-biggest first.\n\nUse this command to produce an actionable NUDGE worklist: stalled documents joined to recipient emails + days-sent, ordered for outreach. Do NOT use it for the raw stalled-document list with no contact join; use 'stalled' instead.\n\nReads the local store — run `sync` first. Recipient emails require recipient data to be present in synced documents; items without it still list with an empty recipients array.",
		Example:     "  pandadoc-cli followup --days 7 --agent",
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

			report := followupReport{Items: make([]followupItem, 0)}
			withRecipients := 0
			for _, raw := range raws {
				var m map[string]json.RawMessage
				if err := json.Unmarshal(raw, &m); err != nil {
					continue
				}
				doc := parseNovelDoc(m)
				if isTerminalStatus(doc.Status) {
					continue
				}
				st := normalizeStatus(doc.Status)
				// Drafts haven't been sent; there is nobody to nudge yet.
				if st == "document.draft" || st == "document.uploaded" {
					continue
				}
				stalledFor := ageDays(doc.lastActivity())
				if stalledFor < days {
					continue
				}
				item := followupItem{
					ID:          doc.ID,
					Name:        doc.Name,
					Status:      shortStatus(doc.Status),
					DaysStalled: stalledFor,
					Value:       doc.GrandTotal,
					Currency:    doc.Currency,
					Recipients:  make([]followupRecipient, 0),
				}
				for _, rec := range parseRecipients(m) {
					item.Recipients = append(item.Recipients, followupRecipient{
						Email: rec.Email,
						Name:  rec.Name,
					})
				}
				if len(item.Recipients) > 0 {
					withRecipients++
				}
				report.Items = append(report.Items, item)
			}
			// Coldest first; dollars break ties so the biggest deals lead.
			sort.Slice(report.Items, func(i, j int) bool {
				a, b := report.Items[i], report.Items[j]
				if a.DaysStalled != b.DaysStalled {
					return a.DaysStalled > b.DaysStalled
				}
				if a.Value != b.Value {
					return a.Value > b.Value
				}
				return a.ID < b.ID
			})
			if limit > 0 && len(report.Items) > limit {
				report.Items = report.Items[:limit]
			}
			if len(raws) == 0 {
				report.Note = "no documents in local store — run `pandadoc-cli sync` first"
			} else if len(report.Items) == 0 {
				report.Note = fmt.Sprintf("nothing idle for %d+ days — the open pipeline is moving", days)
			} else if withRecipients == 0 {
				report.Note = "synced documents have no embedded recipient data; worklist has no contact emails"
			}

			w := cmd.OutOrStdout()
			if flags.asJSON || flags.agent || !isTerminal(w) {
				return printJSONFiltered(w, report, flags)
			}
			if len(report.Items) == 0 {
				fmt.Fprintln(w, report.Note)
				return nil
			}
			fmt.Fprintf(w, "Follow-up queue (idle %d+ days):\n\n", days)
			for _, it := range report.Items {
				emails := make([]string, 0, len(it.Recipients))
				for _, r := range it.Recipients {
					emails = append(emails, r.Email)
				}
				contact := strings.Join(emails, ", ")
				if contact == "" {
					contact = "(no recipient emails synced)"
				}
				val := ""
				if it.Value > 0 {
					val = fmt.Sprintf(" value=%.2f %s", it.Value, it.Currency)
				}
				fmt.Fprintf(w, "  %-36s %-9s idle=%dd%s\n      → %s\n",
					truncateField(it.Name, 36), it.Status, it.DaysStalled, val, contact)
			}
			if report.Note != "" {
				fmt.Fprintf(w, "\n  %s\n", report.Note)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (defaults to standard location)")
	cmd.Flags().IntVar(&days, "days", 7, "Minimum idle days before a document needs a nudge")
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum documents to return (0 = no limit)")
	return cmd
}
