// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
package cli

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

// newNovelCompany360Cmd assembles everything the local store knows about one
// company — five entity tabs in the Autotask UI, one command here.
// pp:data-source local
func newNovelCompany360Cmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	cmd := &cobra.Command{
		Use:   "company-360 [id]",
		Short: "One view of a company's tickets, contacts, contracts, config items, and opportunities.",
		Long: `Assemble everything the local store knows about one company: its tickets, contacts, contracts, configuration items, and opportunities, keyed on companyID. Run ` + "`sync`" + ` first.

Use this command for the full current snapshot of an account. For what CHANGED on an account since a point in time, use 'account-brief'.`,
		Example: strings.Trim(`
  autotask-cli company-360 1234
  autotask-cli company-360 1234 --agent
  autotask-cli company-360 1234 --json`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return nil
			}
			if len(args) == 0 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("company id is required"))
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			companyID := strings.TrimSpace(args[0])
			if _, err := strconv.Atoi(companyID); err != nil {
				return usageErr(fmt.Errorf("company id must be an integer, got %q", companyID))
			}
			db, err := openNovelStore(cmd.Context(), dbPath)
			if err != nil {
				return err
			}
			defer db.Close()
			if !hintIfUnsynced(cmd, db, "companies") {
				hintIfStale(cmd, db, "companies", flags.maxAge)
			}
			if !hintIfUnsynced(cmd, db, "tickets") {
				hintIfStale(cmd, db, "tickets", flags.maxAge)
			}

			matchCompany := func(resourceType string, keys ...string) []map[string]any {
				recs, err := listEntity(db, resourceType)
				if err != nil {
					return nil
				}
				var out []map[string]any
				for _, r := range recs {
					if strAt(r, keys...) == companyID {
						out = append(out, r)
					}
				}
				return out
			}

			tickets := matchCompany("tickets", "companyID", "companyId")
			openTickets := 0
			for _, t := range tickets {
				if isTicketOpen(t) {
					openTickets++
				}
			}
			contacts := matchCompany("contacts", "companyID", "companyId")
			contracts := matchCompany("contracts", "companyID", "companyId")
			cis := matchCompany("configuration-items", "companyID", "companyId")
			opps := matchCompany("opportunities", "companyID", "companyId")

			var companyName string
			if companies, err := listEntity(db, "companies"); err == nil {
				for _, c := range companies {
					if strAt(c, "id") == companyID {
						companyName = strAt(c, "companyName")
						break
					}
				}
			}

			out := map[string]any{
				"companyID":   companyID,
				"companyName": companyName,
				"summary": map[string]int{
					"tickets":            len(tickets),
					"openTickets":        openTickets,
					"contacts":           len(contacts),
					"contracts":          len(contracts),
					"configurationItems": len(cis),
					"opportunities":      len(opps),
				},
				"tickets":            tickets,
				"contacts":           contacts,
				"contracts":          contracts,
				"configurationItems": cis,
				"opportunities":      opps,
			}
			return flags.printJSON(cmd, out)
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/autotask-cli/data.db)")
	return cmd
}
