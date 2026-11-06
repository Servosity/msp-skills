// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel command: monthly cost vs price across the synced book.
// pp:data-source local

package cli

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

type marginRow struct {
	Key           string   `json:"key"`
	Label         string   `json:"label"`
	Subscriptions int      `json:"subscriptions"`
	Seats         float64  `json:"seats"`
	MonthlyPrice  float64  `json:"monthly_price"`
	MonthlyCost   float64  `json:"monthly_cost"`
	MonthlyMSRP   float64  `json:"monthly_msrp"`
	Margin        float64  `json:"margin"`
	MarginPct     *float64 `json:"margin_pct"`
	Currency      string   `json:"currency"`
	NegativeSubs  int      `json:"negative_subs"`
	UnknownCost   int      `json:"cost_unknown_subs"`  // subscriptions ION reported no (or only part of a) cost for
	UnknownPrice  int      `json:"price_unknown_subs"` // subscriptions ION reported no (or only part of a) price for, or with a billing cycle the CLI does not recognize
	UnknownSeats  int      `json:"seats_unknown_subs"` // subscriptions ION reported no licence count for; seats covers the rest
	UnknownMSRP   int      `json:"msrp_unknown_subs"`  // known-price subscriptions ION reported no MSRP for; monthly_msrp covers the rest
	// PriceUnknownCost is the monthly price of subscriptions whose cost is
	// unknown. margin and margin_pct cover only subscriptions whose price and
	// cost are both known; monthly_price and monthly_cost sum the known amounts.
	PriceUnknownCost float64 `json:"monthly_price_cost_unknown"`
	// CostUnknownPrice is the monthly cost of subscriptions whose price is
	// unknown, inside monthly_cost and left out of margin.
	CostUnknownPrice float64  `json:"monthly_cost_price_unknown"`
	ZeroSubs         int      `json:"zero_margin_subs"`
	Flags            []string `json:"flags"`
	Basis            string   `json:"basis"`
	MarginKnown      bool     `json:"margin_known"` // false when no subscription in the group has both a known price and cost: margin is then 0 and not a real margin
	paired           int      // subscriptions with a known price and cost
}

type marginTrendRow struct {
	Key           string  `json:"key"`
	Label         string  `json:"label"`
	Month         string  `json:"month"`
	CustomerCost  float64 `json:"customer_cost"`
	SellerCost    float64 `json:"seller_cost"`
	Margin        float64 `json:"margin"` // rows with a known sellerCost only
	Currency      string  `json:"currency"`
	CostComplete  bool    `json:"cost_complete"`  // false when a billing row for the month had no sellerCost
	PriceComplete bool    `json:"price_complete"` // false when a billing row for the month had no customerCost
	// CustomerCostCostUnknown is the price of billing rows with no sellerCost,
	// left out of margin rather than counted at a cost of 0. margin covers rows
	// with both a customerCost and a sellerCost.
	CustomerCostCostUnknown float64 `json:"customer_cost_cost_unknown"`
	MarginKnown             bool    `json:"margin_known"` // false when no billing row for the month had both a customerCost and a sellerCost
	knownPrice, knownCost   float64
	nPrice, nCost, nPaired  int // billing rows with a customerCost, a sellerCost, and both
}

// subIdentity names a subscription that has no group key of its own: its ION
// id, else its vendor subscription id, else its position in the store.
func subIdentity(s ionSub, i int) string {
	for _, v := range []string{s.ID, s.SubscriptionID} {
		if v != "" {
			return "subscription:" + v
		}
	}
	return "subscription:#" + strconv.Itoa(i)
}

// marginKey returns the grouping key, the key shown, and the label. A
// subscription with no SKU or customer identity is its own group, keyed by its
// store position in a namespace no ION value can reach, so it never merges
// with another (Codex rounds 16, 18, 19 and 22, P1).
func marginKey(b *ionBook, s ionSub, i int, by string) (group, key, label string) {
	if by == "sku" {
		if k := s.SKUKey(); k != "" {
			return "k\x00" + k, k, s.SKULabel()
		}
		return "s\x00" + strconv.Itoa(i), subIdentity(s, i), s.SKULabel()
	}
	if s.CustomerID == "" {
		label := s.CustomerName
		if label == "" {
			label = "unknown customer"
		}
		return "s\x00" + strconv.Itoa(i), subIdentity(s, i), label
	}
	return "c\x00" + s.CustomerID, s.CustomerID, b.CustomerLabel(s.CustomerID, s.CustomerName)
}

