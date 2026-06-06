// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel feature (Phase 3): per-company spend rollup. Not generator-managed.

package cli

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"
	"pax8-pp-cli/internal/store"
)

type spendRow struct {
	CompanyID    string  `json:"companyId"`
	CompanyName  string  `json:"companyName"`
	InvoiceCount int     `json:"invoiceCount"`
	Total        float64 `json:"total"`
	Currency     string  `json:"currency,omitempty"`
}

// pp:data-source local
func newNovelSpendCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	var limit int

	cmd := &cobra.Command{
		Use:         "spend",
		Short:       "Roll up invoice-items by company to show what each customer is costing across invoices.",
		Annotations: map[string]string{"mcp:read-only": "true"},
		Long: `Roll up invoice totals by company from the local store.

Sum every synced invoice per company and rank customers by spend — a per-company
rollup the Pax8 portal does not provide. Run 'pax8-cli sync' first.

Use this command to rank customers by total spend across invoices.
Do NOT use this command to detect billing mismatches; use 'reconcile' instead.
Do NOT use this command for a single named customer's full picture; use 'company show' instead.`,
		Example: `  # Top customers by spend
  pax8-cli spend

  # Agent-friendly JSON, top 10
  pax8-cli spend --agent --limit 10`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			if dryRunOK(flags) {
				return nil
			}
			out := cmd.OutOrStdout()
			if dbPath == "" {
				dbPath = defaultDBPath("pax8-cli")
			}
			db, err := store.OpenWithContext(cmd.Context(), dbPath)
			if err != nil {
				return fmt.Errorf("opening local database: %w\nRun 'pax8-cli sync' first.", err)
			}
			defer db.Close()
			maybeEmitSyncHints(cmd, db, "invoices", flags.maxAge)

			companies, err := pax8ListObjects(db, "companies")
			if err != nil {
				return fmt.Errorf("reading companies: %w", err)
			}
			nameByID := pax8NameByID(companies, []string{"id", "companyId"}, []string{"name", "companyName"})

			invoices, err := pax8ListObjects(db, "invoices")
			if err != nil {
				return fmt.Errorf("reading invoices: %w", err)
			}

			agg := map[string]*spendRow{}
			for _, inv := range invoices {
				cid := pax8FieldStr(inv, "companyId", "company_id")
				row := agg[cid]
				if row == nil {
					row = &spendRow{CompanyID: cid, CompanyName: nameByID[cid]}
					agg[cid] = row
				}
				if total, ok := pax8FieldNum(inv, "total"); ok {
					row.Total += total
				}
				if row.Currency == "" {
					row.Currency = pax8FieldStr(inv, "currencyCode", "currency_code")
				}
				row.InvoiceCount++
			}

			rows := make([]spendRow, 0, len(agg))
			for _, r := range agg {
				rows = append(rows, *r)
			}
			sort.Slice(rows, func(i, j int) bool { return rows[i].Total > rows[j].Total })
			if limit > 0 && len(rows) > limit {
				rows = rows[:limit]
			}

			if flags.asJSON {
				return printJSONFiltered(out, rows, flags)
			}
			if len(rows) == 0 {
				fmt.Fprintln(out, "No invoices in local store. Run 'pax8-cli sync' first.")
				return nil
			}
			fmt.Fprintln(out, "Company\tInvoices\tTotal")
			fmt.Fprintln(out, "-------\t--------\t-----")
			for _, r := range rows {
				name := r.CompanyName
				if name == "" {
					name = r.CompanyID
				}
				fmt.Fprintf(out, "%s\t%d\t%.2f %s\n", name, r.InvoiceCount, r.Total, r.Currency)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	cmd.Flags().IntVar(&limit, "limit", 25, "Max companies to show (0 = all)")
	return cmd
}
