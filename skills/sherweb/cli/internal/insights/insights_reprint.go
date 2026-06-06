// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package insights

import (
	"math"
	"sort"
	"strings"
)

// ---------------------------------------------------------------------------
// margin-trend
// ---------------------------------------------------------------------------

// MarginTrendPoint is one month's margin for one customer.
type MarginTrendPoint struct {
	Month      string  `json:"month"` // YYYY-MM
	Payable    float64 `json:"payable"`
	Receivable float64 `json:"receivable"`
	Margin     float64 `json:"margin"`
}

// MarginTrendRow is a per-customer margin time-series across monthly snapshot
// pairs, with a headline trend (last margin minus first margin).
type MarginTrendRow struct {
	CustomerID   string             `json:"customerId"`
	CustomerName string             `json:"customerName"`
	Points       []MarginTrendPoint `json:"points"`
	FirstMargin  float64            `json:"firstMargin"`
	LastMargin   float64            `json:"lastMargin"`
	Trend        float64            `json:"trend"` // lastMargin - firstMargin
}

// ComputeMarginTrend builds a per-customer margin time-series from charges
// grouped by month (YYYY-MM). months must be sorted ascending and should only
// contain months where both a payable and a receivable snapshot exist, so a
// missing side never masquerades as a 100%-loss month. Rows sort by trend
// ascending (steepest decline first) so sliding accounts surface at the top.
func ComputeMarginTrend(months []string, payableByMonth, receivableByMonth map[string][]Charge) []MarginTrendRow {
	type series struct {
		name   string
		points map[string]*MarginTrendPoint
	}
	by := map[string]*series{}
	get := func(id, name string) *series {
		s, ok := by[id]
		if !ok {
			s = &series{points: map[string]*MarginTrendPoint{}}
			by[id] = s
		}
		if s.name == "" && name != "" {
			s.name = name
		}
		return s
	}
	point := func(s *series, month string) *MarginTrendPoint {
		p, ok := s.points[month]
		if !ok {
			p = &MarginTrendPoint{Month: month}
			s.points[month] = p
		}
		return p
	}
	for _, month := range months {
		for _, c := range payableByMonth[month] {
			point(get(c.CustomerID, c.CustomerName), month).Payable += c.SubTotal
		}
		for _, c := range receivableByMonth[month] {
			point(get(c.CustomerID, c.CustomerName), month).Receivable += c.SubTotal
		}
	}
	out := make([]MarginTrendRow, 0, len(by))
	for id, s := range by {
		row := MarginTrendRow{CustomerID: id, CustomerName: customerLabel(id, s.name)}
		for _, month := range months {
			p, ok := s.points[month]
			if !ok {
				continue
			}
			p.Payable = round2(p.Payable)
			p.Receivable = round2(p.Receivable)
			p.Margin = round2(p.Receivable - p.Payable)
			row.Points = append(row.Points, *p)
		}
		if len(row.Points) == 0 {
			continue
		}
		row.FirstMargin = row.Points[0].Margin
		row.LastMargin = row.Points[len(row.Points)-1].Margin
		row.Trend = round2(row.LastMargin - row.FirstMargin)
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Trend != out[j].Trend {
			return out[i].Trend < out[j].Trend
		}
		return out[i].CustomerID < out[j].CustomerID
	})
	return out
}

// ---------------------------------------------------------------------------
// sub-changes
// ---------------------------------------------------------------------------

// SubChange is one subscription that was added, removed, or quantity-changed
// between two subscription snapshots.
type SubChange struct {
	SubscriptionID  string  `json:"subscriptionId"`
	CustomerID      string  `json:"customerId"`
	CustomerName    string  `json:"customerName"`
	ProductName     string  `json:"productName"`
	SKU             string  `json:"sku"`
	Change          string  `json:"change"` // added | removed | quantity-changed
	PriorQuantity   float64 `json:"priorQuantity"`
	CurrentQuantity float64 `json:"currentQuantity"`
	QuantityDelta   float64 `json:"quantityDelta"`
}

// subKey identifies "the same subscription" across snapshots: the subscription
// id when present, otherwise customer+SKU.
func subKey(s Subscription) string {
	if strings.TrimSpace(s.ID) != "" {
		return s.ID
	}
	return s.CustomerID + "|" + s.SKU
}

