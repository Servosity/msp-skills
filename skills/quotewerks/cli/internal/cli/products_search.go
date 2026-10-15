// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"quotewerks-pp-cli/internal/cliutil"
	"quotewerks-pp-cli/internal/qw"
)

func newNovelProductsSearchCmd(flags *rootFlags) *cobra.Command {
	var search, part, manufacturer, productDB string
	var limit int
	var dbsOnly bool

	cmd := &cobra.Command{
		Use:   "search [query]",
		Short: "Search parts across every QuoteWerks product database at once",
		Long: `Search products across all Products_<Db>_Products tables (QuoteWerks Web keeps one
table per product database). The query matches the description or manufacturer part
number; --part matches a part number exactly. Every row is tagged with its product
database. Use --dbs to list the product databases themselves.`,
		Example: strings.Trim(`
  quotewerks-cli products search "ThinkPad" --limit 20 --json
  quotewerks-cli products search --part 20UES0X100 --json
  quotewerks-cli products search --dbs --json`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "products search")
			}
			if len(args) > 0 {
				search = strings.Join(args, " ")
			}
			if limit <= 0 {
				return usageErr(fmt.Errorf("--limit must be positive"))
			}
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			tables, err := productTables(ctx, c, productDB)
			if err != nil {
				return err
			}
			if dbsOnly {
				dbs := make([]map[string]any, 0, len(tables))
				for _, t := range tables {
					db, _ := qw.ProductDB(t)
					dbs = append(dbs, map[string]any{"product_db": db, "table": t})
				}
				if !wantsHumanTable(cmd.OutOrStdout(), flags) {
					return printJSONFiltered(cmd.OutOrStdout(), dbs, flags)
				}
				return printAutoTable(cmd.OutOrStdout(), dbs)
			}
			products := make([]qw.Product, 0)
			if part != "" {
				found, err := catalogLookup(ctx, c, tables, []string{part})
				if err != nil {
					return err
				}
				products = found
			} else {
				filters := make([]map[string]any, 0)
				if search != "" {
					filters = append(filters, qw.Or(qw.Cond("Description", "contains", search), qw.Cond("ManufacturerPartNumber", "contains", search)))
				}
				if manufacturer != "" {
					filters = append(filters, qw.Cond("Manufacturer", "eq", manufacturer))
				}
				for _, t := range tables {
					if len(products) >= limit {
						break
					}
					rows, _, err := qwSearch(ctx, c, t, qwSearchOpts{filters: filters, limit: limit - len(products), maxPages: qwAllPages})
					if err != nil {
						return asUsage(err)
					}
					for _, r := range rows {
						products = append(products, qw.ProductFromAttrs(t, r.ID, r.Attributes))
					}
				}
			}
			if manufacturer != "" && part != "" {
				kept := products[:0]
				for _, p := range products {
					if strings.EqualFold(p.Manufacturer, manufacturer) {
						kept = append(kept, p)
					}
				}
				products = kept
			}
			if len(products) > limit {
				products = products[:limit]
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), products, flags)
			}
			if len(products) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No matching products.")
				return nil
			}
			for _, p := range products {
				money := func(v float64, ok bool) string {
					if !ok {
						return "       n/a"
					}
					return fmt.Sprintf("%10.2f", v)
				}
				note := ""
				if p.PricingFactor {
					note = "  (priced per unit of pricing other than one)"
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%-14.14s %-22.22s %-40.40s cost %s  price %s%s\n", cliutil.ScrubTerminal(p.DB), cliutil.ScrubTerminal(p.ManufacturerPartNumber), cliutil.ScrubTerminal(p.Description), money(p.Cost, p.HasCost), money(p.Price, p.HasPrice), note)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&part, "part", "", "Exact manufacturer or vendor part number")
	cmd.Flags().StringVar(&manufacturer, "manufacturer", "", "Exact manufacturer name")
	cmd.Flags().StringVar(&productDB, "product-db", "", "One product database only (name from --dbs)")
	cmd.Flags().IntVar(&limit, "limit", 25, "Maximum products to return")
	cmd.Flags().BoolVar(&dbsOnly, "dbs", false, "List the product databases instead of products")
	return cmd
}
