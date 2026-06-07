// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"
)

// thinLine is one under-priced quote line item.
type thinLine struct {
	QuoteID     string  `json:"quoteId"`
	QuoteNumber string  `json:"quoteNumber"`
	Company     string  `json:"company"`
	Item        string  `json:"item"`
	MPN         string  `json:"mpn,omitempty"`
	Quantity    float64 `json:"quantity"`
	Price       float64 `json:"price"`
	Cost        float64 `json:"cost"`
	MarkupPct   float64 `json:"markupPct"`
}

type thinLineReport struct {
	Lines    []thinLine `json:"lines"`
	FloorPct float64    `json:"floorPct"`
	Note     string     `json:"note,omitempty"`
}

// newNovelQuoteThinCmd implements the "quote thin" transcendence command: line
// items across ALL open quotes whose markup falls below a floor.
// pp:data-source local
func newNovelQuoteThinCmd(flags *rootFlags) *cobra.Command {
	var flagFloor float64
	var dbPath string

	cmd := &cobra.Command{
		Use:   "thin",
		Short: "Find every quote line item across all open quotes whose markup falls below a floor",
		Long: "Use this command to find quote line items priced below a markup floor across all open quotes.\n" +
			"Do NOT use this command to find aging quotes or at-risk revenue totals; use 'quote stale' instead.\n\n" +
			"Scans the line items of every open quote in the local store and reports those whose markup\n" +
			"percentage (explicit, or computed from price/cost) is below --floor. The API returns items one\n" +
			"quote at a time; the all-quotes margin scan only exists locally. Run `sync` first.",
		Example: "  salesbuildr-cli quote thin --floor 20\n" +
			"  salesbuildr-cli quote thin --floor 15 --agent",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			if flagFloor < 0 {
				return usageErr(fmt.Errorf("--floor must be >= 0"))
			}
			quotes, err := loadQuotesHinted(cmd, flags, dbPath)
			if err != nil {
				return err
			}

			report := thinLineReport{Lines: make([]thinLine, 0), FloorPct: flagFloor}
			for _, q := range quotes {
				if !q.isOpen() {
					continue
				}
				for _, item := range q.Items {
					markup, ok := item.effectiveMarkup()
					if !ok || markup >= flagFloor {
						continue
					}
					report.Lines = append(report.Lines, thinLine{
						QuoteID:     q.ID,
						QuoteNumber: q.Number,
						Company:     q.Company,
						Item:        item.Name,
						MPN:         item.MPN,
						Quantity:    item.effectiveQuantity(),
						Price:       item.Price,
						Cost:        item.Cost,
						MarkupPct:   markup,
					})
				}
			}
			sort.Slice(report.Lines, func(i, j int) bool { return report.Lines[i].MarkupPct < report.Lines[j].MarkupPct })
			if len(quotes) == 0 {
				report.Note = "no quotes in local store — run `salesbuildr-cli sync` first"
			} else if len(report.Lines) == 0 {
				report.Note = fmt.Sprintf("no open-quote line items below %.1f%% markup", flagFloor)
			}

			w := cmd.OutOrStdout()
			if flags.asJSON || flags.agent || !isTerminal(w) {
				return printJSONFiltered(w, report, flags)
			}
			if len(report.Lines) == 0 {
				fmt.Fprintln(w, report.Note)
				return nil
			}
			tw := newTabWriter(w)
			fmt.Fprintln(tw, "QUOTE\tCOMPANY\tITEM\tQTY\tPRICE\tCOST\tMARKUP%")
			for _, l := range report.Lines {
				fmt.Fprintf(tw, "%s\t%s\t%s\t%.0f\t%.2f\t%.2f\t%.1f\n",
					orDash(l.QuoteNumber), truncate(l.Company, 24), truncate(l.Item, 32), l.Quantity, l.Price, l.Cost, l.MarkupPct)
			}
			return tw.Flush()
		},
	}
	cmd.Flags().Float64Var(&flagFloor, "floor", 20, "Markup floor percentage; lines below this are reported")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (defaults to standard location)")
	return cmd
}
