// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

// Package insights holds the pure, store-agnostic computation behind
// sherweb-cli's transcendence commands (margin, drift, orphans, fleet-subs,
// right-size, amend-preview). Every function takes plain Go values and returns
// plain Go values so the cross-entity joins are fully unit-testable without a
// database or network. Command files in internal/cli load rows from the local
// SQLite store, decode them into these inputs, and render the outputs.
package insights

import (
	"math"
	"sort"
	"strings"
)

// Charge is one payable (you→Sherweb) or receivable (customer→you) charge line.
type Charge struct {
	CustomerID   string
	CustomerName string
	ProductName  string
	SKU          string
	ChargeName   string
	BillingCycle string
	Quantity     float64
	SubTotal     float64
	Currency     string
}

// Subscription is a customer subscription, optionally enriched with a unit price
// resolved from the pricing-information endpoint.
type Subscription struct {
	ID           string
	CustomerID   string
	CustomerName string
	ProductName  string
	SKU          string
	Quantity     float64
	BillingCycle string
	Status       string
	UnitPrice    float64
	Currency     string
}

// Usage is metered consumption for a subscription/SKU on a platform.
type Usage struct {
	CustomerID     string
	SubscriptionID string
	SKU            string
	Used           float64
}

// customerLabel falls back to the id when no display name is present.
func customerLabel(id, name string) string {
	if strings.TrimSpace(name) != "" {
		return name
	}
	return id
}

func round2(f float64) float64 { return math.Round(f*100) / 100 }

// ---------------------------------------------------------------------------
// margin
// ---------------------------------------------------------------------------

// CustomerMargin is net margin for one customer over a period: receivable
// (what the customer owes you) minus payable (what you owe Sherweb).
type CustomerMargin struct {
	CustomerID   string  `json:"customerId"`
	CustomerName string  `json:"customerName"`
	Payable      float64 `json:"payable"`
	Receivable   float64 `json:"receivable"`
	Margin       float64 `json:"margin"`
	MarginPct    float64 `json:"marginPct"`
}

