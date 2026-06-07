// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"
)

// velocityProduct is one catalog product ranked by quote activity.
type velocityProduct struct {
	Product    string  `json:"product"`
	MPN        string  `json:"mpn,omitempty"`
	ProductID  string  `json:"productId,omitempty"`
	QuoteCount int     `json:"quoteCount"`
	TotalQty   float64 `json:"totalQty"`
	TotalValue float64 `json:"totalValue"`
}

type productVelocityReport struct {
	QuotesScanned int               `json:"quotesScanned"`
	Rows          []velocityProduct `json:"rows"`
	Note          string            `json:"note,omitempty"`
}

// newNovelProductVelocityCmd implements the "product velocity" transcendence
// command: the most-quoted catalog products by frequency and value.
// pp:data-source local
func newNovelProductVelocityCmd(flags *rootFlags) *cobra.Command {
	var flagLimit int
	var dbPath string

	cmd := &cobra.Command{
		Use:   "velocity",
		Short: "Rank catalog products by quote frequency and total quoted value — the de-facto bestsellers",
		Long: "Use this command for the most-quoted catalog products ranked by frequency and value. Do NOT\n" +
			"use this command to find products a company has never been quoted; use 'company whitespace'\n" +
			"instead.\n\n" +
			"Joins every synced quote's line items back to product identity (product id, else MPN, else\n" +
			"name) and aggregates quote count, quantity, and value per product. A cross-resource join no\n" +
			"single API call expresses. Run `sync` first.",
		Example: "  salesbuildr-cli product velocity --limit 20\n" +
			"  salesbuildr-cli product velocity --agent",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			if flagLimit <= 0 {
				return usageErr(fmt.Errorf("--limit must be > 0"))
			}
			quotes, err := loadQuotesHinted(cmd, flags, dbPath)
			if err != nil {
				return err
			}
			db, err := openNovelStore(cmd, flags, dbPath, "product")
			if err != nil {
				return err
			}
			products, err := loadProductsFromStore(db)
			_ = db.Close()
			if err != nil {
				return err
			}

			// Catalog lookup so velocity rows display canonical names even
			// when line items only carry an MPN or product id.
			catalogByKey := map[string]novelProduct{}
			for _, p := range products {
				for _, key := range p.productKeys() {
					if _, exists := catalogByKey[key]; !exists {
						catalogByKey[key] = p
					}
				}
			}

			type agg struct {
				name      string
				mpn       string
				productID string
				quotes    map[string]bool
				qty       float64
				value     float64
			}
			byKey := map[string]*agg{}
			// canonicalKey resolves a line item to one identity even when the
			// same physical product is id-linked on one quote and mpn- or
			// name-only on another: any candidate key that matches the
			// catalog collapses to the catalog product's canonical id key.
			canonicalKey := func(item novelQuoteItem) (string, novelProduct, bool) {
				for _, key := range item.candidateKeys() {
					if cp, ok := catalogByKey[key]; ok {
						return "id:" + cp.ID, cp, true
					}
				}
				return item.productKey(), novelProduct{}, false
			}
			for _, q := range quotes {
				if !q.DeletedAt.IsZero() {
					continue
				}
				for _, item := range q.Items {
					key, cp, inCatalog := canonicalKey(item)
					if key == "" {
						continue
					}
					a := byKey[key]
					if a == nil {
						a = &agg{name: item.Name, mpn: item.MPN, productID: item.ProductID, quotes: map[string]bool{}}
						if inCatalog {
							a.name = firstNonEmpty(cp.Name, a.name)
							a.mpn = firstNonEmpty(cp.MPN, a.mpn)
							a.productID = firstNonEmpty(cp.ID, a.productID)
						}
						byKey[key] = a
					}
					a.quotes[q.ID] = true
					a.qty += item.effectiveQuantity()
					a.value += item.Price * item.effectiveQuantity()
				}
			}

			report := productVelocityReport{QuotesScanned: len(quotes), Rows: make([]velocityProduct, 0, len(byKey))}
			for _, a := range byKey {
				report.Rows = append(report.Rows, velocityProduct{
					Product:    firstNonEmpty(a.name, a.mpn, a.productID),
					MPN:        a.mpn,
					ProductID:  a.productID,
					QuoteCount: len(a.quotes),
					TotalQty:   a.qty,
					TotalValue: a.value,
				})
			}
			sort.SliceStable(report.Rows, func(i, j int) bool {
				if report.Rows[i].TotalValue != report.Rows[j].TotalValue {
					return report.Rows[i].TotalValue > report.Rows[j].TotalValue
				}
				if report.Rows[i].QuoteCount != report.Rows[j].QuoteCount {
					return report.Rows[i].QuoteCount > report.Rows[j].QuoteCount
				}
				return report.Rows[i].Product < report.Rows[j].Product
			})
			if len(report.Rows) > flagLimit {
				report.Rows = report.Rows[:flagLimit]
			}
			if len(quotes) == 0 {
				report.Note = "no quotes in local store — run `salesbuildr-cli sync` first"
			} else if len(report.Rows) == 0 {
				report.Note = "synced quotes carry no line items — nothing to rank"
			}

			w := cmd.OutOrStdout()
			if flags.asJSON || flags.agent || !isTerminal(w) {
				return printJSONFiltered(w, report, flags)
			}
			if len(report.Rows) == 0 {
				fmt.Fprintln(w, report.Note)
				return nil
			}
			tw := newTabWriter(w)
			fmt.Fprintln(tw, "PRODUCT\tMPN\tQUOTES\tQTY\tVALUE")
			for _, r := range report.Rows {
				fmt.Fprintf(tw, "%s\t%s\t%d\t%.0f\t%.2f\n", truncate(r.Product, 38), orDash(r.MPN), r.QuoteCount, r.TotalQty, r.TotalValue)
			}
			return tw.Flush()
		},
	}
	cmd.Flags().IntVar(&flagLimit, "limit", 20, "Maximum products to return (ranked by quoted value)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (defaults to standard location)")
	return cmd
}
