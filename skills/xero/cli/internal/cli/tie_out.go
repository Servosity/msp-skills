// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"

	"xero-pp-cli/internal/xfin"
)

// pp:data-source local
func newNovelTieOutCmd(flags *rootFlags) *cobra.Command {
	var dbPath string

	cmd := &cobra.Command{
		Use:   "tie-out",
		Short: "Compare the GL receivable/payable control accounts against outstanding invoices and report the variance",
		Long: "Sum the immutable journal lines by account code for the receivable and payable control accounts\n" +
			"and compare each against the sum of outstanding invoice amounts, reporting the variance. The\n" +
			"general ledger is the independent oracle the documents are checked against — variance of zero is\n" +
			"the tie. Computed from synced journals + invoices + accounts. Run `sync` first.",
		Example:     "  xero-cli tie-out --json",
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
			jRows, err := loadAllRows(db, "journals")
			if err != nil {
				return fmt.Errorf("loading journals: %w", err)
			}
			iRows, err := loadAllRows(db, "invoices")
			if err != nil {
				return fmt.Errorf("loading invoices: %w", err)
			}
			aRows, err := loadAllRows(db, "accounts")
			if err != nil {
				return fmt.Errorf("loading accounts: %w", err)
			}
			report := xfin.ComputeTieOut(xfin.DecodeJournals(jRows), xfin.DecodeInvoices(iRows), xfin.DecodeAccounts(aRows))

			if flags.asJSON || (!isTerminal(cmd.OutOrStdout()) && !humanFriendly) {
				return printJSONFiltered(cmd.OutOrStdout(), report, flags)
			}
			out := [][]string{}
			for _, l := range report.Lines {
				ties := "TIES"
				if !l.Ties {
					ties = "VARIANCE"
				}
				out = append(out, []string{l.Control, l.AccountCode, money(l.LedgerBalance), money(l.InvoiceTotal), money(l.Variance), ties})
			}
			if len(report.Unmatched) > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "No control account resolved for: %v (sync accounts + journals)\n", report.Unmatched)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "All tie: %s\n", strconv.FormatBool(report.AllTie))
			return flags.printTable(cmd, []string{"control", "account", "ledger", "invoices", "variance", "status"}, out)
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/xero-cli/data.db)")
	return cmd
}
