// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel command: one customer's licence book from the local store.
// pp:data-source local

package cli

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

type customerShowSub struct {
	SubscriptionID string  `json:"subscription_id"`
	Subscription   string  `json:"subscription"`
	SKU            string  `json:"sku"`
	Seats          float64 `json:"seats"`
	SeatsKnown     bool    `json:"seats_known"` // false when ION reported no licence count; seats is then 0
	Status         string  `json:"status"`
	RenewsOn       string  `json:"renews_on,omitempty"`
	AutoRenew      string  `json:"auto_renew"`
	MonthlyPrice   float64 `json:"monthly_price"`
	PriceKnown     bool    `json:"price_known"` // false when ION reported no price or only part of it, or the billing cycle is not recognized; monthly_price is then 0
	MonthlyCost    float64 `json:"monthly_cost"`
	CostKnown      bool    `json:"cost_known"` // false when ION reported no cost or only part of it, or the billing cycle is not recognized; monthly_cost is then 0
	Currency       string  `json:"currency"`
}

type customerShowTotal struct {
	Currency      string  `json:"currency"`
	Seats         float64 `json:"seats"`          // subscriptions with a known licence count only
	SeatsComplete bool    `json:"seats_complete"` // false when a subscription in the total has no licence count
	MonthlyPrice  float64 `json:"monthly_price"`  // known prices only
	MonthlyCost   float64 `json:"monthly_cost"`   // known costs only
	Margin        float64 `json:"margin"`         // subscriptions with a known price and cost only
	MarginKnown   bool    `json:"margin_known"`   // false when no subscription has both a known price and cost
	PriceComplete bool    `json:"price_complete"` // false when a subscription in the total has no reported price
	CostComplete  bool    `json:"cost_complete"`  // false when a subscription in the total has no reported cost
	// PriceUnknownCost is the price of subscriptions with no known cost, left
	// out of margin rather than counted at a cost of 0.
	PriceUnknownCost float64 `json:"monthly_price_cost_unknown"`
	// CostUnknownPrice is the cost of subscriptions with no known price, inside
	// monthly_cost and left out of margin.
	CostUnknownPrice float64 `json:"monthly_cost_price_unknown"`
	mPrice, mCost    float64
	nSeats, nPrice   int // contributors with a known seat count, price, cost, and price and cost
	nCost, nPaired   int
}

type customerShowView struct {
	CustomerID    string              `json:"customer_id"`
	Organization  string              `json:"organization"`
	Contact       string              `json:"contact"`
	Email         string              `json:"email"`
	Status        string              `json:"status"`
	Subscriptions []customerShowSub   `json:"subscriptions"`
	Totals        []customerShowTotal `json:"totals"`
	OpenOrders    []stuckOrderRow     `json:"open_orders"`
	RecentChanges []subChangeRow      `json:"recent_changes"`
	Next          string              `json:"next"`
}

// findCustomers resolves a numeric ID exactly, else a case-insensitive
// substring of organization, contact name or email.
func findCustomers(b *ionBook, query string) []ionCustomer {
	q := strings.TrimSpace(query)
	if c, ok := b.Customers[q]; ok {
		return []ionCustomer{c}
	}
	lq := strings.ToLower(q)
	out := make([]ionCustomer, 0)
	for _, c := range b.Customers {
		if strings.Contains(strings.ToLower(c.Organization), lq) || strings.Contains(strings.ToLower(c.ContactName), lq) || strings.EqualFold(c.Email, q) {
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Organization != out[j].Organization {
			return out[i].Organization < out[j].Organization
		}
		return out[i].ID() < out[j].ID()
	})
	return out
}

