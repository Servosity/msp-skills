// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/spf13/cobra"
)

type coldClient struct {
	Email           string `json:"email"`
	Name            string `json:"name,omitempty"`
	Documents       int    `json:"documents"`
	LastSignedAt    string `json:"last_signed_at,omitempty"`
	DaysSinceSigned int    `json:"days_since_signed,omitempty"`
	NeverSigned     bool   `json:"never_signed"`
}

type coldClientsReport struct {
	Clients []coldClient `json:"clients"`
	Note    string       `json:"note,omitempty"`
}

// newNovelColdClientsCmd implements the "cold-clients" transcendence command:
// ranks clients (recipient emails) by how long since they last signed any
// document, surfacing the accounts that have gone quiet. Joins each document's
// embedded recipients against completion dates across the whole corpus — an
// account-recency rollup no single PandaDoc API call returns.
// pp:data-source local
func newNovelColdClientsCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	var days int
	var limit int
	cmd := &cobra.Command{
		Use:         "cold-clients",
		Short:       "Rank clients by how long since they last signed anything — spot the accounts going quiet.",
		Long:        "Rank clients (recipient emails) by how long since they last signed any document, surfacing accounts that have gone quiet.\n\nUse this command to rank CLIENTS/contacts by recency — how long since they last signed anything. Do NOT use it to rank recipients by completion rate; use 'engagement' instead.\n\nReads the local store — run `sync` first. Joins each document's embedded recipients against completion dates across the whole corpus; requires recipient data to be present in synced documents.",
		Example:     "  pandadoc-cli cold-clients --days 30 --agent",
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
				name       string
				docs       int
				lastSigned time.Time
			}
			byEmail := map[string]*agg{}
			now := time.Now().UTC()
			for _, raw := range raws {
				var m map[string]json.RawMessage
				if err := json.Unmarshal(raw, &m); err != nil {
					continue
				}
				doc := parseNovelDoc(m)
				st := normalizeStatus(doc.Status)
				completed := st == "document.completed" || st == "document.paid"
				signedAt := doc.DateCompleted
				if signedAt.IsZero() {
					signedAt = doc.DateModified
				}
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
					if completed && !signedAt.IsZero() && signedAt.After(a.lastSigned) {
						a.lastSigned = signedAt
					}
				}
			}

			report := coldClientsReport{Clients: make([]coldClient, 0, len(byEmail))}
			for email, a := range byEmail {
				c := coldClient{Email: email, Name: a.name, Documents: a.docs}
				if a.lastSigned.IsZero() {
					c.NeverSigned = true
				} else {
					c.LastSignedAt = a.lastSigned.Format(time.RFC3339)
					c.DaysSinceSigned = int(now.Sub(a.lastSigned).Hours() / 24)
				}
				// Only clients colder than the threshold make the report.
				if c.NeverSigned || c.DaysSinceSigned >= days {
					report.Clients = append(report.Clients, c)
				}
			}
			// Coldest first: never-signed leads, then by days since last signature.
			sort.Slice(report.Clients, func(i, j int) bool {
				a, b := report.Clients[i], report.Clients[j]
				if a.NeverSigned != b.NeverSigned {
					return a.NeverSigned
				}
				if a.DaysSinceSigned != b.DaysSinceSigned {
					return a.DaysSinceSigned > b.DaysSinceSigned
				}
				return a.Email < b.Email
			})
			if limit > 0 && len(report.Clients) > limit {
				report.Clients = report.Clients[:limit]
			}
			if len(raws) == 0 {
				report.Note = "no documents in local store — run `pandadoc-cli sync` first"
			} else if len(byEmail) == 0 {
				report.Note = "synced documents have no embedded recipient data; nothing to rank"
			} else if len(report.Clients) == 0 {
				report.Note = fmt.Sprintf("no clients colder than %d days — every known recipient signed recently", days)
			}

			w := cmd.OutOrStdout()
			if flags.asJSON || flags.agent || !isTerminal(w) {
				return printJSONFiltered(w, report, flags)
			}
			if len(report.Clients) == 0 {
				fmt.Fprintln(w, report.Note)
				return nil
			}
			fmt.Fprintf(w, "Cold clients (no signature in %d+ days):\n\n", days)
			for _, c := range report.Clients {
				label := c.Email
				if c.Name != "" {
					label = c.Name + " <" + c.Email + ">"
				}
				when := "never signed"
				if !c.NeverSigned {
					when = fmt.Sprintf("last signed %d days ago", c.DaysSinceSigned)
				}
				fmt.Fprintf(w, "  %-44s docs=%-4d %s\n", truncateField(label, 44), c.Documents, when)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (defaults to standard location)")
	cmd.Flags().IntVar(&days, "days", 30, "Minimum days since last signature to count as cold")
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum clients to return (0 = no limit)")
	return cmd
}
