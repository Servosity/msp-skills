// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel feature (Phase 3): MRR and margin. Not generator-managed.

package cli

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"
	"pax8-pp-cli/internal/store"
)

type mrrProductRow struct {
	ProductID   string  `json:"productId"`
	ProductName string  `json:"productName"`
	Quantity    float64 `json:"quantity"`
	MRR         float64 `json:"mrr"`
	Margin      float64 `json:"margin"`
}

type mrrTotals struct {
	MRR                 float64 `json:"mrr"`
	Margin              float64 `json:"margin"`
	ActiveSubscriptions int     `json:"activeSubscriptions"`
	Currency            string  `json:"currency,omitempty"`
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
	BillingTerm   string  `json:"billingTerm"`
	Amount        float64 `json:"amount"`
	Margin        float64 `json:"margin"`
	Subscriptions int     `json:"subscriptions"`
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

			byProduct := map[string]*mrrProductRow{}
			var res mrrResult
			res.Totals.OtherCadence = []mrrTermTotal{}
			otherByTerm := map[string]*mrrTermTotal{}
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
				switch pax8BillingTermClass(term) {
				case termNonRecurring:
					res.Totals.NonRecurringSubscriptions++
					continue
				case termUnclassified:
					res.Totals.UnclassifiedSubscriptions++
					continue
				case termOtherCadence:
					p, _ := pax8FieldNum(s, "price")
					q, hasQ := pax8FieldNum(s, "quantity")
					if !hasQ {
						q = 1
					}
					c, _ := pax8FieldNum(s, "partnerCost", "partner_cost")
					tt := otherByTerm[term]
					if tt == nil {
						tt = &mrrTermTotal{BillingTerm: term}
						otherByTerm[term] = tt
					}
					tt.Amount += p * q
					tt.Margin += (p - c) * q
					tt.Subscriptions++
					continue
				}
				price, _ := pax8FieldNum(s, "price")
				qty, hasQty := pax8FieldNum(s, "quantity")
				if !hasQty {
					qty = 1
				}
				cost, _ := pax8FieldNum(s, "partnerCost", "partner_cost")
				lineMRR := price * qty
				lineMargin := (price - cost) * qty

				pid := pax8FieldStr(s, "productId", "product_id")
				row := byProduct[pid]
				if row == nil {
					row = &mrrProductRow{ProductID: pid, ProductName: nameByID[pid]}
					byProduct[pid] = row
				}
				row.Quantity += qty
				row.MRR += lineMRR
				row.Margin += lineMargin

				res.Totals.MRR += lineMRR
				res.Totals.Margin += lineMargin
				res.Totals.ActiveSubscriptions++
				if res.Totals.Currency == "" {
					res.Totals.Currency = pax8FieldStr(s, "currencyCode", "currency_code")
				}
			}

			for _, tt := range otherByTerm {
				res.Totals.OtherCadence = append(res.Totals.OtherCadence, *tt)
			}
			sort.Slice(res.Totals.OtherCadence, func(i, j int) bool {
				return res.Totals.OtherCadence[i].BillingTerm < res.Totals.OtherCadence[j].BillingTerm
			})
			res.ByProduct = make([]mrrProductRow, 0, len(byProduct))
			for _, r := range byProduct {
				res.ByProduct = append(res.ByProduct, *r)
			}
			sort.Slice(res.ByProduct, func(i, j int) bool { return res.ByProduct[i].MRR > res.ByProduct[j].MRR })

			if flags.asJSON {
				return printJSONFiltered(out, res, flags)
			}
			fmt.Fprintf(out, "MRR:    %.2f %s\n", res.Totals.MRR, res.Totals.Currency)
			fmt.Fprintf(out, "Margin: %.2f %s\n", res.Totals.Margin, res.Totals.Currency)
			fmt.Fprintf(out, "Active subscriptions: %d\n", res.Totals.ActiveSubscriptions)
			for _, tt := range res.Totals.OtherCadence {
				fmt.Fprintf(out, "Not in MRR: %d %s subscription(s), %.2f %s per term (margin %.2f)\n", tt.Subscriptions, tt.BillingTerm, tt.Amount, res.Totals.Currency, tt.Margin)
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
				fmt.Fprintf(out, "%s\t%.0f\t%.2f\t%.2f\n", name, r.Quantity, r.MRR, r.Margin)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	cmd.Flags().BoolVar(&includeAll, "all", false, "Include cancelled/inactive subscriptions in the totals")
	return cmd
}
