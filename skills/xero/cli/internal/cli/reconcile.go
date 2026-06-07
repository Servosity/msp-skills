// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"xero-pp-cli/internal/xfin"
)

// pp:data-source local
func newNovelReconcileCmd(flags *rootFlags) *cobra.Command {
	var dbPath string

	cmd := &cobra.Command{
		Use:   "reconcile",
		Short: "Cross-join invoices and payments locally to surface invoices owed with no (or partial) applied payment",
		Long: "Surface the gap between the invoice ledger and applied cash: AUTHORISED receivable invoices\n" +
			"still owed (with the payments the store can see applied to each), and payments referencing an\n" +
			"invoice not present locally. Computed from the synced invoices + payments — no live API call.",
		Example:     "  xero-cli reconcile --json",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			db, err := openXeroStore(cmd.Context(), dbPath)
			if err != nil {
				return fmt.Errorf("opening store: %w", err)
			}
			defer db.Close()
			if !hintIfUnsynced(cmd, db, "") {
				hintIfStale(cmd, db, "", flags.maxAge)
			}
			invRows, err := loadAllRows(db, "invoices")
			if err != nil {
				return fmt.Errorf("loading invoices: %w", err)
			}
			payRows, err := loadAllRows(db, "payments")
			if err != nil {
				return fmt.Errorf("loading payments: %w", err)
			}
			report := xfin.ComputeReconcile(xfin.DecodeInvoices(invRows), xfin.DecodePayments(payRows))

			if flags.asJSON || (!isTerminal(cmd.OutOrStdout()) && !humanFriendly) {
				return printJSONFiltered(cmd.OutOrStdout(), report, flags)
			}
			out := [][]string{}
			for _, o := range report.OutstandingInvoices {
				out = append(out, []string{o.InvoiceNumber, o.Contact, money(o.AmountDue), money(o.AppliedGap)})
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Outstanding receivable: %s across %d invoices; %d orphan payment(s)\n",
				money(report.TotalOutstanding), report.OutstandingCount, report.OrphanCount)
			return flags.printTable(cmd, []string{"invoice", "contact", "amount due", "applied gap"}, out)
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/xero-cli/data.db)")
	return cmd
}