// bookMargin rolls active subscriptions up by customer or SKU, per currency.
func bookMargin(b *ionBook, by string) []marginRow {
	type acc struct {
		row           marginRow
		bases         map[string]bool
		mPrice, mCost float64 // subscriptions whose price and cost are both known
		paired        int     // how many such subscriptions
	}
	type gkey struct{ group, cur string }
	groups := map[gkey]*acc{}
	for i, s := range b.Subs {
		if s.Inactive() {
			continue
		}
		group, key, label := marginKey(b, s, i, by)
		m := s.MonthlyMoney()
		price, cost, msrp, basis := m.Price, m.Cost, m.MSRP, m.Basis
		gk := gkey{group, m.Currency} // a struct key: no text in a key or currency can join two groups
		a := groups[gk]
		if a == nil {
			a = &acc{row: marginRow{Key: key, Label: label, Currency: m.Currency}, bases: map[string]bool{}}
			groups[gk] = a
		}
		a.row.Subscriptions++
		if s.Seats.Set {
			a.row.Seats += s.Seats.V
		} else {
			a.row.UnknownSeats++
		}
		// An unknown amount is never counted as 0 (adversarial round 14, P1: a
		// missing price read as 0 and flagged a loss).
		if m.PriceKnown {
			a.row.MonthlyPrice += price
			// A missing MSRP is unknown, never 0 (Codex round 19, P1): monthly_msrp
			// is then a lower bound, so below_msrp stays sound, and the group is
			// flagged msrp_incomplete.
			if m.MSRPKnown {
				a.row.MonthlyMSRP += msrp
			} else {
				a.row.UnknownMSRP++
			}
		} else {
			a.row.UnknownPrice++
		}
		if m.CostKnown {
			a.row.MonthlyCost += cost
			if !m.PriceKnown {
				a.row.CostUnknownPrice += cost
			}
		} else {
			a.row.UnknownCost++
			if m.PriceKnown {
				a.row.PriceUnknownCost += price
			}
		}
		if m.PriceKnown && m.CostKnown {
			a.paired++
			a.mPrice += price
			a.mCost += cost
			switch d := round2(price - cost); {
			case d < 0:
				a.row.NegativeSubs++
			case d == 0 && cost > 0:
				a.row.ZeroSubs++
			}
		}
		if strings.HasPrefix(basis, "billing:") {
			basis = "billing"
		}
		a.bases[basis] = true
	}
	rows := make([]marginRow, 0, len(groups))
	for _, a := range groups {
		r := a.row
		r.paired, r.MarginKnown = a.paired, a.paired > 0
		r.MonthlyPrice, r.MonthlyCost, r.MonthlyMSRP = round2(r.MonthlyPrice), round2(r.MonthlyCost), round2(r.MonthlyMSRP)
		r.PriceUnknownCost, r.CostUnknownPrice = round2(r.PriceUnknownCost), round2(r.CostUnknownPrice)
		r.Margin = round2(a.mPrice - a.mCost) // subscriptions with a known price and cost only
		if a.mPrice != 0 {
			if pct := round2(r.Margin / a.mPrice * 100); !math.IsInf(pct, 0) && !math.IsNaN(pct) {
				r.MarginPct = &pct // a tiny price would divide to Inf and break the JSON report
			}
		}
		r.Flags = make([]string, 0)
		switch {
		case r.Margin < 0:
			r.Flags = append(r.Flags, "negative")
		case r.Margin == 0 && a.mCost > 0:
			r.Flags = append(r.Flags, "zero")
		}
		if r.MonthlyMSRP > 0 && r.MonthlyPrice < r.MonthlyMSRP {
			r.Flags = append(r.Flags, "below_msrp")
		}
		if r.UnknownPrice > 0 {
			r.Flags = append(r.Flags, "price_incomplete") // margin covers the subscriptions with a known price and cost only
		}
		if r.UnknownSeats > 0 {
			r.Flags = append(r.Flags, "seats_incomplete")
		}
		if r.UnknownMSRP > 0 {
			r.Flags = append(r.Flags, "msrp_incomplete")
		}
		if r.NegativeSubs > 0 && r.Margin >= 0 {
			r.Flags = append(r.Flags, "has_negative_subs")
		}
		if r.UnknownCost > 0 {
			r.Flags = append(r.Flags, "cost_incomplete") // margin covers the known-cost subscriptions only
		}
		bases := make([]string, 0, len(a.bases))
		for k := range a.bases {
			bases = append(bases, k)
		}
		sort.Strings(bases)
		r.Basis = strings.Join(bases, "+")
		rows = append(rows, r)
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if (rows[i].paired == 0) != (rows[j].paired == 0) {
			return rows[j].paired == 0 // a group with no known margin sorts last, not as a zero margin
		}
		if rows[i].Margin != rows[j].Margin {
			return rows[i].Margin < rows[j].Margin
		}
		if rows[i].Key != rows[j].Key {
			return rows[i].Key < rows[j].Key
		}
		return rows[i].Currency < rows[j].Currency // a stable order run to run (adversarial round 25)
	})
	return rows
}

