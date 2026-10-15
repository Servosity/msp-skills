// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source local

package cli

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"quotewerks-pp-cli/internal/qw"
)

// marginRow sums only documents or lines whose revenue and cost are both
// known. Anything else is counted in Unknown and excluded, and margin_pct is
// null for the group, so a missing cost never reads as pure profit.
type marginRow struct {
	Key       string   `json:"key"`
	Documents int      `json:"documents,omitempty"`
	Lines     int      `json:"lines,omitempty"`
	Revenue   float64  `json:"revenue"`
	Profit    float64  `json:"profit"`
	MarginPct *float64 `json:"margin_pct"`
	Unknown   int      `json:"unknown_amounts,omitempty"`
}

type marginView struct {
	Mirrored  bool              `json:"mirrored"`
	Filters   map[string]string `json:"filters"`
	By        string            `json:"by"`
	Types     []string          `json:"types"`
	Rows      []marginRow       `json:"rows"`
	Revenue   float64           `json:"revenue"`
	Profit    float64           `json:"profit"`
	MarginPct *float64          `json:"margin_pct"`
	Unknown   int               `json:"unknown_amounts,omitempty"`
	NoLines   int               `json:"documents_without_mirrored_lines,omitempty"`
	Partial   []string          `json:"partial_resources,omitempty"`
}