// DiffSubscriptions diffs the current subscription snapshot against the prior
// one, fleet-wide. Subscriptions present only in current => added; only in
// prior => removed; in both with a different quantity => quantity-changed.
// Unchanged subscriptions are omitted. Sorted by absolute quantity delta
// descending so the biggest seat swings surface first.
func DiffSubscriptions(current, prior []Subscription) []SubChange {
	cur := map[string]Subscription{}
	for _, s := range current {
		cur[subKey(s)] = s
	}
	pri := map[string]Subscription{}
	for _, s := range prior {
		pri[subKey(s)] = s
	}
	out := []SubChange{}
	emit := func(s Subscription, change string, priorQty, curQty float64) {
		out = append(out, SubChange{
			SubscriptionID:  s.ID,
			CustomerID:      s.CustomerID,
			CustomerName:    customerLabel(s.CustomerID, s.CustomerName),
			ProductName:     s.ProductName,
			SKU:             s.SKU,
			Change:          change,
			PriorQuantity:   priorQty,
			CurrentQuantity: curQty,
			QuantityDelta:   round2(curQty - priorQty),
		})
	}
	for k, c := range cur {
		p, ok := pri[k]
		switch {
		case !ok:
			emit(c, "added", 0, c.Quantity)
		case p.Quantity != c.Quantity:
			emit(c, "quantity-changed", p.Quantity, c.Quantity)
		}
	}
	for k, p := range pri {
		if _, ok := cur[k]; !ok {
			emit(p, "removed", p.Quantity, 0)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		di, dj := math.Abs(out[i].QuantityDelta), math.Abs(out[j].QuantityDelta)
		if di != dj {
			return di > dj
		}
		if out[i].CustomerID != out[j].CustomerID {
			return out[i].CustomerID < out[j].CustomerID
		}
		return out[i].SubscriptionID < out[j].SubscriptionID
	})
	return out
}

// ---------------------------------------------------------------------------
// usage-leak
// ---------------------------------------------------------------------------

// UsageLeak is metered platform usage with no matching receivable charge —
// consumption the MSP absorbs instead of billing.
type UsageLeak struct {
	CustomerID     string  `json:"customerId"`
	CustomerName   string  `json:"customerName"`
	SubscriptionID string  `json:"subscriptionId"`
	ProductName    string  `json:"productName"`
	SKU            string  `json:"sku"`
	Used           float64 `json:"used"`
	Reason         string  `json:"reason"` // no-receivable-for-sku | no-receivable-for-customer
}

// FindUsageLeaks anti-joins metered usage against receivable charges. A usage
// row leaks when its customer+SKU pair has no receivable charge; rows whose SKU
// cannot be resolved only leak when the customer has no receivable charges at
// all (a missing SKU alone is not proof). Subscriptions enrich product and
// customer names. Sorted by units used descending so the biggest absorbed
// consumption surfaces first.
func FindUsageLeaks(usage []Usage, receivable []Charge, subs []Subscription) []UsageLeak {
	billedSKU := map[string]bool{}
	billedCustomer := map[string]bool{}
	for _, c := range receivable {
		billedCustomer[c.CustomerID] = true
		if c.SKU != "" {
			billedSKU[c.CustomerID+"|"+strings.ToLower(c.SKU)] = true
		}
	}
	subByID := map[string]Subscription{}
	for _, s := range subs {
		if s.ID != "" {
			subByID[s.ID] = s
		}
	}
	out := []UsageLeak{}
	for _, u := range usage {
		if u.Used <= 0 {
			continue
		}
		sku := u.SKU
		sub, hasSub := subByID[u.SubscriptionID]
		if sku == "" && hasSub {
			sku = sub.SKU
		}
		leak := UsageLeak{
			CustomerID:     u.CustomerID,
			SubscriptionID: u.SubscriptionID,
			SKU:            sku,
			Used:           u.Used,
		}
		if hasSub {
			leak.ProductName = sub.ProductName
			leak.CustomerName = customerLabel(u.CustomerID, sub.CustomerName)
		} else {
			leak.CustomerName = u.CustomerID
		}
		switch {
		case sku != "" && billedSKU[u.CustomerID+"|"+strings.ToLower(sku)]:
			continue // billed — not a leak
		case sku != "":
			leak.Reason = "no-receivable-for-sku"
		case billedCustomer[u.CustomerID]:
			continue // SKU unresolved and customer is billed for something — not provable
		default:
			leak.Reason = "no-receivable-for-customer"
		}
		out = append(out, leak)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Used != out[j].Used {
			return out[i].Used > out[j].Used
		}
		if out[i].CustomerID != out[j].CustomerID {
			return out[i].CustomerID < out[j].CustomerID
		}
		return out[i].SubscriptionID < out[j].SubscriptionID
	})
	return out
}
