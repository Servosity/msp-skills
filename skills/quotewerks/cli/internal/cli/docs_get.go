// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"quotewerks-pp-cli/internal/qw"
)

// lineTotals sums product lines. Extended values win; qty*unit is the
// fallback. A line whose price or cost is missing is counted, not guessed:
// the totals then exclude it and margin_pct is null.
type lineTotals struct {
	Lines            int      `json:"lines"`
	ProductLines     int      `json:"product_lines"`
	Price            float64  `json:"price"`
	Cost             float64  `json:"cost"`
	MarginPct        *float64 `json:"margin_pct"`
	UnknownCostLines int      `json:"unknown_cost_lines,omitempty"`
}

// lineAmount returns the extended amount for one money column pair, or false
// when it cannot be known: no extended value and no unit value, or a unit of
// pricing / unit of measure factor other than 1, whose exact effect on the
// extended amount is QuoteWerks' own calculation and is not reconstructed.
func lineAmount(l map[string]any, extended, unit string) (float64, bool) {
	if v, ok := qw.Num(l[extended]); ok {
		return v, true
	}
	if qw.HasPricingFactor(l) {
		return 0, false
	}
	u, okUnit := qw.Num(l[unit])
	qty, okQty := qw.EffectiveQty(l)
	if okUnit && okQty {
		return qty * u, true
	}
	return 0, false
}

func sumLines(lines []map[string]any) lineTotals {
	t := lineTotals{Lines: len(lines)}
	for _, l := range lines {
		if !qw.IsProductLine(l) {
			continue
		}
		t.ProductLines++
		counts, known := qw.TotalsContribution(l)
		if !known {
			t.UnknownCostLines++ // a recurring line may or may not be in the total
			continue
		}
		if !counts {
			continue
		}
		price, okPrice := lineAmount(l, "ExtendedPrice", "UnitPrice")
		cost, okCost := lineAmount(l, "ExtendedCost", "UnitCost")
		if !okPrice || !okCost {
			t.UnknownCostLines++
			continue
		}
		t.Price += price
		t.Cost += cost
	}
	if t.UnknownCostLines == 0 {
		if m, ok := qw.MarginPct(t.Price, t.Cost); ok { // from raw sums
			t.MarginPct = &m
		}
	}
	t.Price, t.Cost = qw.Round2(t.Price), qw.Round2(t.Cost)
	return t
}

// decorateLine adds the decoded line type and per-line margin.
func decorateLine(l map[string]any, allFields bool) map[string]any {
	out := l
	if !allFields {
		out = project(l, qwItemFields)
	}
	out["line_type"] = qw.LineTypeName(l["LineType"])
	if qw.IsProductLine(l) {
		if m := lineMargin(l); m != nil {
			out["margin_pct"] = *m
		}
	}
	return out
}

func newNovelDocsGetCmd(flags *rootFlags) *cobra.Command {
	var allFields bool
	cmd := &cobra.Command{
		Use:   "get [DocNo-or-RecGUID]",
		Short: "Get one quote, order or invoice with every line, line types decoded and per-line margin",
		Long: `Get one document header plus all of its DocumentItems rows. Lines are read by
filtering DocumentItems on DocRecGUID (the vendor docs disagree on include=Items vs
include=DocumentItems), sorted in display order, with LineType decoded and margin per
product line. Pass a document number (DocNo) or the header RecGUID.`,
		Example: strings.Trim(`
  quotewerks-cli docs get 1042 --agent
  quotewerks-cli docs get 1042 --agent --select header.DocNo,header.GrandTotal,lines.ManufacturerPartNumber,lines.UnitPrice`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live", "pp:happy-args": "doc=1042"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "docs get")
			}
			if len(args) == 0 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("a document number or RecGUID is required"))
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
			header := doc.Flat()
			if !allFields {
				header = project(header, qwHeaderFields)
			}
			if label := qw.QuoteValetLabel(header["QuoteValetStatusCode"]); label != "" {
				header["QuoteValetStatus"] = label
			}
			out := make([]map[string]any, 0, len(lines))
			for _, l := range lines {
				out = append(out, decorateLine(l, allFields))
			}
			view := map[string]any{"header": header, "lines": out, "totals": sumLines(lines)}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), view, flags)
			}
			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "%s %s  %s  %s\n", sc(header["DocType"]), sc(header["DocNo"]), sc(header["SoldToCompany"]), sc(header["DocStatus"]))
			table := make([]map[string]any, 0, len(out))
			for _, l := range out {
				table = append(table, project(l, []string{"line_type", "ManufacturerPartNumber", "Description", "QtyBase", "UnitCost", "UnitPrice", "margin_pct"}))
			}
			if err := printAutoTable(w, table); err != nil {
				return err
			}
			t := sumLines(lines)
			fmt.Fprintf(w, "product lines %d  price %.2f  cost %.2f", t.ProductLines, t.Price, t.Cost)
			if t.UnknownCostLines > 0 {
				fmt.Fprintf(w, "  (%d lines without price or cost excluded)", t.UnknownCostLines)
			}
			fmt.Fprintln(w)
			return nil
		},
	}
	cmd.Flags().BoolVar(&allFields, "all-fields", false, "Return every header and line column instead of the key fields")
	return cmd
}
