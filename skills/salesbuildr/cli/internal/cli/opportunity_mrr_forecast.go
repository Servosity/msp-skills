// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"
)

// forecastOpp is one open opportunity's probability-weighted recurring value.
type forecastOpp struct {
	ID                 string  `json:"id"`
	Name               string  `json:"name"`
	Company            string  `json:"company,omitempty"`
	Stage              string  `json:"stage,omitempty"`
	Probability        float64 `json:"probability"`
	MonthlyRevenue     float64 `json:"monthlyRevenue"`
	WeightedMRR        float64 `json:"weightedMrr"`
	ChurnAdjustedMRR   float64 `json:"churnAdjustedMrr"`
	WeightedMonthlyGP  float64 `json:"weightedMonthlyProfit"`
	WeightedOneTimeRev float64 `json:"weightedOneTimeRevenue"`
}

type forecastReport struct {
	OpenOpportunities  int           `json:"openOpportunities"`
	WeightedMRR        float64       `json:"weightedMrr"`
	ChurnAdjustedMRR   float64       `json:"churnAdjustedMrr"`
	WeightedMonthlyGP  float64       `json:"weightedMonthlyProfit"`
	WeightedOneTimeRev float64       `json:"weightedOneTimeRevenue"`
	Top                []forecastOpp `json:"top"`
	Note               string        `json:"note,omitempty"`
}

// newNovelOpportunityMrrForecastCmd implements the "opportunity mrr-forecast"
// transcendence command: probability-weighted, churn-adjusted recurring revenue.
// pp:data-source local
func newNovelOpportunityMrrForecastCmd(flags *rootFlags) *cobra.Command {
	var flagLimit int
	var dbPath string

	cmd := &cobra.Command{
		Use:   "mrr-forecast",
		Short: "Weight open-pipeline monthly revenue and profit by probability, churn-adjusted",
		Long: "Sums probability-weighted monthlyRevenue and monthlyProfit across every open opportunity in\n" +
			"the local store, discounting by each deal's monthlyChurn. One-time revenue is weighted\n" +
			"separately. The API exposes per-deal fields, never the forecast. Run `sync` first.",
		Example: "  salesbuildr-cli opportunity mrr-forecast\n" +
			"  salesbuildr-cli opportunity mrr-forecast --limit 5 --agent",
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
			opps, err := loadOpportunitiesHinted(cmd, flags, dbPath)
			if err != nil {
				return err
			}

			report := forecastReport{Top: make([]forecastOpp, 0)}
			for _, o := range opps {
				if o.isClosed() {
					continue
				}
				report.OpenOpportunities++
				p := o.Probability / 100
				if p < 0 {
					p = 0
				} else if p > 1 {
					p = 1
				}
				churn := o.MonthlyChurn / 100
				if churn < 0 {
					churn = 0
				} else if churn > 1 {
					churn = 1
				}
				f := forecastOpp{
					ID:                 o.ID,
					Name:               o.Name,
					Company:            o.Company,
					Stage:              o.Stage,
					Probability:        o.Probability,
					MonthlyRevenue:     o.MonthlyRevenue,
					WeightedMRR:        o.MonthlyRevenue * p,
					WeightedMonthlyGP:  o.MonthlyProfit * p,
					WeightedOneTimeRev: o.OnetimeRevenue * p,
				}
				f.ChurnAdjustedMRR = f.WeightedMRR * (1 - churn)
				report.WeightedMRR += f.WeightedMRR
				report.ChurnAdjustedMRR += f.ChurnAdjustedMRR
				report.WeightedMonthlyGP += f.WeightedMonthlyGP
				report.WeightedOneTimeRev += f.WeightedOneTimeRev
				report.Top = append(report.Top, f)
			}
			sort.SliceStable(report.Top, func(i, j int) bool {
				if report.Top[i].WeightedMRR != report.Top[j].WeightedMRR {
					return report.Top[i].WeightedMRR > report.Top[j].WeightedMRR
				}
				return report.Top[i].ID < report.Top[j].ID
			})
			if len(report.Top) > flagLimit {
				report.Top = report.Top[:flagLimit]
			}
			if len(opps) == 0 {
				report.Note = "no opportunities in local store — run `salesbuildr-cli sync` first"
			} else if report.OpenOpportunities == 0 {
				report.Note = "no open opportunities — the forecast covers open pipeline only"
			}

			w := cmd.OutOrStdout()
			if flags.asJSON || flags.agent || !isTerminal(w) {
				return printJSONFiltered(w, report, flags)
			}
			if report.OpenOpportunities == 0 {
				fmt.Fprintln(w, report.Note)
				return nil
			}
			fmt.Fprintf(w, "Open pipeline forecast (%d open deals)\n", report.OpenOpportunities)
			fmt.Fprintf(w, "  weighted MRR:            %.2f\n", report.WeightedMRR)
			fmt.Fprintf(w, "  churn-adjusted MRR:      %.2f\n", report.ChurnAdjustedMRR)
			fmt.Fprintf(w, "  weighted monthly profit: %.2f\n", report.WeightedMonthlyGP)
			fmt.Fprintf(w, "  weighted one-time rev:   %.2f\n\n", report.WeightedOneTimeRev)
			tw := newTabWriter(w)
			fmt.Fprintln(tw, "DEAL\tCOMPANY\tSTAGE\tPROB%\tMRR\tWEIGHTED")
			for _, f := range report.Top {
				fmt.Fprintf(tw, "%s\t%s\t%s\t%.0f\t%.2f\t%.2f\n",
					truncate(firstNonEmpty(f.Name, f.ID), 28), truncate(f.Company, 22), truncate(f.Stage, 18), f.Probability, f.MonthlyRevenue, f.WeightedMRR)
			}
			return tw.Flush()
		},
	}
	cmd.Flags().IntVar(&flagLimit, "limit", 10, "Maximum top deals to list (totals always cover all open deals)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (defaults to standard location)")
	return cmd
}
