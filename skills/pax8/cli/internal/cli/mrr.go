// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel feature (Phase 3): MRR and margin. Not generator-managed.

package cli

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"
	"pax8-pp-cli/internal/store"
)

// mrrMargin sums a margin only over the rows that carry Subscription.partnerCost.
// The spec does not require it and no example carries it, so a row without one has
// an unknown margin: it is counted, never read as a cost of zero.
type mrrMargin struct {
	sum     float64
	rows    int
	lacking int
}

func (m *mrrMargin) add(margin float64, hasCost bool) {
	if !hasCost {
		m.lacking++
		return
	}
	m.sum += margin
	m.rows++
}

// figure is the margin, nil when there are rows and none of them carries the cost.
func (m mrrMargin) figure() *float64 {
	if m.rows == 0 && m.lacking > 0 {
		return nil
	}
	v := m.sum
	return &v
}

type mrrProductRow struct {
	ProductID   string   `json:"productId"`
	ProductName string   `json:"productName"`
	Quantity    float64  `json:"quantity"`
	MRR         float64  `json:"mrr"`
	Margin      *float64 `json:"margin"`
	// Set only when more than one currency is present: then a product has one
	// row per currency.
	Currency                 string `json:"currency,omitempty"`
	SubscriptionsWithoutCost int    `json:"subscriptionsWithoutCost,omitempty"`
	margin                   mrrMargin
}

// mrrCurrencyTotal is the totals of one currency, reported in byCurrency when
// more than one is present.
type mrrCurrencyTotal struct {
	Currency                 string   `json:"currency"`
	MRR                      float64  `json:"mrr"`
	Margin                   *float64 `json:"margin"`
	ActiveSubscriptions      int      `json:"activeSubscriptions"`
	SubscriptionsWithoutCost int      `json:"subscriptionsWithoutCost"`
	margin                   mrrMargin
}

type mrrTotals struct {
	// Null when more than one currency is present, since no single sum exists
	// (ByCurrency has each); Margin also when no subscription carries a cost.
	MRR                      *float64           `json:"mrr"`
	Margin                   *float64           `json:"margin"`
	ActiveSubscriptions      int                `json:"activeSubscriptions"`
	SubscriptionsWithoutCost int                `json:"subscriptionsWithoutCost"`
	Currency                 string             `json:"currency,omitempty"`
	ByCurrency               []mrrCurrencyTotal `json:"byCurrency,omitempty"`
	// Subscriptions left out of MRR (pax8 handfix: novel-documented-shapes):
	// billed on another cadence (per-term amounts, never summed into MRR),
	// not recurring, or with a status or term outside the documented enums.
	OtherCadence              []mrrTermTotal `json:"otherCadence"`
	NonRecurringSubscriptions int            `json:"nonRecurringSubscriptions"`
	UnclassifiedSubscriptions int            `json:"unclassifiedSubscriptions"`
}

// mrrTermTotal is the price x quantity of the billing subscriptions on one
// non-monthly recurring term (Annual, 2-Year, 3-Year), as billed per term.
type mrrTermTotal struct {
	BillingTerm   string   `json:"billingTerm"`
	Amount        float64  `json:"amount"`
	Margin        *float64 `json:"margin"`
	Subscriptions int      `json:"subscriptions"`
	// Set only when more than one currency is present: then a term has one row
	// per currency.
	Currency                 string `json:"currency,omitempty"`
	SubscriptionsWithoutCost int    `json:"subscriptionsWithoutCost,omitempty"`
	margin                   mrrMargin
}

type mrrResult struct {
	Totals    mrrTotals       `json:"totals"`
	ByProduct []mrrProductRow `json:"byProduct"`
}

// mrrIsActive reports a subscription that bills (pax8SubscriptionBills, the
// documented Subscription.status enum; anything else is not active).
func mrrIsActive(status string) bool {
	bills, _ := pax8SubscriptionBills(status)
	return bills
}

// mrrMarginText is a row's margin: its figure, or unknown, and how many of its
// subscriptions carry no partner cost.
func mrrMarginText(margin *float64, withoutCost int) string {
	text := "unknown"
	if margin != nil {
		text = fmt.Sprintf("%.2f", *margin)
	}
	if withoutCost > 0 {
		text += fmt.Sprintf(", %d without a partner cost", withoutCost)
	}
	return text
}

