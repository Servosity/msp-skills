// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"quotewerks-pp-cli/internal/qw"
)

type repriceChange struct {
	ItemID       string   `json:"item_id"`
	Part         string   `json:"part,omitempty"`
	Description  string   `json:"description,omitempty"`
	Qty          float64  `json:"qty"`
	OldCost      float64  `json:"old_cost"`
	NewCost      float64  `json:"new_cost"`
	OldPrice     float64  `json:"old_price"`
	NewPrice     float64  `json:"new_price"`
	OldMarginPct *float64 `json:"old_margin_pct"`
	NewMarginPct *float64 `json:"new_margin_pct"`
	ProductDB    string   `json:"product_db,omitempty"`
}

type repriceFailure struct {
	ItemID string `json:"item_id"`
	Error  string `json:"error"`
}

type repriceSkip struct {
	ItemID string `json:"item_id"`
	Part   string `json:"part,omitempty"`
	Reason string `json:"reason"`
}

// knownMargin is the margin percent, or nil when either amount is unknown.
func knownMargin(hasPrice bool, price float64, hasCost bool, cost float64) *float64 {
	if !hasPrice || !hasCost {
		return nil
	}
	if m, ok := qw.MarginPct(price, cost); ok {
		return &m
	}
	return nil
}

// repricePlan holds the pricing mode chosen by flags.
type repricePlan struct {
	price, margin, markup, cost                           float64
	setPrice, setMargin, setMarkup, setCost, fromProducts bool
}

func (p repricePlan) validate() error {
	priceModes := 0
	for _, b := range []bool{p.setPrice, p.setMargin, p.setMarkup} {
		if b {
			priceModes++
		}
	}
	if priceModes > 1 {
		return usageErr(fmt.Errorf("use only one of --price, --margin, --markup"))
	}
	if p.setCost && p.fromProducts {
		return usageErr(fmt.Errorf("use --cost or --from-products, not both"))
	}
	if priceModes == 0 && !p.setCost && !p.fromProducts {
		return usageErr(fmt.Errorf("nothing to change: pass --price, --margin, --markup, --cost or --from-products"))
	}
	if (p.setPrice && p.price <= 0) || (p.setCost && p.cost <= 0) {
		return usageErr(fmt.Errorf("--price and --cost must be greater than zero"))
	}
	return nil
}

// apply computes the new cost and price for one line. prod is the product
// match (only used with --from-products). skip explains why a line cannot be
// repriced: a margin or markup needs a real cost, and a cost of zero or a
// missing cost would otherwise produce a selling price of zero.
func (p repricePlan) apply(oldCost float64, hasCost bool, oldPrice float64, prod *qw.Product) (newCost, newPrice float64, skip string, err error) {
	newCost, newPrice = oldCost, oldPrice
	switch {
	case p.setCost:
		newCost, hasCost = p.cost, true
	case p.fromProducts && prod != nil && prod.HasCost:
		newCost, hasCost = prod.Cost, true
	}
	if (p.setMargin || p.setMarkup) && (!hasCost || newCost <= 0) {
		return 0, 0, "line has no unit cost to price from; pass --cost or --from-products", nil
	}
	// Supplied and untouched amounts are kept exactly; only a price computed
	// from a margin or markup is rounded (to cents, by PriceForMargin/Markup).
	switch {
	case p.setPrice:
		newPrice = p.price
	case p.setMargin:
		if newPrice, err = qw.PriceForMargin(newCost, p.margin); err != nil {
			return 0, 0, "", usageErr(err)
		}
	case p.setMarkup:
		newPrice = qw.PriceForMarkup(newCost, p.markup)
	case p.fromProducts && prod != nil && prod.HasPrice:
		newPrice = prod.Price
	}
	if (p.setMargin || p.setMarkup) && newPrice <= 0 {
		return 0, 0, "computed price rounds to zero or less; use --price", nil
	}
	return newCost, newPrice, "", nil
}

