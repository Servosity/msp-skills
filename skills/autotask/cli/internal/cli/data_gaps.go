// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// newNovelDataGapsCmd anti-joins synced tables to surface referential-integrity
// gaps — tickets with no contract, time entries linked to nothing, contacts
// and config items pointing at companies the store has never seen. These are
// the data-hygiene holes that silently break billing and reporting.
// pp:data-source local
func newNovelDataGapsCmd(flags *rootFlags) *cobra.Command {
	var flagEntity string
	var dbPath string
	cmd := &cobra.Command{
		Use:   "data-gaps",
		Short: "Find tickets with no contract, unlinked time entries, and contacts or config items with no company.",
		Long:  "Anti-join synced tables (LEFT JOIN ... WHERE NULL, in Go) to report referential-integrity gaps: tickets missing a contract, time entries linked to neither ticket nor task, and contacts or configuration items whose companyID is absent from the local store. Run `sync` first.",
		Example: strings.Trim(`
  autotask-cli data-gaps
  autotask-cli data-gaps --entity tickets --agent
  autotask-cli data-gaps --json --select counts`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			entity := strings.TrimSpace(flagEntity)
			validEntities := map[string]bool{"": true, "tickets": true, "time-entries": true, "contacts": true, "configuration-items": true}
			if !validEntities[entity] {
				return usageErr(fmt.Errorf("invalid --entity %q: must be tickets, time-entries, contacts, or configuration-items", entity))
			}
			db, err := openNovelStore(cmd.Context(), dbPath)
			if err != nil {
				return err
			}
			defer db.Close()
			if !hintIfUnsynced(cmd, db, "tickets") {
				hintIfStale(cmd, db, "tickets", flags.maxAge)
			}

			// Build the referenced-ID sets once.
			companies, _ := listEntity(db, "companies")
			companyIDs := map[string]bool{}
			for _, c := range companies {
				if id := strAt(c, "id"); id != "" {
					companyIDs[id] = true
				}
			}
			haveCompanies := len(companyIDs) > 0

			type gap struct {
				Entity string `json:"entity"`
				ID     int64  `json:"id"`
				Issue  string `json:"issue"`
			}
			var gaps []gap
			add := func(entityName string, m map[string]any, issue string) {
				id, _ := intAt(m, "id")
				gaps = append(gaps, gap{Entity: entityName, ID: id, Issue: issue})
			}
			want := func(e string) bool { return entity == "" || entity == e }

			if want("tickets") {
				tickets, err := listEntity(db, "tickets")
				if err != nil {
					return apiErr(err)
				}
				for _, t := range tickets {
					if !isTicketOpen(t) {
						continue
					}
					if cid, ok := intAt(t, "contractID", "contractId"); !ok || cid == 0 {
						add("tickets", t, "no contract attached")
					}
					if haveCompanies {
						if comp := strAt(t, "companyID", "companyId"); comp != "" && !companyIDs[comp] {
							add("tickets", t, "companyID not in local store")
						}
					}
				}
			}
			if want("time-entries") {
				entries, err := listEntity(db, "time-entries")
				if err != nil {
					return apiErr(err)
				}
				for _, e := range entries {
					tid, hasTicket := intAt(e, "ticketID", "ticketId")
					kid, hasTask := intAt(e, "taskID", "taskId")
					if (!hasTicket || tid == 0) && (!hasTask || kid == 0) {
						add("time-entries", e, "linked to neither ticket nor task")
					}
				}
			}
			if want("contacts") {
				contacts, err := listEntity(db, "contacts")
				if err != nil {
					return apiErr(err)
				}
				for _, c := range contacts {
					comp := strAt(c, "companyID", "companyId")
					if comp == "" || comp == "0" {
						add("contacts", c, "no company")
					} else if haveCompanies && !companyIDs[comp] {
						add("contacts", c, "companyID not in local store")
					}
				}
			}
			if want("configuration-items") {
				cis, err := listEntity(db, "configuration-items")
				if err != nil {
					return apiErr(err)
				}
				for _, ci := range cis {
					comp := strAt(ci, "companyID", "companyId")
					if comp == "" || comp == "0" {
						add("configuration-items", ci, "no company")
					} else if haveCompanies && !companyIDs[comp] {
						add("configuration-items", ci, "companyID not in local store")
					}
				}
			}

			counts := map[string]int{}
			for _, g := range gaps {
				counts[g.Entity]++
			}
			out := map[string]any{
				"counts":                counts,
				"totalGaps":             len(gaps),
				"companiesInLocalStore": len(companyIDs),
				"gaps":                  gaps,
			}
			if !haveCompanies {
				out["note"] = "companies not synced locally; companyID membership checks were skipped. Run 'autotask-cli sync --resources companies' for full coverage."
			}
			return flags.printJSON(cmd, out)
		},
	}
	cmd.Flags().StringVar(&flagEntity, "entity", "", "limit the scan to one entity: tickets, time-entries, contacts, or configuration-items")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/autotask-cli/data.db)")
	return cmd
}
