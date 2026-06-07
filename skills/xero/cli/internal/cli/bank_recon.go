// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"

	"xero-pp-cli/internal/xfin"
)

// pp:data-source local
func newNovelBankReconCmd(flags *rootFlags) *cobra.Command {
	var dbPath string

	cmd := &cobra.Command{
		Use:   "bank-recon",
		Short: "List unreconciled bank transactions and match them to invoices and payments by contact and exact amount",
		Long: "List bank transactions still flagged unreconciled (the synced IsReconciled=false rows) and suggest\n" +
			"likely matches against invoices and payments by same contact + equal amount. Computed locally —\n" +
			"a reconciliation worksheet, not a live call. Run `sync` first.",
		Example:     "  xero-cli bank-recon --json",
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
			txnRows, err := loadAllRows(db, "bank-transactions")
			if err != nil {
				return fmt.Errorf("loading bank transactions: %w", err)
			}
			invRows, err := loadAllRows(db, "invoices")
			if err != nil {
				return fmt.Errorf("loading invoices: %w", err)
			}
			payRows, err := loadAllRows(db, "payments")
			if err != nil {
				return fmt.Errorf("loading payments: %w", err)
			}
			report := xfin.ComputeBankRecon(
				xfin.DecodeBankTransactions(txnRows),
				xfin.DecodeInvoices(invRows),
				xfin.DecodePayments(payRows),
			)

			if flags.asJSON || (!isTerminal(cmd.OutOrStdout()) && !humanFriendly) {
				return printJSONFiltered(cmd.OutOrStdout(), report, flags)
			}
			out := [][]string{}
			for _, u := range report.Unreconciled {
				out = append(out, []string{u.BankTransactionID, u.Type, u.Contact, money(u.Total), strconv.Itoa(len(u.Candidates))})
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Unreconciled: %d transaction(s), %s total\n", report.UnreconciledCount, money(report.UnreconciledTotal))
			return flags.printTable(cmd, []string{"txn id", "type", "contact", "total", "candidate matches"}, out)
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/xero-cli/data.db)")
	return cmd
}
