// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// `compare` — month-over-month (or any two periods) delta by service.
//
// pp:client-call
package cli

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"

	"aws-billing-pp-cli/internal/awsx"
)

type compareRow struct {
	Service  string  `json:"service"`
	FromUSD  float64 `json:"from_usd"`
	ToUSD    float64 `json:"to_usd"`
	DeltaUSD float64 `json:"delta_usd"`
	DeltaPct float64 `json:"delta_pct"`
}

type compareResult struct {
	From      string       `json:"from"`
	To        string       `json:"to"`
	Source    string       `json:"source"`
	FromTotal float64      `json:"from_total_usd"`
	ToTotal   float64      `json:"to_total_usd"`
	DeltaPct  float64      `json:"delta_pct"`
	Rows      []compareRow `json:"rows"`
}

func newCompareCmd(flags *rootFlags) *cobra.Command {
	var fromPeriod, toPeriod, dbPath, profile, region string
	var limit int

	cmd := &cobra.Command{
		Use:   "compare",
		Short: "Compare two periods and rank what changed by service",
		Long: `Compare spend across two periods and rank services by how much they moved.
Defaults to last-month vs this-month — the "why did my bill change?" answer.`,
		Example: `  # Last month vs this month
  aws-billing-cli compare

  # Two explicit months
  aws-billing-cli compare --from last-month --to this-month --agent`,
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			fromPR, err := resolvePeriod(fromPeriod)
			if err != nil {
				return usageErr(err)
			}
			toPR, err := resolvePeriod(toPeriod)
			if err != nil {
				return usageErr(err)
			}
			o := awsReadOptsFromFlags(flags, dbPath, profile, region)
			fromLines, src, err := canonicalCostLines(cmd.Context(), o, fromPR)
			if err != nil {
				return err
			}
			toLines, _, err := canonicalCostLines(cmd.Context(), o, toPR)
			if err != nil {
				return err
			}
			fromBySvc, fromTotal := sumByService(fromLines)
			toBySvc, toTotal := sumByService(toLines)

			seen := map[string]bool{}
			var rows []compareRow
			for svc := range fromBySvc {
				seen[svc] = true
			}
			for svc := range toBySvc {
				seen[svc] = true
			}
			for svc := range seen {
				f := fromBySvc[svc]
				t := toBySvc[svc]
				rows = append(rows, compareRow{Service: svc, FromUSD: round2f(f), ToUSD: round2f(t), DeltaUSD: round2f(t - f), DeltaPct: pctDelta(f, t)})
			}
			sort.Slice(rows, func(i, j int) bool {
				return absf(rows[i].DeltaUSD) > absf(rows[j].DeltaUSD)
			})
			if limit > 0 && len(rows) > limit {
				rows = rows[:limit]
			}
			res := compareResult{From: fromPR.Label, To: toPR.Label, Source: src, FromTotal: round2f(fromTotal), ToTotal: round2f(toTotal), DeltaPct: pctDelta(fromTotal, toTotal), Rows: rows}

			if flags.asJSON {
				return flags.printJSON(cmd, res)
			}
			w := cmd.OutOrStdout()
			snark := flags.snark && !flags.asJSON
			fmt.Fprintf(w, "Compare %s -> %s (source: %s)\n", res.From, res.To, res.Source)
			if len(rows) == 0 {
				fmt.Fprintln(w, "  no cost data for these periods (run 'sync', or check 'doctor')")
				return nil
			}
			fmt.Fprintf(w, "  %-46s %12s %12s %10s\n", "SERVICE", "FROM", "TO", "Δ$")
			for _, r := range rows {
				fmt.Fprintf(w, "  %-46s %12.2f %12.2f %+10.2f\n", truncate(r.Service, 46), r.FromUSD, r.ToUSD, r.DeltaUSD)
			}
			fmt.Fprintf(w, "  %-46s %12.2f %12.2f %+9.1f%%\n", "TOTAL", res.FromTotal, res.ToTotal, res.DeltaPct)
			if res.DeltaPct >= 10 {
				fmt.Fprint(w, snarkf(snark, "big-jump", int(res.ToTotal)))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&fromPeriod, "from", "last-month", "Baseline period (preset or YYYY-MM-DD:YYYY-MM-DD)")
	cmd.Flags().StringVar(&toPeriod, "to", "this-month", "Comparison period (preset or YYYY-MM-DD:YYYY-MM-DD)")
	cmd.Flags().IntVar(&limit, "limit", 0, "Show only the top N movers (0 = all)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: per-user cache)")
	cmd.Flags().StringVar(&profile, "profile-aws", "", "AWS shared-config profile for a live call")
	cmd.Flags().StringVar(&region, "region", "", "AWS region for a live call")
	return cmd
}

func sumByService(lines []awsx.CostLine) (map[string]float64, float64) {
	m := map[string]float64{}
	var total float64
	for _, l := range lines {
		svc := l.Service
		if svc == "" {
			svc = "(unattributed)"
		}
		m[svc] += l.AmountUSD
		total += l.AmountUSD
	}
	return m, total
}

func absf(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}
