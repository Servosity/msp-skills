// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel feature (Phase 3): per-company spend rollup. Not generator-managed.

package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"pax8-pp-cli/internal/store"
)

type spendRow struct {
	CompanyID    string  `json:"companyId"`
	CompanyName  string  `json:"companyName"`
	InvoiceCount int     `json:"invoiceCount"`
	Total        float64 `json:"total"`
	Currency     string  `json:"currency,omitempty"`
	// Invoices left out of Total (pax8InvoiceOwed): not owed, or with a status
	// Pax8 does not document, whose amount stays visible here.
	NotOwedInvoices      int     `json:"notOwedInvoices,omitempty"`
	UnclassifiedInvoices int     `json:"unclassifiedInvoices,omitempty"`
	UnclassifiedTotal    float64 `json:"unclassifiedTotal,omitempty"`
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

Sum Invoice.total per company and rank customers by spend, a per-company rollup
the Pax8 portal does not provide. Every company is shown unless --limit asks for
fewer. Run 'pax8-cli sync' first.

An invoice counts when Pax8 billed it (status Unpaid, Paid or Carried). Void,
Nothing Due and Credited invoices are left out and counted in notOwedInvoices. Any
other status, or none, is left out and reported in unclassifiedInvoices and
unclassifiedTotal. Currencies are never added: a company billed in two has one row
per currency. Invoice.carriedBalance is not subtracted, because Pax8 does not
document whether Invoice.total includes it.

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

			// One row per company and currency: money is never added across
			// currencyCode, and pax8InvoiceOwed decides what counts.
			type spendKey struct{ company, currency string }
			agg := map[spendKey]*spendRow{}
			for _, inv := range invoices {
				cid := pax8FieldStr(inv, "companyId", "company_id")
				key := spendKey{cid, pax8Currency(inv)}
				row := agg[key]
				if row == nil {
					row = &spendRow{CompanyID: cid, CompanyName: nameByID[cid], Currency: key.currency}
					agg[key] = row
				}
				total, _ := pax8FieldNum(inv, "total")
				owed, known := pax8InvoiceOwed(pax8FieldStr(inv, "status"))
				switch {
				case !known:
					row.UnclassifiedInvoices++
					row.UnclassifiedTotal += total
				case !owed:
					row.NotOwedInvoices++
				default:
					row.Total += total
					row.InvoiceCount++
				}
			}

			rows := make([]spendRow, 0, len(agg))
			for _, r := range agg {
				rows = append(rows, *r)
			}
			sort.Slice(rows, func(i, j int) bool {
				if rows[i].Total != rows[j].Total {
					return rows[i].Total > rows[j].Total
				}
				if rows[i].CompanyID != rows[j].CompanyID {
					return rows[i].CompanyID < rows[j].CompanyID
				}
				return rows[i].Currency < rows[j].Currency
			})
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
				if name == "" {
					name = "(no company)" // examples/invoices-paged carries no companyId
				}
				var left []string
				if n := r.NotOwedInvoices; n > 0 {
					left = append(left, fmt.Sprintf("%d not owed", n))
				}
				if n := r.UnclassifiedInvoices; n > 0 {
					left = append(left, fmt.Sprintf("%d with a status Pax8 does not document, %s", n, pax8Money(r.UnclassifiedTotal, r.Currency)))
				}
				note := ""
				if len(left) > 0 {
					note = "\t(left out: " + strings.Join(left, "; ") + ")"
				}
				fmt.Fprintf(out, "%s\t%d\t%s%s\n", name, r.InvoiceCount, pax8Money(r.Total, r.Currency), note)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	cmd.Flags().IntVar(&limit, "limit", 0, "Max rows to show, highest first (0 = all)")
	return cmd
}