// marginTrend sums billingData per group and month for the newest n months.
func marginTrend(b *ionBook, by string, months int) []marginTrendRow {
	type k struct{ group, month, cur string }
	type gc struct{ group, cur string }
	sums := map[k]*marginTrendRow{}
	monthSet := map[string]bool{}
	undated := map[gc]bool{} // a billing row with no readable month may belong to any month
	for i, s := range b.Subs {
		group, key, label := marginKey(b, s, i, by)
		for _, d := range s.BillingData {
			cur := d.Currency
			if cur == "" {
				cur = s.CurrencyCode()
			}
			month := billingMonthKey(d.BillingMonth)
			if month == "" {
				undated[gc{group, cur}] = true
				continue
			}
			gk := k{group, month, cur}
			r := sums[gk]
			if r == nil {
				r = &marginTrendRow{Key: key, Label: label, Month: month, Currency: cur, CostComplete: true, PriceComplete: true}
				sums[gk] = r
			}
			r.CustomerCost += d.CustomerCost.V
			r.SellerCost += d.SellerCost.V
			if d.CustomerCost.Set {
				r.nPrice++
			}
			if d.SellerCost.Set {
				r.nCost++
			}
			switch {
			case d.SellerCost.Set && d.CustomerCost.Set:
				r.nPaired++
				r.knownPrice += d.CustomerCost.V
				r.knownCost += d.SellerCost.V
			case d.CustomerCost.Set:
				r.CustomerCostCostUnknown += d.CustomerCost.V
			}
			r.CostComplete = r.CostComplete && d.SellerCost.Set
			r.PriceComplete = r.PriceComplete && d.CustomerCost.Set
			monthSet[month] = true
		}
	}
	all := make([]string, 0, len(monthSet))
	for m := range monthSet {
		all = append(all, m)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(all)))
	keep := map[string]bool{}
	for i, m := range all {
		if i < months {
			keep[m] = true
		}
	}
	rows := make([]marginTrendRow, 0)
	for gk, r := range sums {
		if !keep[r.Month] {
			continue
		}
		if undated[gc{gk.group, gk.cur}] {
			// Codex round 22, P1: an undated row was dropped while every month
			// read as complete.
			r.PriceComplete, r.CostComplete, r.nPaired = false, false, 0
		}
		r.CustomerCost, r.SellerCost, r.CustomerCostCostUnknown = round2(r.CustomerCost), round2(r.SellerCost), round2(r.CustomerCostCostUnknown)
		r.Margin, r.MarginKnown = round2(r.knownPrice-r.knownCost), r.nPaired > 0
		rows = append(rows, *r)
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Key != rows[j].Key {
			return rows[i].Key < rows[j].Key
		}
		if rows[i].Month != rows[j].Month {
			return rows[i].Month < rows[j].Month
		}
		return rows[i].Currency < rows[j].Currency
	})
	return rows
}

