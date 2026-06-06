// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// pp:data-source local
func newNovelAccountCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "account [id-or-identifier]",
		Short: "Account 360: contacts, agreements, configurations, open tickets for one company.",
		Long: strings.Trim(`
Assembles a one-card view of a company from the local store — contacts,
active agreements, deployed configurations, open-ticket count, and last
activity — joining five synced tables the PSA web UI spreads across five
screens. Pass a company id or identifier. Run
'sync company-companies company company-configurations finance-agreements service-tickets' first.

Use this command to assemble a company's full context (contacts, agreements,
configs, open tickets) before a call. Do NOT use this command to measure
hours-vs-allotment utilization on an agreement; use 'agreement-burn' instead.`, "\n"),
		Example: strings.Trim(`
  connectwise-manage-cli account AcmeCorp
  connectwise-manage-cli account 42 --agent`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			query := args[0]

			db, err := cwOpenStore(cmd.Context())
			if err != nil {
				return cwNoStoreHint(cmd, flags, accountCard{Found: false}, nil, "company-companies company company-configurations finance-agreements service-tickets")
			}
			defer db.Close()
			if !hintIfUnsynced(cmd, db, cwCompanies) {
				hintIfStale(cmd, db, cwCompanies, flags.maxAge)
			}

			companies, err := cwLoad(cmd.Context(), db, cwCompanies)
			if err != nil {
				return err
			}
			contacts, err := cwLoad(cmd.Context(), db, cwContacts)
			if err != nil {
				return err
			}
			agreements, err := cwLoad(cmd.Context(), db, cwAgreements)
			if err != nil {
				return err
			}
			configs, err := cwLoad(cmd.Context(), db, cwConfigs)
			if err != nil {
				return err
			}
			tickets, err := cwLoad(cmd.Context(), db, cwTickets)
			if err != nil {
				return err
			}

			card := computeAccount(companies, contacts, agreements, configs, tickets, query)

			if flags.asJSON || flags.compact || flags.csv || flags.quiet || (!isTerminal(cmd.OutOrStdout()) && !humanFriendly) {
				if err := flags.printJSON(cmd, card); err != nil {
					return err
				}
				if !card.Found {
					return fmt.Errorf("no company matched %q (try the company identifier or numeric id; run sync if the store is empty)", query)
				}
				return nil
			}

			w := cmd.OutOrStdout()
			if !card.Found {
				fmt.Fprintf(w, "No company matched %q. Try the company identifier or numeric id, or run `sync company-companies`.\n", query)
				return fmt.Errorf("company not found: %s", query)
			}
			fmt.Fprintf(w, "Account: %s (%s)  id=%d  status=%s\n", card.Name, card.Identifier, card.ID, dash(card.Status))
			fmt.Fprintf(w, "  Open tickets:   %d\n", card.OpenTickets)
			fmt.Fprintf(w, "  Contacts:       %d  %s\n", card.Contacts, strings.Join(card.ContactNames, ", "))
			fmt.Fprintf(w, "  Configurations: %d\n", card.Configurations)
			fmt.Fprintf(w, "  Agreements:     %d  %s\n", len(card.Agreements), strings.Join(card.Agreements, ", "))
			fmt.Fprintf(w, "  Last activity:  %s\n", dash(card.LastActivity))
			return nil
		},
	}
	return cmd
}

func dash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "-"
	}
	return s
}
