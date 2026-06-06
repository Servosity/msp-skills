// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"

	"xero-pp-cli/internal/xfin"
)

// pp:data-source local
func newNovelExposureCmd(flags *rootFlags) *cobra.Command {
	var dbPath, asOf, contact string

	cmd := &cobra.Command{
		Use:   "exposure",
		Short: "Rank contacts by total outstanding amount due across their authorised invoices, with an overdue split",
		Long: "Rank contacts by total outstanding receivable amount due (with an overdue split and invoice\n" +
			"count), joining synced invoices to contacts locally. --contact <id> drills into a single\n" +
			"contact's outstanding invoices. Run `sync` first.",
		Example:     "  xero-cli exposure --json\n  xero-cli exposure --contact 00000000-0000-0000-0000-000000000000",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			as, err := parseAsOf(asOf)
			if err != nil {
				return err
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
			conRows, err := loadAllRows(db, "contacts")
			if err != nil {
				return fmt.Errorf("loading contacts: %w", err)
			}
			exposures := xfin.ComputeExposure(xfin.DecodeInvoices(invRows), xfin.DecodeContacts(conRows), as, contact)

			if flags.asJSON || (!isTerminal(cmd.OutOrStdout()) && !humanFriendly) {
				return printJSONFiltered(cmd.OutOrStdout(), exposures, flags)
			}
			out := [][]string{}
			for _, e := range exposures {
				out = append(out, []string{e.Contact, money(e.TotalDue), money(e.OverdueDue), strconv.Itoa(e.InvoiceCount), strconv.Itoa(e.OverdueCount)})
			}
			return flags.printTable(cmd, []string{"contact", "total due", "overdue due", "invoices", "overdue"}, out)
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/xero-cli/data.db)")
	cmd.Flags().StringVar(&asOf, "as-of", "", "Overdue cutoff date (YYYY-MM-DD); defaults to today")
	cmd.Flags().StringVar(&contact, "contact", "", "Drill into a single contact id (statement view)")
	return cmd
}
