// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source local

package cli

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"quotewerks-pp-cli/internal/qw"
)

type driftRow struct {
	DocNo          string   `json:"DocNo"`
	DocID          string   `json:"doc_id"`
	Customer       string   `json:"SoldToCompany,omitempty"`
	SalesRep       string   `json:"SalesRep,omitempty"`
	ItemID         string   `json:"item_id"`
	Part           string   `json:"part"`
	Qty            *float64 `json:"qty"`
	QuotedCost     *float64 `json:"quoted_cost"`
	CurrentCost    float64  `json:"current_cost"`
	ProductDB      string   `json:"product_db"`
	DeltaPct       *float64 `json:"delta_pct"`
	UnitPrice      float64  `json:"unit_price"`
	MarginQuoted   *float64 `json:"margin_at_quoted_cost_pct"`
	MarginCurrent  *float64 `json:"margin_at_current_cost_pct"`
	ExtendedImpact *float64 `json:"extended_cost_change"`
}

func newNovelCostDriftCmd(flags *rootFlags) *cobra.Command {
	var productDB, customer, rep, wonStatus, lostStatus, dbPath string
	var threshold float64
	var limit int

	cmd := &cobra.Command{
		Use:   "cost-drift",
		Short: "Find open-quote lines whose quoted cost no longer matches your product database",
		Long: `Join product lines on open quotes to the current product-database Cost (manufacturer
part number, preferring the same manufacturer, then vendor part number) and list lines
whose quoted UnitCost moved by at least --threshold percent, with margin at the quoted and
current cost and the line RecGUID to pass to 'items reprice'. Reads the local mirror; run
'mirror' first so quotes and product databases are current.

Use this command to find lines on open quotes whose quoted UnitCost no longer matches the
current product-database Cost. Do NOT use it to audit one quote before sending; use 'docs
lint' instead. Do NOT use it to change prices; use 'items reprice' instead.`,
		Example: strings.Trim(`
  quotewerks-cli cost-drift --threshold 2 --json
  quotewerks-cli cost-drift --rep "Derek Bryan" --product-db AllProducts --json`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "cost-drift")
			}
			if threshold < 0 {
				return usageErr(fmt.Errorf("--threshold must be zero or more"))
			}
			ctx := cmd.Context()
			view := map[string]any{"mirrored": false, "filters": filterEcho(cmd), "threshold_pct": threshold, "lines_checked": 0, "lines_matched": 0, "drifted": 0, "rows": make([]driftRow, 0)}
			db, ok, err := openMirror(ctx, cmd, flags, dbPath, qwResDocuments, qwResLineItems, qwResProducts)
			if err != nil {
				return err
			}
			if !ok {
				return noMirrorResult(cmd, flags, view)
			}
			defer db.Close()
			view["mirrored"] = true
			view["partial_resources"] = partialResources(db, qwResDocuments, qwResLineItems, qwResProducts)
			quotes, err := mirroredDocs(db, map[string]bool{"QUOTE": true}, time.Time{}, false)
			if err != nil {
				return err
			}
			cls := qw.NewClassifier(wonStatus, lostStatus)
			open := map[string]map[string]any{}
			for _, q := range quotes {
				if outcome, _ := cls.Classify(q); outcome != qw.OutcomeOpen {
					continue
				}
				if rep != "" && !strings.EqualFold(qw.Str(q["SalesRep"]), rep) {
					continue
				}
				if customer != "" && !strings.Contains(strings.ToLower(qw.Str(q["SoldToCompany"])), strings.ToLower(customer)) {
					continue
				}
				open[qw.Str(q["id"])] = q
			}
			idx, err := mirroredProductIndex(db, productDB)
			if err != nil {
				return err
			}
			if idx == nil {
				view["note"] = "no product databases in the mirror; run 'mirror --only products'"
				if wantsHumanTable(cmd.OutOrStdout(), flags) {
					fmt.Fprintf(cmd.OutOrStdout(), "No product databases in the mirror; run '%s mirror --only products' first.\n", qwCLIName)
					return nil
				}
				return printJSONFiltered(cmd.OutOrStdout(), view, flags)
			}
			items, err := loadMirror(db, qwResLineItems)
			if err != nil {
				return err
			}
			covered, err := coveredDocs(db)
			if err != nil {
				return err
			}
			uncovered := 0
			for id := range open {
				if !covered[id] {
					uncovered++
				}
			}
			view["quotes_without_mirrored_lines"] = uncovered
			if uncovered > 0 {
				view["note"] = fmt.Sprintf("%d open quotes have no mirrored lines and were not checked; run 'mirror' without --only documents", uncovered)
			}
			rows := make([]driftRow, 0)
			checked, matched, factorLines := 0, 0, 0
			for _, l := range items {
				doc, ok := open[qw.Str(l["DocRecGUID"])]
				if !ok || !qw.IsProductLine(l) {
					continue
				}
				if counts, known := qw.TotalsContribution(l); known && !counts {
					continue // an excluded line is not part of the quote
				}
				if qw.HasPricingFactor(l) {
					factorLines++ // priced per unit other than one: not comparable with catalog cost
					continue
				}
				checked++
				p, ok := idx.Match(qw.Str(l["Manufacturer"]), qw.Str(l["ManufacturerPartNumber"]), qw.Str(l["VendorPartNumber"]))
				if !ok || !p.HasCost {
					continue
				}
				matched++
				quoted, hasQuoted := qw.Num(l["UnitCost"])
				var delta *float64
				drifted := false
				if !hasQuoted || quoted == 0 {
					drifted = p.Cost > 0 // the line has no cost, the catalog has one
				} else {
					raw := (p.Cost - quoted) / quoted * 100
					d := qw.Round2(raw)
					delta = &d
					drifted = math.Abs(raw) >= threshold && p.Cost != quoted
				}
				if !drifted {
					continue
				}
				price := qw.NumOr(l["UnitPrice"], 0)
				var qtyPtr, impact, quotedPtr *float64
				var quotedForMargin any
				if hasQuoted {
					quotedPtr, quotedForMargin = &quoted, quoted
				}
				if q, ok := qw.EffectiveQty(l); ok {
					qtyPtr = &q
					if hasQuoted {
						v := qw.Round2((p.Cost - quoted) * q)
						impact = &v
					}
				}
				rows = append(rows, driftRow{
					DocNo: qw.Str(doc["DocNo"]), DocID: qw.Str(doc["id"]), Customer: qw.Str(doc["SoldToCompany"]), SalesRep: qw.Str(doc["SalesRep"]),
					ItemID: qw.Str(l["id"]), Part: qw.Str(l["ManufacturerPartNumber"]), Qty: qtyPtr,
					QuotedCost: quotedPtr, CurrentCost: p.Cost, ProductDB: p.DB, DeltaPct: delta, UnitPrice: price,
					MarginQuoted: marginOf(l["UnitPrice"], quotedForMargin), MarginCurrent: marginOf(l["UnitPrice"], p.Cost),
					ExtendedImpact: impact,
				})
			}
			// Largest cost impact first; a line without a quantity ranks by its unit change.
			weight := func(r driftRow) float64 {
				if r.ExtendedImpact != nil {
					return math.Abs(*r.ExtendedImpact)
				}
				if r.QuotedCost != nil {
					return math.Abs(r.CurrentCost - *r.QuotedCost)
				}
				return r.CurrentCost
			}
			sort.SliceStable(rows, func(i, j int) bool { return weight(rows[i]) > weight(rows[j]) })
			view["lines_checked"] = checked
			view["lines_skipped_pricing_factor"] = factorLines
			view["lines_matched"] = matched
			view["drifted"] = len(rows)
			if limit > 0 && len(rows) > limit {
				rows = rows[:limit]
			}
			view["rows"] = rows
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), view, flags)
			}
			if note, ok := view["note"].(string); ok {
				fmt.Fprintln(cmd.OutOrStdout(), "note: "+note)
			}
			if len(rows) == 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "No drift: %d open-quote lines checked, %d matched a product.\n", checked, matched)
				return nil
			}
			for _, r := range rows {
				quotedText := "      none"
				if r.QuotedCost != nil {
					quotedText = fmt.Sprintf("%10.2f", *r.QuotedCost)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%-10.10s %-20.20s %-18.18s quoted %s  now %10.2f  (%s)\n", sc(r.DocNo), sc(r.Customer), sc(r.Part), quotedText, r.CurrentCost, sc(r.ProductDB))
			}
			return nil
		},
	}
	cmd.Flags().Float64Var(&threshold, "threshold", 2, "Minimum cost change in percent to report")
	cmd.Flags().StringVar(&productDB, "product-db", "", "Compare against one product database only")
	cmd.Flags().StringVar(&customer, "customer", "", "Only quotes for customers whose name contains this text")
	cmd.Flags().StringVar(&rep, "rep", "", "Only quotes for this sales rep")
	cmd.Flags().StringVar(&wonStatus, "won-status", "", "DocStatus values that mean won (not open), comma-separated")
	cmd.Flags().StringVar(&lostStatus, "lost-status", "", "DocStatus values that mean lost (not open), comma-separated")
	cmd.Flags().IntVar(&limit, "limit", 100, "Maximum lines to return, largest cost impact first")
	cmd.Flags().StringVar(&dbPath, "db", "", "Local mirror database path")
	return cmd
}
