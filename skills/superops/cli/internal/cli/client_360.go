// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source local

package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func newNovelClient360Cmd(flags *rootFlags) *cobra.Command {
	var dbPath string

	cmd := &cobra.Command{
		Use:   "client-360 <client>",
		Short: "One offline bundle of a client plus its sites, users, contracts, open tickets, assets, and open invoices.",
		Long: `Assemble everything tied to one client from the local store in a single command:
the client record, its sites, users, contracts, open tickets, assets, and open
(unpaid) invoices. Replaces six separate web UI page loads. Accepts a client
account ID or an exact client name.`,
		Example: strings.Trim(`
  superops-cli client-360 5016054681314398208
  superops-cli client-360 "Acme Corp" --json
`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local", "pp:happy-args": "Acme"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			if len(args) == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "client-360")
			}
			query := strings.TrimSpace(args[0])
			db, err := openStoreForNovel(cmd.Context(), dbPath)
			if err != nil {
				return err
			}
			defer db.Close()
			clients, err := loadRecs(cmd, db, "clients")
			if err != nil {
				return err
			}
			sites, err := loadRecs(cmd, db, "sites")
			if err != nil {
				return err
			}
			users, err := loadRecs(cmd, db, "users")
			if err != nil {
				return err
			}
			contracts, err := loadRecs(cmd, db, "contracts")
			if err != nil {
				return err
			}
			tickets, err := loadRecs(cmd, db, "tickets")
			if err != nil {
				return err
			}
			assets, err := loadRecs(cmd, db, "assets")
			if err != nil {
				return err
			}
			invoices, err := loadRecs(cmd, db, "invoices")
			if err != nil {
				return err
			}

			bundle, ok := buildClient360(query, clients, sites, users, contracts, tickets, assets, invoices)
			if !ok {
				return fmt.Errorf("no client matching %q in the local store (try the account ID or exact name; run 'sync' first)", query)
			}
			if flags.asJSON {
				return printJSONFiltered(cmd.OutOrStdout(), bundle, flags)
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Client 360: %s (%s)\n", recStr(bundle.Client, "name"), recStr(bundle.Client, "accountId"))
			fmt.Fprintf(out, "  Sites:         %d\n", len(bundle.Sites))
			fmt.Fprintf(out, "  Users:         %d\n", len(bundle.Users))
			fmt.Fprintf(out, "  Contracts:     %d\n", len(bundle.Contracts))
			fmt.Fprintf(out, "  Open tickets:  %d\n", len(bundle.OpenTickets))
			fmt.Fprintf(out, "  Assets:        %d\n", len(bundle.Assets))
			fmt.Fprintf(out, "  Open invoices: %d\n", len(bundle.OpenInvoices))
			fmt.Fprintln(out, "\n(use --json for the full bundle)")
			return nil
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/superops-cli/data.db)")
	return cmd
}
