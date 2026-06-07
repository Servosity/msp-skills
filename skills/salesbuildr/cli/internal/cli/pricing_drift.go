// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"math"
	"sort"

	"github.com/spf13/cobra"
)

// driftRow is one pricing-book price that has diverged from the master catalog.
type driftRow struct {
	Book         string  `json:"book"`
	ProductID    string  `json:"productId"`
	Product      string  `json:"product,omitempty"`
	BookPrice    float64 `json:"bookPrice"`
	CatalogPrice float64 `json:"catalogPrice"`
	PriceDelta   float64 `json:"priceDelta"`
	BookCost     float64 `json:"bookCost"`
	CatalogCost  float64 `json:"catalogCost"`
	CostDelta    float64 `json:"costDelta"`
}

type driftReport struct {
	Rows         []driftRow `json:"rows"`
	BooksScanned int        `json:"booksScanned"`
	Note         string     `json:"note,omitempty"`
}

// newNovelPricingDriftCmd implements the "pricing drift" transcendence
// command: per-company pricing-book prices diverged from the master catalog.
// pp:data-source local
func newNovelPricingDriftCmd(flags *rootFlags) *cobra.Command {
	var dbPath string

	cmd := &cobra.Command{
		Use:   "drift",
		Short: "Flag pricing-book prices that have diverged from the master catalog price or cost",
		Long: "Joins every synced pricing book's product overrides against the master product catalog and\n" +
			"reports entries whose price or cost no longer matches. The two resources only co-exist in the\n" +
			"local store — the API has no comparison endpoint. Run `sync` first.",
		Example: "  salesbuildr-cli pricing drift\n" +
			"  salesbuildr-cli pricing drift --agent",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			books, err := loadPricingBooksHinted(cmd, flags, dbPath)
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

			catalog := map[string]novelProduct{}
			for _, p := range products {
				if p.ID != "" {
					catalog[p.ID] = p
				}
			}

			report := driftReport{Rows: make([]driftRow, 0), BooksScanned: len(books)}
			for _, b := range books {
				for _, bp := range b.Products {
					cp, ok := catalog[bp.ProductID]
					if !ok {
						continue
					}
					priceDelta := bp.Price - cp.Price
					costDelta := bp.Cost - cp.Cost
					if priceDelta == 0 && costDelta == 0 {
						continue
					}
					report.Rows = append(report.Rows, driftRow{
						Book:         b.Name,
						ProductID:    bp.ProductID,
						Product:      cp.Name,
						BookPrice:    bp.Price,
						CatalogPrice: cp.Price,
						PriceDelta:   priceDelta,
						BookCost:     bp.Cost,
						CatalogCost:  cp.Cost,
						CostDelta:    costDelta,
					})
				}
			}
			sort.Slice(report.Rows, func(i, j int) bool {
				return math.Abs(report.Rows[i].PriceDelta) > math.Abs(report.Rows[j].PriceDelta)
			})
			if len(books) == 0 {
				report.Note = "no pricing books in local store — run `salesbuildr-cli sync` first"
			} else if len(report.Rows) == 0 {
				report.Note = "no pricing-book entries diverge from the catalog"
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
			fmt.Fprintln(tw, "BOOK\tPRODUCT\tBOOK-PRICE\tCATALOG\tΔPRICE\tBOOK-COST\tCATALOG\tΔCOST")
			for _, r := range report.Rows {
				fmt.Fprintf(tw, "%s\t%s\t%.2f\t%.2f\t%+.2f\t%.2f\t%.2f\t%+.2f\n",
					truncate(r.Book, 22), truncate(firstNonEmpty(r.Product, r.ProductID), 30),
					r.BookPrice, r.CatalogPrice, r.PriceDelta, r.BookCost, r.CatalogCost, r.CostDelta)
			}
			return tw.Flush()
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (defaults to standard location)")
	return cmd
}
