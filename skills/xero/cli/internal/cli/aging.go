// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"xero-pp-cli/internal/xfin"
)

// pp:data-source local
func newNovelAgingCmd(flags *rootFlags) *cobra.Command {
	var dbPath, asOf string
	var payable bool

	cmd := &cobra.Command{
		Use:   "aging",
		Short: "Bucket outstanding invoices into current / 1-30 / 31-60 / 61-90 / 90+ days overdue with totals per bucket",
		Long: "Bucket outstanding invoices by how overdue they are (current / 1-30 / 31-60 / 61-90 / 90+),\n" +
			"computed locally from the synced invoices — no Reports API call. Receivables by default;\n" +
			"--payable buckets what you owe instead. Run `sync` first to populate the local store.",
		Example:     "  xero-cli aging --json\n  xero-cli aging --payable --as-of 2026-05-31",
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
			if !hintIfUnsynced(cmd, db, "invoices") {
				hintIfStale(cmd, db, "invoices", flags.maxAge)
			}
			rows, err := loadAllRows(db, "invoices")
			if err != nil {
				return fmt.Errorf("loading invoices: %w", err)
			}
			report := xfin.ComputeAging(xfin.DecodeInvoices(rows), as, payable)

			if flags.asJSON || (!isTerminal(cmd.OutOrStdout()) && !humanFriendly) {
				return printJSONFiltered(cmd.OutOrStdout(), report, flags)
			}
			out := [][]string{}
			for _, b := range report.Buckets {
				out = append(out, []string{b.Label, strconv.Itoa(b.Count), money(b.TotalDue)})
			}
			out = append(out, []string{"TOTAL", strconv.Itoa(report.InvoiceCount), money(report.TotalDue)})
			return flags.printTable(cmd, []string{report.Kind + " bucket (as of " + report.AsOf + ")", "count", "total due"}, out)
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/xero-cli/data.db)")
	cmd.Flags().StringVar(&asOf, "as-of", "", "Aging date (YYYY-MM-DD); defaults to today")
	cmd.Flags().BoolVar(&payable, "payable", false, "Bucket payable (ACCPAY) invoices instead of receivable (ACCREC)")
	return cmd
}

// parseAsOf parses a YYYY-MM-DD date, defaulting to today (UTC) when empty.
func parseAsOf(s string) (time.Time, error) {
	if s == "" {
		return time.Now().UTC(), nil
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid --as-of %q: use YYYY-MM-DD", s)
	}
	return t.UTC(), nil
}

// money formats an amount with 2 decimals for table output.
func money(f float64) string {
	return strconv.FormatFloat(f, 'f', 2, 64)
}
