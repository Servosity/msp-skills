// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// `consolidated` — per-account + aggregate org rollup with inline month-over-month delta.
//
// pp:client-call
package cli

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"

	"aws-billing-pp-cli/internal/awsx"
)

type consolidatedAccount struct {
	AccountID string  `json:"account_id"`
	Name      string  `json:"name,omitempty"`
	AmountUSD float64 `json:"amount_usd"`
	PriorUSD  float64 `json:"prior_usd"`
	DeltaUSD  float64 `json:"delta_usd"`
	DeltaPct  float64 `json:"delta_pct"`
}

type consolidatedResult struct {
	Period   string                `json:"period"`
	Source   string                `json:"source"`
	TotalUSD float64               `json:"total_usd"`
	PriorUSD float64               `json:"prior_total_usd"`
	DeltaPct float64               `json:"delta_pct"`
	Accounts []consolidatedAccount `json:"accounts"`
}

func newNovelConsolidatedCmd(flags *rootFlags) *cobra.Command {
	var period, dbPath, profile, region string

	cmd := &cobra.Command{
		Use:   "consolidated",
		Short: "Per-account + aggregate org rollup with month-over-month delta",
		Long: `Show each linked account's bill (names resolved from AWS Organizations),
the management rollup total, and an inline month-over-month delta per account.

Org-wide data requires a management (payer) account profile; from a member
account you'll see only that account.`,
		Example: `  # This month, per account, with deltas
  aws-billing-cli consolidated

  # Last month as JSON
  aws-billing-cli consolidated --period last-month --agent`,
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			pr, err := resolvePeriod(period)
			if err != nil {
				return usageErr(err)
			}
			prev := previousPeriod(pr)
			o := awsReadOptsFromFlags(flags, dbPath, profile, region)

			cur, source, err := canonicalCostLines(cmd.Context(), o, pr)
			if err != nil {
				return err
			}
			priorLines, _, _ := canonicalCostLines(cmd.Context(), o, prev)

			curByAcct, curTotal := sumByAccount(cur)
			priorByAcct, priorTotal := sumByAccount(priorLines)
			nameByAcct := map[string]string{}
			for _, l := range cur {
				if l.AccountName != "" {
					nameByAcct[l.AccountID] = l.AccountName
				}
			}

			res := consolidatedResult{Period: pr.Label, Source: source, TotalUSD: round2f(curTotal), PriorUSD: round2f(priorTotal), DeltaPct: pctDelta(priorTotal, curTotal)}
			for acct, amt := range curByAcct {
				key := acct
				if key == "" {
					key = "(unattributed)"
				}
				prior := priorByAcct[acct]
				res.Accounts = append(res.Accounts, consolidatedAccount{
					AccountID: key,
					Name:      nameByAcct[acct],
					AmountUSD: round2f(amt),
					PriorUSD:  round2f(prior),
					DeltaUSD:  round2f(amt - prior),
					DeltaPct:  pctDelta(prior, amt),
				})
			}
			sort.Slice(res.Accounts, func(i, j int) bool { return res.Accounts[i].AmountUSD > res.Accounts[j].AmountUSD })

			if flags.asJSON {
				return flags.printJSON(cmd, res)
			}
			w := cmd.OutOrStdout()
			snark := flags.snark && !flags.asJSON
			fmt.Fprint(w, snarkf(snark, "intro", len(cur)))
			fmt.Fprintf(w, "Consolidated bill — %s (source: %s)\n", res.Period, res.Source)
			if len(res.Accounts) == 0 {
				fmt.Fprintln(w, "  no cost data (run 'sync' against a management-account profile, or check 'doctor')")
				return nil
			}
			fmt.Fprintf(w, "  %-34s %12s %12s %8s\n", "ACCOUNT", "THIS", "PRIOR", "Δ%")
			for _, a := range res.Accounts {
				label := a.AccountID
				if a.Name != "" {
					label = fmt.Sprintf("%s %s", a.Name, a.AccountID)
				}
				fmt.Fprintf(w, "  %-34s %12.2f %12.2f %+7.1f%%\n", truncate(label, 34), a.AmountUSD, a.PriorUSD, a.DeltaPct)
			}
			fmt.Fprintf(w, "  %-34s %12.2f %12.2f %+7.1f%%\n", "TOTAL", res.TotalUSD, res.PriorUSD, res.DeltaPct)
			if res.DeltaPct >= 10 {
				fmt.Fprint(w, snarkf(snark, "big-jump", int(res.TotalUSD)))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&period, "period", "this-month", "Period: this-month, last-month, last-3-months, ytd, 7d, 30d, yesterday, or YYYY-MM-DD:YYYY-MM-DD")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: per-user cache)")
	cmd.Flags().StringVar(&profile, "profile-aws", "", "AWS shared-config profile for a live call")
	cmd.Flags().StringVar(&region, "region", "", "AWS region for a live call")
	return cmd
}

func sumByAccount(lines []awsx.CostLine) (map[string]float64, float64) {
	m := map[string]float64{}
	var total float64
	for _, l := range lines {
		m[l.AccountID] += l.AmountUSD
		total += l.AmountUSD
	}
	return m, total
}

func pctDelta(prior, cur float64) float64 {
	if prior == 0 {
		if cur == 0 {
			return 0
		}
		return 100
	}
	return round2f((cur - prior) / prior * 100)
}