func newNovelMarginCmd(flags *rootFlags) *cobra.Command {
	var by, types, since, dbPath string
	var limit int

	cmd := &cobra.Command{
		Use:   "margin",
		Short: "Profit and margin percent by customer, rep, month or manufacturer from the local mirror",
		Long: `Group mirrored documents and report revenue (SubTotal), profit (ProfitAmount) and
margin percent. --by manufacturer works at line level: product lines' ExtendedPrice minus
ExtendedCost. Defaults to ORDER documents so a quote that became an order is not counted
twice. Reads the local mirror; run 'mirror' first.

Use this command for profit and margin percent on documents grouped by customer, rep,
month or manufacturer. Do NOT use it for quote win and loss rates; use 'winrate' instead.`,
		Example: strings.Trim(`
  quotewerks-cli margin --by rep --since 90d --json
  quotewerks-cli margin --by customer --type ORDER,INVOICE --limit 10`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "margin")
			}
			if err := validateBy(by, "customer", "rep", "month", "manufacturer"); err != nil {
				return err
			}
			cutoff, hasCutoff, err := sinceCutoff(since, time.Now())
			if err != nil {
				return err
			}
			typeSet := upperCSVSet(types)
			if err := validateDocTypes(typeSet, "--type"); err != nil {
				return err
			}
			view := marginView{By: by, Filters: filterEcho(cmd), Rows: make([]marginRow, 0), Types: make([]string, 0)}
			for t := range typeSet {
				view.Types = append(view.Types, t)
			}
			sort.Strings(view.Types)
			ctx := cmd.Context()
			resources := []string{qwResDocuments}
			if by == "manufacturer" {
				resources = append(resources, qwResLineItems)
			}
			db, ok, err := openMirror(ctx, cmd, flags, dbPath, resources...)
			if err != nil {
				return err
			}
			if !ok {
				return noMirrorResult(cmd, flags, view)
			}
			defer db.Close()
			view.Mirrored = true
			view.Partial = partialResources(db, resources...)
			docs, err := mirroredDocs(db, typeSet, cutoff, hasCutoff)
			if err != nil {
				return err
			}
			// Lines are only needed for manufacturer grain. Header grain uses the
			// server's ProfitAmount; rebuilding it from lines would miss percent
			// discounts and charges, so a missing ProfitAmount counts as unknown.
			needLines := by == "manufacturer"
			linesByDoc := map[string][]map[string]any{}
			covered := map[string]bool{}
			if needLines {
				if covered, err = coveredDocs(db); err != nil {
					return err
				}
				items, err := loadMirror(db, qwResLineItems)
				if err != nil {
					return err
				}
				for _, it := range items {
					if qw.IsProductLine(it) {
						id := qw.Str(it["DocRecGUID"])
						linesByDoc[id] = append(linesByDoc[id], it)
					}
				}
			}
			groups := map[string]*marginRow{}
			get := func(k string) *marginRow {
				if groups[k] == nil {
					groups[k] = &marginRow{Key: k}
				}
				return groups[k]
			}
			for _, d := range docs {
				id := qw.Str(d["id"])
				if by == "manufacturer" {
					if !covered[id] {
						view.NoLines++
						continue
					}
					for _, l := range linesByDoc[id] {
						k := qw.Str(l["Manufacturer"])
						if k == "" {
							k = "(none)"
						}
						r := get(k)
						counts, known := qw.TotalsContribution(l)
						if !known {
							r.Unknown++
							continue
						}
						if !counts {
							continue
						}
						rev, okRev := lineAmount(l, "ExtendedPrice", "UnitPrice")
						cost, okCost := lineAmount(l, "ExtendedCost", "UnitCost")
						if !okRev || !okCost {
							r.Unknown++
							continue
						}
						r.Lines++
						r.Revenue += rev
						r.Profit += rev - cost
					}
					continue
				}
				r := get(groupKey(d, by))
				// Revenue is SubTotal only; rebuilding it from GrandTotal would
				// need tax, shipping and every other non-subtotal component.
				rev, okRev := qw.Num(d["SubTotal"])
				profit, okProfit := qw.Num(d["ProfitAmount"])
				if !okRev || !okProfit {
					r.Unknown++
					continue
				}
				r.Documents++
				r.Revenue += rev
				r.Profit += profit
			}
			// Sum and compute margins on raw amounts; round only what is shown.
			for _, r := range groups {
				view.Revenue += r.Revenue
				view.Profit += r.Profit
				view.Unknown += r.Unknown
				if r.Unknown == 0 {
					r.MarginPct = pctPtr(r.Profit, r.Revenue)
				}
				r.Revenue, r.Profit = qw.Round2(r.Revenue), qw.Round2(r.Profit)
				view.Rows = append(view.Rows, *r)
			}
			sort.Slice(view.Rows, func(i, j int) bool {
				if view.Rows[i].Profit != view.Rows[j].Profit {
					return view.Rows[i].Profit > view.Rows[j].Profit
				}
				return view.Rows[i].Key < view.Rows[j].Key
			})
			if limit > 0 && len(view.Rows) > limit {
				view.Rows = view.Rows[:limit]
			}
			if view.Unknown == 0 && view.NoLines == 0 {
				view.MarginPct = pctPtr(view.Profit, view.Revenue)
			}
			view.Revenue, view.Profit = qw.Round2(view.Revenue), qw.Round2(view.Profit)
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), view, flags)
			}
			if view.NoLines > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "note: %d documents have no mirrored lines; run 'mirror' without --only documents\n", view.NoLines)
			}
			if len(view.Rows) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No mirrored documents match.")
				return nil
			}
			pct := func(p *float64) string {
				if p == nil {
					return "n/a"
				}
				return fmt.Sprintf("%.2f%%", *p)
			}
			for _, r := range view.Rows {
				fmt.Fprintf(cmd.OutOrStdout(), "%-32.32s revenue %12.2f  profit %12.2f  margin %s\n", sc(r.Key), r.Revenue, r.Profit, pct(r.MarginPct))
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%-32.32s revenue %12.2f  profit %12.2f  margin %s\n", "TOTAL ("+strings.Join(view.Types, ",")+")", view.Revenue, view.Profit, pct(view.MarginPct))
			if view.Unknown > 0 || view.NoLines > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "note: %d documents or lines had no known cost or price and %d documents had no mirrored lines; they are left out and margins show n/a\n", view.Unknown, view.NoLines)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&by, "by", "customer", "Group by: customer, rep, month or manufacturer")
	cmd.Flags().StringVar(&types, "type", "ORDER", "Document types to include, comma-separated (QUOTE, ORDER, INVOICE)")
	cmd.Flags().StringVar(&since, "since", "", "Only documents dated within this window, e.g. 90d")
	cmd.Flags().IntVar(&limit, "limit", 25, "Maximum groups to return, highest profit first")
	cmd.Flags().StringVar(&dbPath, "db", "", "Local mirror database path")
	return cmd
}
