// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

// driftRow is one pricing-book price that has diverged from the master catalog.
type driftRow struct {
	Book      string `json:"book"`
	ProductID string `json:"productId"`
	Product   string `json:"product,omitempty"`
	// A side that carries no number is null, and so is its delta: an
	// absent catalog price or cost is unknown, not 0.
	BookPrice    *float64 `json:"bookPrice"`
	CatalogPrice *float64 `json:"catalogPrice"`
	PriceDelta   *float64 `json:"priceDelta"`
	BookCost     *float64 `json:"bookCost"`
	CatalogCost  *float64 `json:"catalogCost"`
	CostDelta    *float64 `json:"costDelta"`
}

type driftReport struct {
	Rows         []driftRow `json:"rows"`
	BooksScanned int        `json:"booksScanned"`
	// Incomparable counts book entries with neither a price nor a cost on
	// both sides (the catalog product or the book entry lacks them).
	Incomparable int    `json:"incomparable"`
	Note         string `json:"note,omitempty"`
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
					// Compare only numbers both sides carry (gnumOK presence):
					// an absent catalog cost is unknown, not a drift to 0.
					priceKnown := bp.HasPrice && cp.HasPrice
					costKnown := bp.HasCost && cp.HasCost
					if !priceKnown && !costKnown {
						report.Incomparable++
						continue
					}
					row := driftRow{Book: b.Name, ProductID: bp.ProductID, Product: cp.Name,
						BookPrice: presentNum(bp.Price, bp.HasPrice), CatalogPrice: presentNum(cp.Price, cp.HasPrice),
						BookCost: presentNum(bp.Cost, bp.HasCost), CatalogCost: presentNum(cp.Cost, cp.HasCost)}
					drifted := false
					if priceKnown {
						d := bp.Price - cp.Price
						row.PriceDelta = &d
						drifted = drifted || d != 0
					}
					if costKnown {
						d := bp.Cost - cp.Cost
						row.CostDelta = &d
						drifted = drifted || d != 0
					}
					if !drifted {
						continue
					}
					report.Rows = append(report.Rows, row)
				}
			}
			sort.Slice(report.Rows, func(i, j int) bool {
				return math.Abs(derefNum(report.Rows[i].PriceDelta)) > math.Abs(derefNum(report.Rows[j].PriceDelta))
			})
			if len(books) == 0 {
				report.Note = "no pricing books in local store — run `salesbuildr-cli sync` first"
			} else if len(report.Rows) == 0 {
				report.Note = "no pricing-book entries diverge from the catalog"
			}
			if report.Incomparable > 0 {
				report.Note = strings.TrimSpace(report.Note + fmt.Sprintf(" (%d book entries could not be compared: price and cost are missing on the book entry or the catalog product)", report.Incomparable))
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
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
					truncate(r.Book, 22), truncate(firstNonEmpty(r.Product, r.ProductID), 30),
					fmtNum(r.BookPrice, "%.2f"), fmtNum(r.CatalogPrice, "%.2f"), fmtNum(r.PriceDelta, "%+.2f"),
					fmtNum(r.BookCost, "%.2f"), fmtNum(r.CatalogCost, "%.2f"), fmtNum(r.CostDelta, "%+.2f"))
			}
			return tw.Flush()
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (defaults to standard location)")
	return cmd
}

// presentNum is v when the field was present, else nil (unknown).
func presentNum(v float64, present bool) *float64 {
	if !present {
		return nil
	}
	return &v
}

func derefNum(p *float64) float64 {
	if p == nil {
		return 0
	}
	return *p
}

// fmtNum prints a present number with format, an unknown one as "-".
func fmtNum(p *float64, format string) string {
	if p == nil {
		return "-"
	}
	return fmt.Sprintf(format, *p)
}
