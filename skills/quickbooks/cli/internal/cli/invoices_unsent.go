// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"github.com/spf13/cobra"

	"quickbooks-pp-cli/internal/analytics"
)

// pp:data-source local
func newNovelInvoicesUnsentCmd(flags *rootFlags) *cobra.Command {
	var dbPath string

	cmd := &cobra.Command{
		Use:   "unsent",
		Short: "Open invoices QuickBooks has not emailed to the customer (EmailStatus NeedToSend or NotSet), ranked by balance",
		Long: "Billed but never delivered: open invoices whose EmailStatus is not EmailSent and\n" +
			"whose PrintStatus is not PrintComplete, largest balance first. has_bill_email\n" +
			"says whether the invoice has an address to send to. Invoices with no reported\n" +
			"EmailStatus are counted in status_unreported, not listed. Computed offline from\n" +
			"the local store. Run `sync` first.",
		Example:     "  quickbooks-cli invoices unsent --agent\n  quickbooks-cli invoices unsent --json --select invoices.doc_number,invoices.balance",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			ctx := cmd.Context()
			db, err := openLocalStore(ctx, dbPath)
			if err != nil {
				return err
			}
			defer db.Close()
			if !hintIfUnsynced(cmd, db, "invoices") {
				hintIfStale(cmd, db, "invoices", flags.maxAge)
			}
			invoices, err := loadResources(ctx, db, "invoices")
			if err != nil {
				return err
			}
			return flags.printJSON(cmd, analytics.UnsentInvoices(invoices))
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Path to the local SQLite store (default: per-CLI data dir)")
	return cmd
}