func newNovelMarginCmd(flags *rootFlags) *cobra.Command {
	var by, dbPath string
	var months int

	cmd := &cobra.Command{
		Use:   "margin",
		Short: "See monthly cost vs price and margin per customer or per SKU, with negative, zero and below-MSRP rows flagged.",
		Long: "See monthly cost vs price and margin per customer or per SKU across every active synced subscription. " +
			"Monthly billing uses the newest billed month when ION returns billingData (basis=billing); annual and longer cycles bill lumpily across terms, so they use the " +
			"subscription's price and cost (when ION sends neither, the billed rows of the trailing cycle, basis=billing); a billed month missing its price or cost takes the subscription's price and cost together, never one half of each; otherwise the " +
			"subscription's price and cost normalized by billing cycle (basis=subscription). Rows are split by currency " +
			"and sorted worst margin first; negative_subs counts loss-making subscriptions hidden inside a profitable group, and " +
			"cost_unknown_subs (flag cost_incomplete) and price_unknown_subs (flag price_incomplete) count subscriptions ION reported no cost or no price for (or whose billing cycle the CLI does not recognize); " +
			"an unknown amount is never counted as 0: margin and margin_pct cover only subscriptions with a known price and cost, the price of those with an unknown cost is reported as monthly_price_cost_unknown, and the cost of those with an unknown price as monthly_cost_price_unknown. " +
			"Amounts are as ION reports them per subscription (not multiplied by seats). --months N adds a per-month trend " +
			"from billingData instead.\n\n" +
			"Use this command for cost vs price within TD SYNNEX. Do NOT use this command to compare seats or costs " +
			"across distributors; use 'position trueup' instead. Do NOT use it for one customer's full picture; use " +
			"'customers show' instead.",
		Example: strings.Trim(`
  td-synnex-cli margin --by customer --agent
  td-synnex-cli margin --by sku --csv
  td-synnex-cli margin --by customer --months 6`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local", "pp:happy-args": "--by=customer"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "margin")
			}
			by = strings.ToLower(strings.TrimSpace(by))
			if by != "customer" && by != "sku" {
				return usageErr(fmt.Errorf("--by must be customer or sku, got %q", by))
			}
			if months < 0 {
				return usageErr(fmt.Errorf("--months must be 0 or more"))
			}
			book, closeDB, err := openIONBook(cmd, flags, dbPath, "subscriptions", "customers")
			if err != nil {
				return err
			}
			defer closeDB()
			if months > 0 {
				trend := make([]marginTrendRow, 0)
				if book != nil {
					trend = marginTrend(book, by, months)
				}
				if !wantsHumanTable(cmd.OutOrStdout(), flags) {
					return printIONJSON(cmd.OutOrStdout(), trend, flags)
				}
				if len(trend) == 0 {
					fmt.Fprintln(cmd.OutOrStdout(), "No billingData months in the local store (ION returns it for Microsoft CSP subscriptions).")
					return nil
				}
				table := make([][]string, 0, len(trend))
				for _, r := range trend {
					price, cost, margin := moneyText(r.CustomerCost, r.nPrice > 0), moneyText(r.SellerCost, r.nCost > 0), moneyText(r.Margin, r.nPaired > 0)
					if !r.PriceComplete {
						price += " (incomplete)"
					}
					if !r.CostComplete {
						cost += " (incomplete)"
					}
					if !r.PriceComplete || !r.CostComplete {
						margin += " (rows with price and cost only)"
					}
					table = append(table, []string{r.Month, r.Label, price, cost, margin, r.Currency})
				}
				return printTable(cmd.OutOrStdout(), []string{"MONTH", strings.ToUpper(by), "PRICE", "COST", "MARGIN", "CUR"}, table)
			}
			rows := make([]marginRow, 0)
			if book != nil {
				rows = bookMargin(book, by)
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printIONJSON(cmd.OutOrStdout(), rows, flags)
			}
			if len(rows) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No active subscriptions in the local store.")
				return nil
			}
			table := make([][]string, 0, len(rows))
			seen := map[string]string{}
			dup := map[string]bool{}
			for _, r := range rows {
				if k, ok := seen[r.Label]; ok && k != r.Key {
					dup[r.Label] = true
				}
				seen[r.Label] = r.Key
			}
			for _, r := range rows {
				pct := "-"
				if r.MarginPct != nil {
					pct = strconv.FormatFloat(*r.MarginPct, 'f', 1, 64) + "%"
				}
				label := r.Label
				if dup[label] {
					label += " (" + r.Key + ")" // two customers or SKUs share a name
				}
				table = append(table, []string{label, strconv.Itoa(r.Subscriptions), seatsText(r.Seats, r.UnknownSeats < r.Subscriptions),
					moneyText(r.MonthlyPrice, r.UnknownPrice < r.Subscriptions), moneyText(r.MonthlyCost, r.UnknownCost < r.Subscriptions),
					moneyText(r.Margin, r.paired > 0), pct, r.Currency, strings.Join(r.Flags, ",")})
			}
			return printTable(cmd.OutOrStdout(), []string{strings.ToUpper(by), "SUBS", "SEATS", "PRICE/MO", "COST/MO", "MARGIN", "PCT", "CUR", "FLAGS"}, table)
		},
	}
	cmd.Flags().StringVar(&by, "by", "customer", "Group by customer or sku")
	cmd.Flags().IntVar(&months, "months", 0, "Show a per-month trend for the newest N billing months instead of the current run-rate")
	cmd.Flags().StringVar(&dbPath, "db", "", "Local store path (default: the CLI's synced database)")
	return cmd
}
