// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// `ask` — deterministic natural-language intent router over the synced bill.
// Recognized questions are answered straight from the local cache / SDK; an
// unrecognized question emits the relevant cost slice as JSON to pipe to an LLM.
//
// pp:client-call
package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

func newNovelAskCmd(flags *rootFlags) *cobra.Command {
	var period, dbPath, profile, region string

	cmd := &cobra.Command{
		Use:   "ask <question>",
		Short: "Ask a plain-English question about your bill (top services, biggest mover, per-account total, waste)",
		Long: `Answer common questions about your AWS bill straight from the synced cache:

  "what are my top services"        -> top services by spend
  "which account costs the most"    -> per-account ranking
  "what changed since last month"   -> month-over-month movers
  "where is my data transfer going" -> data-transfer bleed
  "how much am I wasting"           -> waste rollup
  "what's my total"                 -> period total

Anything it doesn't recognize is answered with the relevant cost slice as JSON,
ready to pipe to an LLM (e.g. '... --agent | claude -p "..."').`,
		Example: `  aws-billing-cli ask "what changed since last month"
  aws-billing-cli ask "which account costs the most" --agent`,
		// ask accepts any free-text question (unknown -> emits a cost slice,
		// exit 0), so there is no invalid-argument error path to probe.
		Annotations: map[string]string{"mcp:read-only": "true", "pp:no-error-path-probe": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return nil
			}
			q := strings.ToLower(strings.Join(args, " "))
			pr, err := resolvePeriod(period)
			if err != nil {
				return usageErr(err)
			}
			o := awsReadOptsFromFlags(flags, dbPath, profile, region)
			w := cmd.OutOrStdout()

			switch intent := classifyAsk(q); intent {
			case "changed":
				fromPR, _ := resolvePeriod("last-month")
				toPR, _ := resolvePeriod("this-month")
				from, _, _ := canonicalCostLines(cmd.Context(), o, fromPR)
				to, src, _ := canonicalCostLines(cmd.Context(), o, toPR)
				fromS, ft := sumByService(from)
				toS, tt := sumByService(to)
				type mv struct {
					Service  string  `json:"service"`
					DeltaUSD float64 `json:"delta_usd"`
				}
				var movers []mv
				seen := map[string]bool{}
				for s := range fromS {
					seen[s] = true
				}
				for s := range toS {
					seen[s] = true
				}
				for s := range seen {
					movers = append(movers, mv{s, round2f(toS[s] - fromS[s])})
				}
				sort.Slice(movers, func(i, j int) bool { return absf(movers[i].DeltaUSD) > absf(movers[j].DeltaUSD) })
				if len(movers) > 5 {
					movers = movers[:5]
				}
				if flags.asJSON {
					return flags.printJSON(cmd, map[string]any{"intent": intent, "source": src, "from_total": round2f(ft), "to_total": round2f(tt), "top_movers": movers})
				}
				fmt.Fprintf(w, "Last month $%.2f -> this month $%.2f (%+.1f%%). Biggest movers:\n", ft, tt, pctDelta(ft, tt))
				for _, m := range movers {
					fmt.Fprintf(w, "  %+10.2f  %s\n", m.DeltaUSD, m.Service)
				}
				return nil

			case "account":
				lines, src, _ := canonicalCostLines(cmd.Context(), o, pr)
				byAcct, total := sumByAccount(lines)
				nameByAcct := map[string]string{}
				for _, l := range lines {
					if l.AccountName != "" {
						nameByAcct[l.AccountID] = l.AccountName
					}
				}
				type ar struct {
					AccountID string  `json:"account_id"`
					Name      string  `json:"name,omitempty"`
					AmountUSD float64 `json:"amount_usd"`
				}
				var rows []ar
				for a, amt := range byAcct {
					rows = append(rows, ar{a, nameByAcct[a], round2f(amt)})
				}
				sort.Slice(rows, func(i, j int) bool { return rows[i].AmountUSD > rows[j].AmountUSD })
				if flags.asJSON {
					return flags.printJSON(cmd, map[string]any{"intent": intent, "source": src, "period": pr.Label, "total": round2f(total), "accounts": rows})
				}
				fmt.Fprintf(w, "Per-account spend (%s):\n", pr.Label)
				for _, r := range rows {
					label := r.AccountID
					if r.Name != "" {
						label = r.Name + " " + r.AccountID
					}
					fmt.Fprintf(w, "  $%12.2f  %s\n", r.AmountUSD, label)
				}
				return nil

			case "transfer":
				return newNovelWasteTransferCmd(flags).RunE(cmd, nil)

			case "waste":
				return runWasteList(cmd, flags, o, "waste rank", "", "", "savings")

			case "total":
				lines, src, _ := canonicalCostLines(cmd.Context(), o, pr)
				_, total := sumByService(lines)
				if flags.asJSON {
					return flags.printJSON(cmd, map[string]any{"intent": intent, "source": src, "period": pr.Label, "total_usd": round2f(total)})
				}
				fmt.Fprintf(w, "Total for %s: $%.2f (source: %s)\n", pr.Label, round2f(total), src)
				return nil

			case "services":
				lines, src, _ := canonicalCostLines(cmd.Context(), o, pr)
				groups, total := groupCostLines(lines, "service")
				if len(groups) > 10 {
					groups = groups[:10]
				}
				if flags.asJSON {
					return flags.printJSON(cmd, map[string]any{"intent": intent, "source": src, "period": pr.Label, "total": round2f(total), "top_services": groups})
				}
				fmt.Fprintf(w, "Top services (%s):\n", pr.Label)
				for _, g := range groups {
					fmt.Fprintf(w, "  $%12.2f  %5.1f%%  %s\n", g.AmountUSD, g.Pct, g.Key)
				}
				return nil

			default:
				// Unknown intent: emit the cost slice for an LLM to interpret.
				lines, src, _ := canonicalCostLines(cmd.Context(), o, pr)
				groups, total := groupCostLines(lines, "service")
				payload := map[string]any{
					"question":     strings.Join(args, " "),
					"unrecognized": true,
					"source":       src,
					"period":       pr.Label,
					"total_usd":    round2f(total),
					"by_service":   groups,
				}
				if flags.asJSON {
					return flags.printJSON(cmd, payload)
				}
				fmt.Fprintf(w, "I don't have a built-in answer for %q.\n", strings.Join(args, " "))
				fmt.Fprintf(w, "Here's this period's by-service breakdown ($%.2f total). For a free-form answer, pipe the JSON to an LLM:\n", round2f(total))
				fmt.Fprintf(w, "  aws-billing-cli ask %q --agent | claude -p \"answer the question from this AWS cost JSON\"\n", strings.Join(args, " "))
				for _, g := range groups {
					if g.AmountUSD <= 0 {
						continue
					}
					fmt.Fprintf(w, "  $%12.2f  %s\n", g.AmountUSD, g.Key)
				}
				return nil
			}
		},
	}
	cmd.Flags().StringVar(&period, "period", "this-month", "Period the question applies to")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: per-user cache)")
	cmd.Flags().StringVar(&profile, "profile-aws", "", "AWS shared-config profile for a live call")
	cmd.Flags().StringVar(&region, "region", "", "AWS region for a live call")
	return cmd
}

// classifyAsk maps a lowercased question to a known intent or "" (unknown).
func classifyAsk(q string) string {
	switch {
	case containsAny(q, "changed", "change", "jump", "spike", "why", "increase", "went up", "more than last"):
		return "changed"
	case containsAny(q, "account", "linked", "which org", "per account"):
		return "account"
	case containsAny(q, "transfer", "egress", "data out", "nat", "bandwidth"):
		return "transfer"
	case containsAny(q, "waste", "wasting", "save", "savings", "idle", "orphan", "unused"):
		return "waste"
	case containsAny(q, "total", "how much", "overall", "altogether", "sum"):
		return "total"
	case containsAny(q, "top service", "biggest service", "most expensive", "what costs", "by service", "services"):
		return "services"
	}
	return ""
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