// pp:data-source local
func newNovelMrrCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	var includeAll bool

	cmd := &cobra.Command{
		Use:         "mrr",
		Short:       "Compute monthly recurring revenue and margin from subscriptions and product pricing, trended across syncs.",
		Annotations: map[string]string{"mcp:read-only": "true"},
		Long: `Compute monthly recurring revenue (MRR) and margin from the local store.

MRR is the sum of subscription price x quantity for billing subscriptions
(Active, Activated, Trial, PendingCancel) on the Monthly billing term; margin subtracts
partner cost. Broken down by product. Annual, 2-Year and 3-Year subscriptions
are reported beside MRR as per-term amounts (otherCadence), never summed into
it; One-Time, Activation and Trial terms do not recur and are counted only.
Run 'pax8-cli sync' first.

A subscription with no partner cost has an unknown margin: margin covers only the
subscriptions that carry one, subscriptionsWithoutCost counts the rest, and margin
is null when none carries it. Currencies are never added: with more than one,
totals.mrr and totals.margin are null, totals.byCurrency has one total per currency,
and each byProduct and otherCadence row names its currency.

Use this command for monthly recurring revenue and margin totals, with a per-product breakdown in the output.
Do NOT use this command for what changed week-over-week; use 'since' instead.
Do NOT use this command for per-customer spend rollups; use 'spend' instead.`,
		Example: `  # MRR summary
  pax8-cli mrr

  # Just the headline numbers for an agent
  pax8-cli mrr --agent --select totals.mrr,totals.margin`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			if dryRunOK(flags) {
				return nil
			}
			out := cmd.OutOrStdout()
			if dbPath == "" {
				dbPath = defaultDBPath("pax8-cli")
			}
			db, err := store.OpenWithContext(cmd.Context(), dbPath)
			if err != nil {
				return fmt.Errorf("opening local database: %w\nRun 'pax8-cli sync' first.", err)
			}
			defer db.Close()
			maybeEmitSyncHints(cmd, db, "subscriptions", flags.maxAge)

			products, err := pax8ListObjects(db, "products")
			if err != nil {
				return fmt.Errorf("reading products: %w", err)
			}
			nameByID := pax8NameByID(products, []string{"id", "productId"}, []string{"name", "productName"})

			subs, err := pax8ListObjects(db, "subscriptions")
			if err != nil {
				return fmt.Errorf("reading subscriptions: %w", err)
			}

			// One accumulator per currency: money is never added across
			// currencyCode (pax8Currency).
			type rowKey struct{ name, currency string }
			byProduct := map[rowKey]*mrrProductRow{}
			otherByTerm := map[rowKey]*mrrTermTotal{}
			byCurrency := map[string]*mrrCurrencyTotal{}
			var res mrrResult
			res.Totals.OtherCadence = []mrrTermTotal{}
			for _, s := range subs {
				status := pax8FieldStr(s, "status")
				bills, knownStatus := pax8SubscriptionBills(status)
				if !includeAll && !bills {
					if !knownStatus {
						res.Totals.UnclassifiedSubscriptions++
					}
					continue
				}
				term := pax8FieldStr(s, "billingTerm", "billing_term")
				otherCadence := false
				switch pax8BillingTermClass(term) {
				case termNonRecurring:
					res.Totals.NonRecurringSubscriptions++
					continue
				case termUnclassified:
					res.Totals.UnclassifiedSubscriptions++
					continue
				case termOtherCadence:
					otherCadence = true
				}
				currency := pax8Currency(s)
				total := byCurrency[currency]
				if total == nil {
					total = &mrrCurrencyTotal{Currency: currency}
					byCurrency[currency] = total
				}
				price, _ := pax8FieldNum(s, "price")
				qty, hasQty := pax8FieldNum(s, "quantity")
				if !hasQty {
					qty = 1
				}
				cost, hasCost := pax8FieldNum(s, "partnerCost", "partner_cost")
				lineAmount := price * qty
				lineMargin := (price - cost) * qty

				if otherCadence {
					key := rowKey{term, currency}
					tt := otherByTerm[key]
					if tt == nil {
						tt = &mrrTermTotal{BillingTerm: term, Currency: currency}
						otherByTerm[key] = tt
					}
					tt.Amount += lineAmount
					tt.margin.add(lineMargin, hasCost)
					tt.Subscriptions++
					continue
				}

				pid := pax8FieldStr(s, "productId", "product_id")
				key := rowKey{pid, currency}
				row := byProduct[key]
				if row == nil {
					row = &mrrProductRow{ProductID: pid, ProductName: nameByID[pid], Currency: currency}
					byProduct[key] = row
				}
				row.Quantity += qty
				row.MRR += lineAmount
				row.margin.add(lineMargin, hasCost)

				total.MRR += lineAmount
				total.margin.add(lineMargin, hasCost)
				total.ActiveSubscriptions++
			}

			currencies := make([]mrrCurrencyTotal, 0, len(byCurrency))
			for _, t := range byCurrency {
				t.Margin, t.SubscriptionsWithoutCost = t.margin.figure(), t.margin.lacking
				currencies = append(currencies, *t)
				res.Totals.ActiveSubscriptions += t.ActiveSubscriptions
				res.Totals.SubscriptionsWithoutCost += t.SubscriptionsWithoutCost
			}
			sort.Slice(currencies, func(i, j int) bool { return currencies[i].Currency < currencies[j].Currency })
			multiCurrency := len(currencies) > 1
			switch len(currencies) {
			case 0:
				var mrr, margin float64
				res.Totals.MRR, res.Totals.Margin = &mrr, &margin
			case 1:
				res.Totals.MRR, res.Totals.Margin, res.Totals.Currency = &currencies[0].MRR, currencies[0].Margin, currencies[0].Currency
			default:
				res.Totals.ByCurrency = currencies
			}
			// The currency a row is printed in. With one currency a row does not
			// repeat it, which is the shape that case has always had.
			label := func(currency string) string {
				if !multiCurrency {
					return res.Totals.Currency
				}
				if currency == "" {
					return "(no currency)"
				}
				return currency
			}

			for _, tt := range otherByTerm {
				tt.Margin, tt.SubscriptionsWithoutCost = tt.margin.figure(), tt.margin.lacking
				if !multiCurrency {
					tt.Currency = ""
				}
				res.Totals.OtherCadence = append(res.Totals.OtherCadence, *tt)
			}
			sort.Slice(res.Totals.OtherCadence, func(i, j int) bool {
				a, b := res.Totals.OtherCadence[i], res.Totals.OtherCadence[j]
				if a.BillingTerm != b.BillingTerm {
					return a.BillingTerm < b.BillingTerm
				}
				return a.Currency < b.Currency
			})
			res.ByProduct = make([]mrrProductRow, 0, len(byProduct))
			for _, r := range byProduct {
				r.Margin, r.SubscriptionsWithoutCost = r.margin.figure(), r.margin.lacking
				if !multiCurrency {
					r.Currency = ""
				}
				res.ByProduct = append(res.ByProduct, *r)
			}
			sort.Slice(res.ByProduct, func(i, j int) bool {
				a, b := res.ByProduct[i], res.ByProduct[j]
				if a.MRR != b.MRR {
					return a.MRR > b.MRR
				}
				if a.ProductID != b.ProductID {
					return a.ProductID < b.ProductID
				}
				return a.Currency < b.Currency
			})

			if flags.asJSON {
				return pax8PrintNovelJSON(out, res, flags) // pax8 handfix: novel-money-shapes (every row field kept under --agent)
			}
			if len(currencies) == 0 {
				currencies = []mrrCurrencyTotal{{Margin: res.Totals.Margin}}
			}
			for _, t := range currencies {
				fmt.Fprintf(out, "MRR:    %s\n", pax8Money(t.MRR, label(t.Currency)))
				switch {
				case t.Margin == nil:
					fmt.Fprintf(out, "Margin: unknown: %d subscription(s) carry no partner cost\n", t.SubscriptionsWithoutCost)
				case t.SubscriptionsWithoutCost > 0:
					fmt.Fprintf(out, "Margin: %s over the subscriptions with a partner cost; %d subscription(s) carry no partner cost\n", pax8Money(*t.Margin, label(t.Currency)), t.SubscriptionsWithoutCost)
				default:
					fmt.Fprintf(out, "Margin: %s\n", pax8Money(*t.Margin, label(t.Currency)))
				}
			}
			fmt.Fprintf(out, "Active subscriptions: %d\n", res.Totals.ActiveSubscriptions)
			for _, tt := range res.Totals.OtherCadence {
				fmt.Fprintf(out, "Not in MRR: %d %s subscription(s), %s per term (margin %s)\n", tt.Subscriptions, tt.BillingTerm, pax8Money(tt.Amount, label(tt.Currency)), mrrMarginText(tt.Margin, tt.SubscriptionsWithoutCost))
			}
			if n := res.Totals.NonRecurringSubscriptions; n > 0 {
				fmt.Fprintf(out, "Not in MRR: %d non-recurring subscription(s) (One-Time, Activation or Trial term)\n", n)
			}
			if n := res.Totals.UnclassifiedSubscriptions; n > 0 {
				fmt.Fprintf(out, "Not in MRR: %d subscription(s) with a status or billing term Pax8 does not document\n", n)
			}
			fmt.Fprintln(out)
			if len(res.ByProduct) == 0 {
				fmt.Fprintln(out, "No subscriptions in local store. Run 'pax8-cli sync' first.")
				return nil
			}
			fmt.Fprintln(out, "Product\tQty\tMRR\tMargin")
			fmt.Fprintln(out, "-------\t---\t---\t------")
			for _, r := range res.ByProduct {
				name := r.ProductName
				if name == "" {
					name = r.ProductID
				}
				amount := fmt.Sprintf("%.2f", r.MRR)
				if multiCurrency {
					amount = pax8Money(r.MRR, label(r.Currency))
				}
				fmt.Fprintf(out, "%s\t%.0f\t%s\t%s\n", name, r.Quantity, amount, mrrMarginText(r.Margin, r.SubscriptionsWithoutCost))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	cmd.Flags().BoolVar(&includeAll, "all", false, "Include cancelled/inactive subscriptions in the totals")
	return cmd
}
