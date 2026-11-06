// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// `waste transfer` — rank the data-transfer / NAT-gateway bleed by spend.
//
// pp:client-call
// pp:data-source auto
package cli

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"
)

func newNovelWasteTransferCmd(flags *rootFlags) *cobra.Command {
	var period, dbPath, profile, region string
	var limit int

	cmd := &cobra.Command{
		Use:   "transfer",
		Short: "Rank the data-transfer and NAT-gateway bleed — the most confusing AWS cost",
		Long: `Surface the cross-AZ, cross-region, and NAT-gateway data-transfer line items
ranked by spend. Data transfer is the most opaque and most-overlooked AWS cost;
this names where it's leaking using the usage-type detail from your synced bill.`,
		Example: `  # This month's transfer bleed
  aws-billing-cli waste transfer

  # Last month as JSON
  aws-billing-cli waste transfer --period last-month --agent`,
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "auto"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			pr, err := resolvePeriod(period)
			if err != nil {
				return usageErr(err)
			}
			return runWasteTransfer(cmd, flags, awsReadOptsFromFlags(flags, dbPath, profile, region), pr, limit)
		},
	}
	cmd.Flags().StringVar(&period, "period", "this-month", "Period: this-month, last-month, last-3-months, ytd, or YYYY-MM-DD:YYYY-MM-DD")
	cmd.Flags().IntVar(&limit, "limit", 0, "Show only the top N usage types (0 = all)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: per-user cache)")
	cmd.Flags().StringVar(&profile, "profile-aws", "", "AWS shared-config profile for a live call")
	cmd.Flags().StringVar(&region, "region", "", "AWS region for a live call")
	return cmd
}

// runWasteTransfer ranks the data-transfer line items of pr read through o:
// waste transfer and ask's transfer answer run it with their own period,
// database, profile and region (Codex r1 class
// delegated-command-drops-caller-scope: ask ran a fresh waste transfer
// command, whose flags were all defaults).
func runWasteTransfer(cmd *cobra.Command, flags *rootFlags, o awsReadOpts, pr periodRange, limit int) error {
	like := []string{"DataTransfer", "Data-Transfer", "NatGateway", "Bytes", "DataXfer"}
	lines, source, err := ensureUsageTypeLines(cmd.Context(), o, pr, like)
	if err != nil {
		return err
	}
	// Sum by usage type.
	type row struct {
		UsageType string
		Service   string
		Amount    float64
	}
	byUT := map[string]*row{}
	var total float64
	for _, l := range lines {
		if byUT[l.UsageType] == nil {
			byUT[l.UsageType] = &row{UsageType: l.UsageType, Service: l.Service}
		}
		byUT[l.UsageType].Amount += l.AmountUSD
		total += l.AmountUSD
	}
	rows := make([]row, 0, len(byUT))
	for _, r := range byUT {
		rows = append(rows, *r)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Amount > rows[j].Amount })
	if limit > 0 && len(rows) > limit {
		rows = rows[:limit]
	}

	if flags.asJSON {
		type jr struct {
			UsageType string  `json:"usage_type"`
			Service   string  `json:"service"`
			AmountUSD float64 `json:"amount_usd"`
		}
		out := struct {
			Period   string  `json:"period"`
			Source   string  `json:"source"`
			TotalUSD float64 `json:"total_usd"`
			Rows     []jr    `json:"rows"`
		}{Period: pr.Label, Source: source, TotalUSD: round2f(total)}
		for _, r := range rows {
			out.Rows = append(out.Rows, jr{r.UsageType, r.Service, round2f(r.Amount)})
		}
		return flags.printJSON(cmd, out)
	}
	w := cmd.OutOrStdout()
	snark := flags.snark && !flags.asJSON
	if len(rows) == 0 {
		fmt.Fprintf(w, "transfer: no data-transfer line items found for %s (source: %s)\n", pr.Label, source)
		return nil
	}
	fmt.Fprintf(w, "Data-transfer bleed — %s (source: %s) — $%.2f total\n", pr.Label, source, round2f(total))
	fmt.Fprintf(w, "  %-44s %14s\n", "USAGE TYPE", "$/period")
	for _, r := range rows {
		fmt.Fprintf(w, "  %-44s %14.2f\n", truncate(r.UsageType, 44), r.Amount)
	}
	fmt.Fprint(w, snarkf(snark, "transfer", len(rows)))
	return nil
}
