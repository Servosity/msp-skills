// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"quotewerks-pp-cli/internal/qw"
)

func newNovelDocsSearchCmd(flags *rootFlags) *cobra.Command {
	var customer, contact, number, status, docType, rep, since, from, to, text, sortBy string
	var minTotal, maxTotal float64
	var limit int
	var allFields bool

	cmd := &cobra.Command{
		Use:   "search",
		Short: "Find quotes, orders and invoices by customer, number, status, rep, type or date",
		Long: `Find quotes, orders and invoices with typed filters that use the real QuoteWerks
column names (SoldToCompany, DocNo, DocStatus, DocType, SalesRep, DocDate, GrandTotal).
If QuoteWerks reports that it ignored a filter, the command fails instead of returning
unfiltered rows.

Use this command to find quotes, orders or invoices by customer, number, status, rep,
type or date. Do NOT use it for line-level part lookups; use 'items list' instead.`,
		Example: strings.Trim(`
  quotewerks-cli docs search --customer "Acme" --type QUOTE --agent
  quotewerks-cli docs search --rep "Derek Bryan" --status Open --since 30d --json
  quotewerks-cli docs search --number 1042 --json`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "docs search")
			}
			if limit <= 0 {
				return usageErr(fmt.Errorf("--limit must be positive"))
			}
			filters := make([]map[string]any, 0)
			add := func(name, op string, val any) { filters = append(filters, qw.Cond(name, op, val)) }
			if customer != "" {
				add("SoldToCompany", "contains", customer)
			}
			if contact != "" {
				add("SoldToContact", "contains", contact)
			}
			if number != "" {
				add("DocNo", "eq", number)
			}
			if status != "" {
				add("DocStatus", "eq", status)
			}
			if docType != "" {
				typeSet := upperCSVSet(docType)
				if err := validateDocTypes(typeSet, "--type"); err != nil {
					return err
				}
				if len(typeSet) == 1 {
					for t := range typeSet {
						add("DocType", "eq", t)
					}
				} else {
					vals := make([]any, 0, len(typeSet))
					for t := range typeSet {
						vals = append(vals, t)
					}
					add("DocType", "in", vals)
				}
			}
			if rep != "" {
				add("SalesRep", "eq", rep)
			}
			if text != "" {
				add("DocName", "contains", text)
			}
			if cutoff, ok, err := sinceCutoff(since, time.Now()); err != nil {
				return err
			} else if ok {
				add("DocDate", "gte", qw.FilterDate(cutoff))
			}
			if from != "" {
				if _, err := time.Parse("2006-01-02", from); err != nil {
					return usageErr(fmt.Errorf("--from %q: use YYYY-MM-DD", from))
				}
				add("DocDate", "gte", from+"T00:00:00")
			}
			if to != "" {
				day, err := time.Parse("2006-01-02", to)
				if err != nil {
					return usageErr(fmt.Errorf("--to %q: use YYYY-MM-DD", to))
				}
				// Exclusive next-midnight boundary keeps the whole day, fractions included.
				add("DocDate", "lt", day.AddDate(0, 0, 1).Format("2006-01-02")+"T00:00:00")
			}
			if cmd.Flags().Changed("min-total") {
				add("GrandTotal", "gte", minTotal)
			}
			if cmd.Flags().Changed("max-total") {
				add("GrandTotal", "lte", maxTotal)
			}
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			rows, _, err := qwSearch(cmd.Context(), c, qw.TableHeaders, qwSearchOpts{filters: filters, sort: sortBy, limit: limit, maxPages: qwAllPages})
			if err != nil {
				return asUsage(err)
			}
			out := make([]map[string]any, 0, len(rows))
			for _, r := range rows {
				row := r.Flat()
				if !allFields {
					row = project(row, qwHeaderFields)
				}
				if label := qw.QuoteValetLabel(row["QuoteValetStatusCode"]); label != "" {
					row["QuoteValetStatus"] = label
				}
				out = append(out, row)
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), out, flags)
			}
			if len(out) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No matching documents.")
				return nil
			}
			table := make([]map[string]any, 0, len(out))
			for _, r := range out {
				table = append(table, project(r, []string{"DocNo", "DocType", "DocStatus", "DocDate", "SoldToCompany", "SalesRep", "GrandTotal"}))
			}
			return printAutoTable(cmd.OutOrStdout(), table)
		},
	}
	cmd.Flags().StringVar(&customer, "customer", "", "Customer company name contains this text (SoldToCompany)")
	cmd.Flags().StringVar(&contact, "contact", "", "Customer contact name contains this text (SoldToContact)")
	cmd.Flags().StringVar(&number, "number", "", "Exact document number (DocNo)")
	cmd.Flags().StringVar(&status, "status", "", "Exact document status as your tenant names it, e.g. Open (DocStatus; list values with 'tables values DocumentHeaders DocStatus')")
	cmd.Flags().StringVar(&docType, "type", "", "Document types, comma-separated: QUOTE, ORDER, INVOICE (DocType)")
	cmd.Flags().StringVar(&rep, "rep", "", "Exact sales rep user name (SalesRep)")
	cmd.Flags().StringVar(&text, "text", "", "Document name contains this text (DocName)")
	cmd.Flags().StringVar(&since, "since", "", "Only documents dated within this window, e.g. 30d, 12w (DocDate)")
	cmd.Flags().StringVar(&from, "from", "", "Only documents dated on or after this date, YYYY-MM-DD")
	cmd.Flags().StringVar(&to, "to", "", "Only documents dated on or before this date, YYYY-MM-DD")
	cmd.Flags().Float64Var(&minTotal, "min-total", 0, "Only documents with GrandTotal at or above this amount")
	cmd.Flags().Float64Var(&maxTotal, "max-total", 0, "Only documents with GrandTotal at or below this amount")
	cmd.Flags().StringVar(&sortBy, "sort", "-DocDate", "Sort columns, comma-separated; prefix - for descending")
	cmd.Flags().IntVar(&limit, "limit", 25, "Maximum documents to return")
	cmd.Flags().BoolVar(&allFields, "all-fields", false, "Return every DocumentHeaders column instead of the key fields")
	return cmd
}
