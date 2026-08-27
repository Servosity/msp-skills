// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// `bill` — pull the AWS bill for a period and break it down by a dimension.
//
// pp:client-call
package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"aws-billing-pp-cli/internal/awsx"
)

// costGroup is one row of an aggregated cost breakdown.
type costGroup struct {
	Key       string  `json:"key"`
	Name      string  `json:"name,omitempty"`
	AmountUSD float64 `json:"amount_usd"`
	Pct       float64 `json:"pct_of_total"`
}

type billResult struct {
	Period   string      `json:"period"`
	GroupBy  string      `json:"group_by"`
	Source   string      `json:"source"`
	TotalUSD float64     `json:"total_usd"`
	Groups   []costGroup `json:"groups"`
}

func newBillCmd(flags *rootFlags) *cobra.Command {
	var period, groupBy, account, service, dbPath, profile, region string
	var limit int

	cmd := &cobra.Command{
		Use:   "bill",
		Short: "Pull the AWS bill for a period and break it down by service, account, region, or usage type",
		Long: `Pull the AWS bill for a period and break it down along one dimension.

Reads from the local cache first (after 'sync'); falls back to a live Cost
Explorer call and caches the result. Use --data-source live to force a live
call or --data-source local to require the cache.`,
		Example: `  # This month, by service
  aws-billing-cli bill

  # Last month, by account, as JSON for an agent
  aws-billing-cli bill --period last-month --group-by account --agent

  # A specific window, by usage type
  aws-billing-cli bill --period 2026-01-01:2026-04-01 --group-by usage-type`,
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			pr, err := resolvePeriod(period)
			if err != nil {
				return usageErr(err)
			}
			gb, err := normalizeGroupBy(groupBy)
			if err != nil {
				return usageErr(err)
			}
			o := awsReadOptsFromFlags(flags, dbPath, profile, region)
			lines, source, err := canonicalCostLines(cmd.Context(), o, pr)
			if err != nil {
				return err
			}
			// Apply optional filters.
			lines = filterCostLines(lines, account, service)
			groups, total := groupCostLines(lines, gb)
			if limit > 0 && len(groups) > limit {
				groups = groups[:limit]
			}
			res := billResult{Period: pr.Label, GroupBy: gb, Source: source, TotalUSD: round2f(total), Groups: groups}

			if flags.asJSON {
				return flags.printJSON(cmd, res)
			}
			w := cmd.OutOrStdout()
			snark := flags.snark && !flags.asJSON
			fmt.Fprint(w, snarkf(snark, "intro", len(lines)))
			fmt.Fprintf(w, "AWS bill — %s (by %s, source: %s)\n", res.Period, res.GroupBy, res.Source)
			if len(groups) == 0 {
				fmt.Fprintln(w, "  no cost data for this period (run 'sync' against a management-account profile, or check 'doctor')")
				return nil
			}
			for _, g := range groups {
				label := g.Key
				if g.Name != "" {
					label = fmt.Sprintf("%s (%s)", g.Name, g.Key)
				}
				fmt.Fprintf(w, "  %-50s $%10.2f  %5.1f%%\n", truncate(label, 50), g.AmountUSD, g.Pct)
			}
			fmt.Fprintf(w, "  %-50s $%10.2f\n", "TOTAL", res.TotalUSD)
			return nil
		},
	}

	cmd.Flags().StringVar(&period, "period", "this-month", "Period: this-month, last-month, last-3-months, ytd, 7d, 30d, yesterday, or YYYY-MM-DD:YYYY-MM-DD")
	cmd.Flags().StringVar(&groupBy, "group-by", "service", "Breakdown dimension: service, account, region, usage-type")
	cmd.Flags().StringVar(&account, "account", "", "Filter to a 12-digit account ID")
	cmd.Flags().StringVar(&service, "service", "", "Filter to an AWS service name substring")
	cmd.Flags().IntVar(&limit, "limit", 0, "Show only the top N groups (0 = all)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: per-user cache)")
	cmd.Flags().StringVar(&profile, "profile-aws", "", "AWS shared-config profile for a live call")
	cmd.Flags().StringVar(&region, "region", "", "AWS region for a live call")
	return cmd
}

func normalizeGroupBy(s string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "service":
		return "service", nil
	case "account", "linked_account", "linked-account":
		return "account", nil
	case "region":
		return "region", nil
	case "usage-type", "usage_type", "usagetype":
		return "usage-type", nil
	}
	return "", fmt.Errorf("unknown --group-by %q (service, account, region, usage-type)", s)
}

func filterCostLines(lines []awsx.CostLine, account, service string) []awsx.CostLine {
	if account == "" && service == "" {
		return lines
	}
	var out []awsx.CostLine
	for _, l := range lines {
		if account != "" && l.AccountID != account {
			continue
		}
		if service != "" && !strings.Contains(strings.ToLower(l.Service), strings.ToLower(service)) {
			continue
		}
		out = append(out, l)
	}
	return out
}

// groupCostLines sums amounts by the chosen dimension and returns rows sorted
// by amount descending, plus the grand total.
func groupCostLines(lines []awsx.CostLine, groupBy string) ([]costGroup, float64) {
	type acc struct {
		amount float64
		name   string
	}
	sums := map[string]*acc{}
	var total float64
	for _, l := range lines {
		var key, name string
		switch groupBy {
		case "account":
			key = l.AccountID
			name = l.AccountName
		case "region":
			key = l.Region
		case "usage-type":
			key = l.UsageType
		default:
			key = l.Service
		}
		if key == "" {
			key = "(unattributed)"
		}
		if sums[key] == nil {
			sums[key] = &acc{}
		}
		sums[key].amount += l.AmountUSD
		if name != "" {
			sums[key].name = name
		}
		total += l.AmountUSD
	}
	groups := make([]costGroup, 0, len(sums))
	for k, v := range sums {
		pct := 0.0
		if total > 0 {
			pct = v.amount / total * 100
		}
		groups = append(groups, costGroup{Key: k, Name: v.name, AmountUSD: round2f(v.amount), Pct: round2f(pct)})
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i].AmountUSD > groups[j].AmountUSD })
	return groups, total
}

func round2f(f float64) float64 { return float64(int64(f*100+0.5)) / 100 }