// ComputeMargin groups payable and receivable charges by customer and returns
// per-customer margin, sorted by margin ascending (worst/leakiest first) so the
// rows that need attention surface at the top. A customer that appears in only
// one side still produces a row (the other side is 0).
func ComputeMargin(payable, receivable []Charge) []CustomerMargin {
	type agg struct {
		name       string
		payable    float64
		receivable float64
	}
	by := map[string]*agg{}
	get := func(id, name string) *agg {
		a, ok := by[id]
		if !ok {
			a = &agg{name: name}
			by[id] = a
		}
		if a.name == "" && name != "" {
			a.name = name
		}
		return a
	}
	for _, c := range payable {
		get(c.CustomerID, c.CustomerName).payable += c.SubTotal
	}
	for _, c := range receivable {
		get(c.CustomerID, c.CustomerName).receivable += c.SubTotal
	}
	out := make([]CustomerMargin, 0, len(by))
	for id, a := range by {
		margin := a.receivable - a.payable
		pct := 0.0
		if a.receivable != 0 {
			pct = margin / a.receivable * 100
		}
		out = append(out, CustomerMargin{
			CustomerID:   id,
			CustomerName: customerLabel(id, a.name),
			Payable:      round2(a.payable),
			Receivable:   round2(a.receivable),
			Margin:       round2(margin),
			MarginPct:    round2(pct),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Margin != out[j].Margin {
			return out[i].Margin < out[j].Margin
		}
		return out[i].CustomerID < out[j].CustomerID
	})
	return out
}

// ---------------------------------------------------------------------------
// drift
// ---------------------------------------------------------------------------

// DriftEntry is one charge that appeared, vanished, or changed between the prior
// and current payable-charges snapshot.
type DriftEntry struct {
	Key             string  `json:"key"`
	CustomerName    string  `json:"customerName"`
	ProductName     string  `json:"productName"`
	Change          string  `json:"change"` // appeared | vanished | changed
	PriorSubTotal   float64 `json:"priorSubTotal"`
	CurrentSubTotal float64 `json:"currentSubTotal"`
	Delta           float64 `json:"delta"`
}

// chargeKey identifies "the same charge" across snapshots. ChargeName + SKU +
// customer is stable across periods where chargeId is not.
func chargeKey(c Charge) string {
	return strings.Join([]string{c.CustomerID, c.SKU, c.ChargeName}, "|")
}

// ComputeDrift diffs the current payable snapshot against the prior one. Charges
// present only in current => appeared; only in prior => vanished; in both with a
// changed subtotal => changed. Identical charges are omitted. Sorted by absolute
// delta descending so the biggest swings surface first.
func ComputeDrift(current, prior []Charge) []DriftEntry {
	cur := map[string]Charge{}
	for _, c := range current {
		k := chargeKey(c)
		e := cur[k]
		e.SubTotal += c.SubTotal
		e.CustomerID, e.CustomerName, e.ProductName, e.SKU, e.ChargeName = c.CustomerID, c.CustomerName, c.ProductName, c.SKU, c.ChargeName
		cur[k] = e
	}
	pri := map[string]Charge{}
	for _, c := range prior {
		k := chargeKey(c)
		e := pri[k]
		e.SubTotal += c.SubTotal
		e.CustomerID, e.CustomerName, e.ProductName, e.SKU, e.ChargeName = c.CustomerID, c.CustomerName, c.ProductName, c.SKU, c.ChargeName
		pri[k] = e
	}
	seen := map[string]bool{}
	out := []DriftEntry{}
	add := func(k string) {
		if seen[k] {
			return
		}
		seen[k] = true
		c, inCur := cur[k]
		p, inPri := pri[k]
		var change string
		switch {
		case inCur && !inPri:
			change = "appeared"
		case !inCur && inPri:
			change = "vanished"
		case round2(c.SubTotal) == round2(p.SubTotal):
			return // unchanged
		default:
			change = "changed"
		}
		ref := c
		if !inCur {
			ref = p
		}
		out = append(out, DriftEntry{
			Key:             k,
			CustomerName:    customerLabel(ref.CustomerID, ref.CustomerName),
			ProductName:     ref.ProductName,
			Change:          change,
			PriorSubTotal:   round2(p.SubTotal),
			CurrentSubTotal: round2(c.SubTotal),
			Delta:           round2(c.SubTotal - p.SubTotal),
		})
	}
	for k := range cur {
		add(k)
	}
	for k := range pri {
		add(k)
	}
	sort.Slice(out, func(i, j int) bool {
		ai, aj := math.Abs(out[i].Delta), math.Abs(out[j].Delta)
		if ai != aj {
			return ai > aj
		}
		return out[i].Key < out[j].Key
	})
	return out
}

// ---------------------------------------------------------------------------
// orphans
// ---------------------------------------------------------------------------

// Orphan is an active subscription whose customer has no receivable charges in
// the latest period — a subscription you pay Sherweb for but are not billing on.
type Orphan struct {
	SubscriptionID string  `json:"subscriptionId"`
	CustomerID     string  `json:"customerId"`
	CustomerName   string  `json:"customerName"`
	ProductName    string  `json:"productName"`
	Quantity       float64 `json:"quantity"`
	Reason         string  `json:"reason"`
}

func isActive(status string) bool {
	s := strings.ToLower(strings.TrimSpace(status))
	return s == "" || s == "active" || s == "enabled" || s == "provisioned"
}

// FindOrphans returns active subscriptions whose customer billed zero receivable
// charges. Sorted by customer then product for stable output.
func FindOrphans(subs []Subscription, receivable []Charge) []Orphan {
	billed := map[string]bool{}
	for _, c := range receivable {
		if c.SubTotal != 0 || c.Quantity != 0 {
			billed[c.CustomerID] = true
		}
	}
	out := []Orphan{}
	for _, s := range subs {
		if !isActive(s.Status) {
			continue
		}
		if billed[s.CustomerID] {
			continue
		}
		out = append(out, Orphan{
			SubscriptionID: s.ID,
			CustomerID:     s.CustomerID,
			CustomerName:   customerLabel(s.CustomerID, s.CustomerName),
			ProductName:    s.ProductName,
			Quantity:       s.Quantity,
			Reason:         "active subscription with no receivable charges in the latest period",
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CustomerID != out[j].CustomerID {
			return out[i].CustomerID < out[j].CustomerID
		}
		return out[i].ProductName < out[j].ProductName
	})
	return out
}

// ---------------------------------------------------------------------------
// fleet-subs
// ---------------------------------------------------------------------------

// FleetRow is a cross-customer roll-up of subscriptions for one product/SKU.
type FleetRow struct {
	ProductName       string  `json:"productName"`
	SKU               string  `json:"sku"`
	TotalSeats        float64 `json:"totalSeats"`
	CustomerCount     int     `json:"customerCount"`
	SubscriptionCount int     `json:"subscriptionCount"`
}

// RollupFleet aggregates subscriptions across all customers by product+SKU.
// productFilter / skuFilter are optional case-insensitive substring filters.
// Sorted by total seats descending.
func RollupFleet(subs []Subscription, productFilter, skuFilter string) []FleetRow {
	pf := strings.ToLower(strings.TrimSpace(productFilter))
	sf := strings.ToLower(strings.TrimSpace(skuFilter))
	type agg struct {
		product   string
		sku       string
		seats     float64
		subs      int
		customers map[string]bool
	}
	by := map[string]*agg{}
	for _, s := range subs {
		if pf != "" && !strings.Contains(strings.ToLower(s.ProductName), pf) {
			continue
		}
		if sf != "" && !strings.Contains(strings.ToLower(s.SKU), sf) {
			continue
		}
		key := s.ProductName + "\x00" + s.SKU
		a, ok := by[key]
		if !ok {
			a = &agg{product: s.ProductName, sku: s.SKU, customers: map[string]bool{}}
			by[key] = a
		}
		a.seats += s.Quantity
		a.subs++
		a.customers[s.CustomerID] = true
	}
	out := make([]FleetRow, 0, len(by))
	for _, a := range by {
		out = append(out, FleetRow{
			ProductName:       a.product,
			SKU:               a.sku,
			TotalSeats:        a.seats,
			CustomerCount:     len(a.customers),
			SubscriptionCount: a.subs,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].TotalSeats != out[j].TotalSeats {
			return out[i].TotalSeats > out[j].TotalSeats
		}
		return out[i].ProductName < out[j].ProductName
	})
	return out
}

// ---------------------------------------------------------------------------
// right-size
// ---------------------------------------------------------------------------

// RightSizeRow compares seats paid (subscription quantity) against metered seats
// used for one subscription.
type RightSizeRow struct {
	SubscriptionID string  `json:"subscriptionId"`
	CustomerID     string  `json:"customerId"`
	CustomerName   string  `json:"customerName"`
	ProductName    string  `json:"productName"`
	SeatsPaid      float64 `json:"seatsPaid"`
	SeatsUsed      float64 `json:"seatsUsed"`
	Delta          float64 `json:"delta"` // paid - used; positive = over-provisioned
	Recommendation string  `json:"recommendation"`
}

// RightSize joins subscriptions to usage (by subscription id first, then SKU)
// and flags over/under-provisioning. Only subscriptions with a usage signal are
// returned. Sorted by absolute delta descending.
func RightSize(subs []Subscription, usage []Usage) []RightSizeRow {
	bySub := map[string]float64{}
	bySKU := map[string]float64{}
	for _, u := range usage {
		if u.SubscriptionID != "" {
			bySub[u.SubscriptionID] += u.Used
		}
		if u.SKU != "" {
			bySKU[u.SKU] += u.Used
		}
	}
	out := []RightSizeRow{}
	for _, s := range subs {
		used, ok := bySub[s.ID]
		if !ok {
			used, ok = bySKU[s.SKU]
		}
		if !ok {
			continue // no usage signal for this subscription
		}
		delta := s.Quantity - used
		rec := "ok"
		switch {
		case delta > 0:
			rec = "over-provisioned"
		case delta < 0:
			rec = "under-provisioned"
		}
		out = append(out, RightSizeRow{
			SubscriptionID: s.ID,
			CustomerID:     s.CustomerID,
			CustomerName:   customerLabel(s.CustomerID, s.CustomerName),
			ProductName:    s.ProductName,
			SeatsPaid:      s.Quantity,
			SeatsUsed:      used,
			Delta:          round2(delta),
			Recommendation: rec,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		ai, aj := math.Abs(out[i].Delta), math.Abs(out[j].Delta)
		if ai != aj {
			return ai > aj
		}
		return out[i].SubscriptionID < out[j].SubscriptionID
	})
	return out
}

// ---------------------------------------------------------------------------
// amend-preview
// ---------------------------------------------------------------------------

// AmendPreview is the projected cost impact of a seat-quantity amendment.
type AmendPreview struct {
	SubscriptionID  string  `json:"subscriptionId"`
	ProductName     string  `json:"productName"`
	CurrentQuantity float64 `json:"currentQuantity"`
	NewQuantity     float64 `json:"newQuantity"`
	DeltaQuantity   float64 `json:"deltaQuantity"`
	UnitPrice       float64 `json:"unitPrice"`
	Currency        string  `json:"currency"`
	CurrentMonthly  float64 `json:"currentMonthly"`
	NewMonthly      float64 `json:"newMonthly"`
	MonthlyDelta    float64 `json:"monthlyDelta"`
}

// PreviewAmendment computes the cost delta of changing a subscription to
// newQty seats, using the subscription's stored unit price.
func PreviewAmendment(sub Subscription, newQty float64) AmendPreview {
	delta := newQty - sub.Quantity
	return AmendPreview{
		SubscriptionID:  sub.ID,
		ProductName:     sub.ProductName,
		CurrentQuantity: sub.Quantity,
		NewQuantity:     newQty,
		DeltaQuantity:   delta,
		UnitPrice:       sub.UnitPrice,
		Currency:        sub.Currency,
		CurrentMonthly:  round2(sub.Quantity * sub.UnitPrice),
		NewMonthly:      round2(newQty * sub.UnitPrice),
		MonthlyDelta:    round2(delta * sub.UnitPrice),
	}
}