func newNovelItemsRepriceCmd(flags *rootFlags) *cobra.Command {
	var parts, items, productDB string
	var allLines, confirm bool
	var plan repricePlan

	cmd := &cobra.Command{
		Use:   "reprice [DocNo-or-RecGUID]",
		Short: "Change cost or price on quote lines with an old-vs-new margin diff; previews unless --confirm",
		Long: `Reprice product lines on one document. Choose lines with --part, --item or
--all-lines; set the new cost with --cost or --from-products (current product-database
cost); set the new price with --price, --margin (target margin percent on the new cost),
--markup, or --from-products (product-database price). Every changed line shows old and
new cost, price and margin. Nothing is written until --confirm; then each line is PATCHed
and any per-line failure is reported without hiding the lines that succeeded.

Use this command to change cost or price on specific lines of one document. Do NOT use it
to find which open quotes drifted; use 'cost-drift' instead.`,
		Example: strings.Trim(`
  quotewerks-cli items reprice 1042 --part 20UES0X100 --margin 18 --json
  quotewerks-cli items reprice 1042 --all-lines --from-products --json
  quotewerks-cli items reprice 1042 --item 7C1E2B9A-0D4F-4A55-9E21-3B8F6A7D1C02 --price 1099 --confirm`, "\n"),
		Annotations: map[string]string{"pp:data-source": "live", "mcp:read-only-unless-flags": "confirm", "pp:happy-args": "doc=1042;--all-lines;--margin=18"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "items reprice")
			}
			if len(args) == 0 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("a document number or RecGUID is required"))
			}
			plan.setPrice = cmd.Flags().Changed("price")
			plan.setMargin = cmd.Flags().Changed("margin")
			plan.setMarkup = cmd.Flags().Changed("markup")
			plan.setCost = cmd.Flags().Changed("cost")
			if err := plan.validate(); err != nil {
				return err
			}
			partSet, itemSet := partCSVSet(parts), upperCSVSet(items)
			if !allLines && len(partSet) == 0 && len(itemSet) == 0 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("choose lines with --part, --item or --all-lines"))
			}
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			doc, err := resolveDoc(ctx, c, args[0])
			if err != nil {
				return err
			}
			lines, err := fetchItems(ctx, c, doc.ID)
			if err != nil {
				return err
			}
			selected := make([]map[string]any, 0)
			for _, l := range lines {
				if !qw.IsProductLine(l) {
					continue
				}
				if allLines || itemSet[strings.ToUpper(qw.Str(l["id"]))] || partSet[qw.NormalizePart(qw.Str(l["ManufacturerPartNumber"]))] {
					selected = append(selected, l)
				}
			}
			if len(selected) == 0 {
				return notFoundErr(fmt.Errorf("no product lines on document %s match the selection", qw.Str(doc.Attributes["DocNo"])))
			}
			var idx *qw.ProductIndex
			if plan.fromProducts {
				tables, err := productTables(ctx, c, productDB)
				if err != nil {
					return err
				}
				found, err := catalogLookup(ctx, c, tables, lineParts(selected))
				if err != nil {
					return err
				}
				idx = qw.NewProductIndex(found)
			}
			changes := make([]repriceChange, 0)
			skipped := make([]repriceSkip, 0)
			notInProducts := make([]string, 0)
			unchanged := 0
			for _, l := range selected {
				oldCost, hasCost := qw.Num(l["UnitCost"])
				oldPrice, hasPrice := qw.Num(l["UnitPrice"])
				if qw.HasPricingFactor(l) {
					// Unit price and unit cost are on different bases here; any
					// computed or catalog price would be wrong by the factor.
					skipped = append(skipped, repriceSkip{ItemID: qw.Str(l["id"]), Part: qw.Str(l["ManufacturerPartNumber"]), Reason: "line is priced per unit of pricing other than one; reprice it in QuoteWerks"})
					continue
				}
				var prod *qw.Product
				if idx != nil {
					if p, ok := idx.Match(qw.Str(l["Manufacturer"]), qw.Str(l["ManufacturerPartNumber"]), qw.Str(l["VendorPartNumber"])); ok {
						prod = &p
					} else {
						notInProducts = append(notInProducts, qw.Str(l["ManufacturerPartNumber"]))
					}
				}
				newCost, newPrice, skip, err := plan.apply(oldCost, hasCost, oldPrice, prod)
				if err != nil {
					return err
				}
				if skip != "" {
					skipped = append(skipped, repriceSkip{ItemID: qw.Str(l["id"]), Part: qw.Str(l["ManufacturerPartNumber"]), Reason: skip})
					continue
				}
				if newCost == oldCost && newPrice == oldPrice {
					unchanged++
					continue
				}
				ch := repriceChange{
					ItemID: qw.Str(l["id"]), Part: qw.Str(l["ManufacturerPartNumber"]), Description: qw.Str(l["Description"]),
					Qty: qw.NumOr(l["QtyBase"], 0), OldCost: oldCost, NewCost: newCost, OldPrice: oldPrice, NewPrice: newPrice,
					OldMarginPct: knownMargin(hasPrice, oldPrice, hasCost, oldCost),
					NewMarginPct: knownMargin(hasPrice || plan.setPrice || plan.setMargin || plan.setMarkup || (prod != nil && prod.HasPrice), newPrice,
						hasCost || plan.setCost || (prod != nil && prod.HasCost), newCost),
				}
				if prod != nil {
					ch.ProductDB = prod.DB
				}
				changes = append(changes, ch)
			}
			view := map[string]any{
				"document":  project(doc.Flat(), []string{"id", "DocNo", "DocType", "SoldToCompany"}),
				"changes":   changes,
				"unchanged": unchanged,
				"skipped":   skipped,
			}
			if len(notInProducts) > 0 {
				view["not_in_product_databases"] = notInProducts
			}
			if _, err := json.Marshal(view); err != nil {
				return usageErr(fmt.Errorf("the repricing plan cannot be encoded (check the amounts): %w", err))
			}
			if !confirm {
				return printWriteResult(cmd.OutOrStdout(), writeModeEnvelope(false, view), flags)
			}
			failures := make([]repriceFailure, 0)
			applied := 0
			for _, ch := range changes {
				attrs := map[string]any{}
				if ch.NewCost != ch.OldCost {
					attrs["UnitCost"] = ch.NewCost
				}
				if ch.NewPrice != ch.OldPrice {
					attrs["UnitPrice"] = ch.NewPrice
				}
				if ch.ItemID == "" {
					failures = append(failures, repriceFailure{Error: "line has no RecGUID"})
					continue
				}
				if _, err := qwWrite(ctx, c, "PATCH", qw.TableItems, ch.ItemID, attrs); err != nil {
					failures = append(failures, repriceFailure{ItemID: ch.ItemID, Error: err.Error()})
					continue
				}
				applied++
			}
			view["applied"] = applied
			view["update_failures"] = failures
			if err := printWriteResult(cmd.OutOrStdout(), writeModeEnvelope(true, view), flags); err != nil {
				return err
			}
			if len(failures) > 0 {
				// The summary rides in the error too: MCP clients only see stderr on a non-zero exit.
				ids := make([]string, 0, len(failures))
				for _, f := range failures {
					ids = append(ids, f.ItemID)
				}
				return partialFailureErr(fmt.Errorf("%d of %d line updates failed (applied %d); failed item ids: %s; re-run for just those lines with --item", len(failures), len(changes), applied, strings.Join(ids, ",")))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&parts, "part", "", "Reprice lines with these manufacturer part numbers (comma-separated)")
	cmd.Flags().StringVar(&items, "item", "", "Reprice these line RecGUIDs (comma-separated, from 'docs get')")
	cmd.Flags().BoolVar(&allLines, "all-lines", false, "Reprice every product line on the document")
	cmd.Flags().Float64Var(&plan.price, "price", 0, "New unit sell price")
	cmd.Flags().Float64Var(&plan.margin, "margin", 0, "Target margin percent; price = cost / (1 - margin/100)")
	cmd.Flags().Float64Var(&plan.markup, "markup", 0, "Markup percent over cost; price = cost * (1 + markup/100)")
	cmd.Flags().Float64Var(&plan.cost, "cost", 0, "New unit cost")
	cmd.Flags().BoolVar(&plan.fromProducts, "from-products", false, "Take cost (and price, unless --price/--margin/--markup is set) from the current product database")
	cmd.Flags().StringVar(&productDB, "product-db", "", "With --from-products, use one product database only")
	cmd.Flags().BoolVar(&confirm, "confirm", false, "Apply the changes; without it the command only previews them")
	return cmd
}