func customerView(b *ionBook, c ionCustomer, now time.Time) customerShowView {
	id := c.ID() // never empty: an empty ID would match every subscription that lacks one
	v := customerShowView{
		CustomerID: id, Organization: c.Organization, Contact: c.ContactName, Email: c.Email, Status: c.Status,
		Subscriptions: make([]customerShowSub, 0), Totals: make([]customerShowTotal, 0),
		OpenOrders: make([]stuckOrderRow, 0), RecentChanges: make([]subChangeRow, 0),
		Next: "GDAP/MCA status: td-synnex-cli cloud-profiles " + id + " --provider-id 30",
	}
	totals := map[string]*customerShowTotal{}
	for _, s := range b.Subs {
		if id == "" || s.CustomerID != id {
			continue
		}
		m := s.MonthlyMoney()
		row := customerShowSub{
			SubscriptionID: s.ID, Subscription: s.Label(), SKU: s.SKUKey(), Seats: s.Seats.V, SeatsKnown: s.Seats.Set, Status: s.Status,
			AutoRenew: s.AutoRenewState(), MonthlyPrice: round2(m.Price), PriceKnown: m.PriceKnown, MonthlyCost: round2(m.Cost), CostKnown: m.CostKnown, Currency: m.Currency,
		}
		if t := s.RenewOn(); !t.IsZero() {
			row.RenewsOn = t.Format("2006-01-02")
		}
		v.Subscriptions = append(v.Subscriptions, row)
		if s.Inactive() {
			continue
		}
		t := totals[row.Currency]
		if t == nil {
			t = &customerShowTotal{Currency: row.Currency, SeatsComplete: true, PriceComplete: true, CostComplete: true}
			totals[row.Currency] = t
		}
		// An unknown seat count, price or cost is never counted as 0.
		if row.SeatsKnown {
			t.Seats += row.Seats
			t.nSeats++
		}
		if m.PriceKnown {
			t.MonthlyPrice += m.Price
			t.nPrice++
		}
		if m.CostKnown {
			t.MonthlyCost += m.Cost
			t.nCost++
			if !m.PriceKnown {
				t.CostUnknownPrice += m.Cost
			}
		} else if m.PriceKnown {
			t.PriceUnknownCost += m.Price
		}
		if m.PriceKnown && m.CostKnown {
			t.mPrice += m.Price
			t.mCost += m.Cost
			t.nPaired++
		}
		t.SeatsComplete = t.SeatsComplete && row.SeatsKnown
		t.PriceComplete = t.PriceComplete && m.PriceKnown
		t.CostComplete = t.CostComplete && m.CostKnown
	}
	for _, t := range totals {
		t.MonthlyPrice, t.MonthlyCost, t.PriceUnknownCost, t.CostUnknownPrice = round2(t.MonthlyPrice), round2(t.MonthlyCost), round2(t.PriceUnknownCost), round2(t.CostUnknownPrice)
		t.Margin, t.MarginKnown = round2(t.mPrice-t.mCost), t.nPaired > 0
		v.Totals = append(v.Totals, *t)
	}
	sort.Slice(v.Totals, func(i, j int) bool { return v.Totals[i].Currency < v.Totals[j].Currency })
	sort.SliceStable(v.Subscriptions, func(i, j int) bool { return v.Subscriptions[i].Subscription < v.Subscriptions[j].Subscription })
	for _, o := range stuckOrders(b, now, 0, 0) {
		if o.CustomerID == id && strings.HasPrefix(o.Reason, "open") {
			v.OpenOrders = append(v.OpenOrders, o)
		}
	}
	v.RecentChanges = subscriptionChanges(b, now, 30*24*time.Hour, id, false)
	return v
}

func newNovelCustomersShowCmd(flags *rootFlags) *cobra.Command {
	var dbPath string

	cmd := &cobra.Command{
		Use:   "show [customer-id-or-name]",
		Short: "One view of a customer: profile, subscriptions with seats and monthly cost and price, open orders and recent changes",
		Long: "One view of a customer from the local store: profile, every subscription with seats, renewal and monthly " +
			"price and cost, per-currency totals, open orders and the last 30 days of subscription changes (completed and " +
			"in-flight orders). Pass the " +
			"numeric customer ID or part of the organization or contact name; exits 3 when a synced store has no match. " +
			"Microsoft GDAP and MCA status is not included: it is live data, and the output names the cloud-profiles " +
			"command that fetches it.\n\n" +
			"Use this command for everything about one customer in one view. Do NOT use this command to rank all " +
			"customers by margin; use 'margin' instead. Do NOT use it for renewals across the whole book; use " +
			"'subscriptions renewals' instead.",
		Example: strings.Trim(`
  td-synnex-cli customers show 108536 --agent
  td-synnex-cli customers show "SilverLeaf"`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local", "pp:happy-args": "customer=108536", "pp:no-error-path-probe": "true", "pp:typed-exit-codes": "0,3"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "customers show")
			}
			if len(args) == 0 || strings.TrimSpace(args[0]) == "" {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("customer ID or name is required"))
			}
			book, closeDB, err := openIONBook(cmd, flags, dbPath, "customers", "subscriptions", "orders")
			if err != nil {
				return err
			}
			defer closeDB()
			matches := make([]ionCustomer, 0)
			if book != nil {
				matches = findCustomers(book, args[0])
			}
			if len(matches) != 1 {
				candidates := make([]map[string]string, 0, len(matches))
				for _, c := range matches {
					candidates = append(candidates, map[string]string{"customer_id": c.ID(), "organization": c.Organization})
				}
				if !wantsHumanTable(cmd.OutOrStdout(), flags) {
					if err := printIONJSON(cmd.OutOrStdout(), map[string]any{"query": args[0], "matches": candidates}, flags); err != nil {
						return err
					}
				} else if len(matches) == 0 {
					fmt.Fprintf(cmd.OutOrStdout(), "No customer matches %q in the local store.\n", args[0])
				} else {
					fmt.Fprintf(cmd.OutOrStdout(), "%d customers match %q; pass the customer ID:\n", len(matches), args[0])
					for _, c := range candidates {
						fmt.Fprintf(cmd.OutOrStdout(), "  %s  %s\n", c["customer_id"], c["organization"])
					}
				}
				if len(matches) == 0 {
					if book == nil || len(book.Customers) == 0 {
						return nil // never synced: an empty local result, not an error
					}
					return notFoundErr(fmt.Errorf("no customer matches %q in the local store", args[0]))
				}
				return usageErr(fmt.Errorf("%d customers match %q; pass the customer ID", len(matches), args[0]))
			}
			v := customerView(book, matches[0], time.Now().UTC())
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printIONJSON(cmd.OutOrStdout(), v, flags)
			}
			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "%s (customer %s) %s\n%s <%s>\n\n", v.Organization, v.CustomerID, v.Status, v.Contact, v.Email)
			table := make([][]string, 0, len(v.Subscriptions))
			for _, s := range v.Subscriptions {
				table = append(table, []string{s.Subscription, seatsText(s.Seats, s.SeatsKnown), s.Status, s.RenewsOn, s.AutoRenew,
					moneyText(s.MonthlyPrice, s.PriceKnown), moneyText(s.MonthlyCost, s.CostKnown), s.Currency})
			}
			if err := printTable(w, []string{"SUBSCRIPTION", "SEATS", "STATUS", "RENEWS", "AUTO-RENEW", "PRICE/MO", "COST/MO", "CUR"}, table); err != nil {
				return err
			}
			for _, t := range v.Totals {
				note := ""
				if !t.PriceComplete || !t.CostComplete || !t.SeatsComplete {
					note = " (incomplete: ION reported no seat count, price or cost for some subscriptions; each total covers the known amounts and margin covers only subscriptions with a known price and cost)"
				}
				fmt.Fprintf(w, "\nTotal %s: %s seats, %s/mo price, %s/mo cost, %s/mo margin%s", t.Currency, seatsText(t.Seats, t.nSeats > 0),
					moneyText(t.MonthlyPrice, t.nPrice > 0), moneyText(t.MonthlyCost, t.nCost > 0), moneyText(t.Margin, t.nPaired > 0), note)
			}
			dated := 0
			for _, c := range v.RecentChanges {
				if c.When != "" {
					dated++
				}
			}
			undatedNote := ""
			if n := len(v.RecentChanges) - dated; n > 0 {
				undatedNote = fmt.Sprintf(" (+%d with unknown dates)", n)
			}
			fmt.Fprintf(w, "\n\nOpen orders: %d   Changes in the last 30 days: %d%s\n%s\n", len(v.OpenOrders), dated, undatedNote, v.Next)
			return nil
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Local store path (default: the CLI's synced database)")
	return cmd
}
