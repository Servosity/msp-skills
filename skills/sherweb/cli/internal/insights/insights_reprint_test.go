// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package insights

import "testing"

func TestComputeMarginTrend(t *testing.T) {
	months := []string{"2026-03", "2026-04"}
	payable := map[string][]Charge{
		"2026-03": {{CustomerID: "c1", CustomerName: "Acme", SubTotal: 100}},
		"2026-04": {{CustomerID: "c1", CustomerName: "Acme", SubTotal: 150}},
	}
	receivable := map[string][]Charge{
		"2026-03": {{CustomerID: "c1", CustomerName: "Acme", SubTotal: 180}},
		"2026-04": {{CustomerID: "c1", CustomerName: "Acme", SubTotal: 170}},
	}
	rows := ComputeMarginTrend(months, payable, receivable)
	if len(rows) != 1 {
		t.Fatalf("want 1 row, got %d", len(rows))
	}
	r := rows[0]
	if r.CustomerName != "Acme" || len(r.Points) != 2 {
		t.Fatalf("unexpected row: %+v", r)
	}
	if r.FirstMargin != 80 || r.LastMargin != 20 || r.Trend != -60 {
		t.Errorf("margins: first=%v last=%v trend=%v, want 80/20/-60", r.FirstMargin, r.LastMargin, r.Trend)
	}
}

func TestComputeMarginTrendSortsWorstDeclineFirst(t *testing.T) {
	months := []string{"2026-03", "2026-04"}
	payable := map[string][]Charge{
		"2026-03": {{CustomerID: "up", SubTotal: 100}, {CustomerID: "down", SubTotal: 100}},
		"2026-04": {{CustomerID: "up", SubTotal: 100}, {CustomerID: "down", SubTotal: 200}},
	}
	receivable := map[string][]Charge{
		"2026-03": {{CustomerID: "up", SubTotal: 120}, {CustomerID: "down", SubTotal: 150}},
		"2026-04": {{CustomerID: "up", SubTotal: 160}, {CustomerID: "down", SubTotal: 150}},
	}
	rows := ComputeMarginTrend(months, payable, receivable)
	if len(rows) != 2 {
		t.Fatalf("want 2 rows, got %d", len(rows))
	}
	if rows[0].CustomerID != "down" {
		t.Errorf("worst decline should sort first, got %q", rows[0].CustomerID)
	}
}

func TestComputeMarginTrendEmptyMonths(t *testing.T) {
	rows := ComputeMarginTrend(nil, map[string][]Charge{}, map[string][]Charge{})
	if len(rows) != 0 {
		t.Fatalf("want no rows for no months, got %d", len(rows))
	}
}

func TestDiffSubscriptions(t *testing.T) {
	prior := []Subscription{
		{ID: "s1", CustomerID: "c1", CustomerName: "Acme", ProductName: "M365 E3", SKU: "E3", Quantity: 10},
		{ID: "s2", CustomerID: "c1", ProductName: "Defender", SKU: "DEF", Quantity: 5},
	}
	current := []Subscription{
		{ID: "s1", CustomerID: "c1", CustomerName: "Acme", ProductName: "M365 E3", SKU: "E3", Quantity: 25},
		{ID: "s3", CustomerID: "c2", CustomerName: "Beta", ProductName: "Exchange", SKU: "EXO", Quantity: 3},
	}
	changes := DiffSubscriptions(current, prior)
	if len(changes) != 3 {
		t.Fatalf("want 3 changes, got %d: %+v", len(changes), changes)
	}
	byChange := map[string]SubChange{}
	for _, c := range changes {
		byChange[c.Change] = c
	}
	if qc := byChange["quantity-changed"]; qc.SubscriptionID != "s1" || qc.QuantityDelta != 15 {
		t.Errorf("quantity-changed: %+v", qc)
	}
	if a := byChange["added"]; a.SubscriptionID != "s3" || a.CurrentQuantity != 3 {
		t.Errorf("added: %+v", a)
	}
	if r := byChange["removed"]; r.SubscriptionID != "s2" || r.PriorQuantity != 5 {
		t.Errorf("removed: %+v", r)
	}
	// Biggest absolute delta (s1, +15) sorts first.
	if changes[0].SubscriptionID != "s1" {
		t.Errorf("biggest delta should sort first, got %q", changes[0].SubscriptionID)
	}
}

func TestDiffSubscriptionsNoChanges(t *testing.T) {
	subs := []Subscription{{ID: "s1", CustomerID: "c1", Quantity: 10}}
	if changes := DiffSubscriptions(subs, subs); len(changes) != 0 {
		t.Fatalf("identical snapshots must produce no changes, got %+v", changes)
	}
}

func TestFindUsageLeaks(t *testing.T) {
	usage := []Usage{
		{CustomerID: "c1", SubscriptionID: "s1", SKU: "E3", Used: 12},  // billed → not a leak
		{CustomerID: "c1", SubscriptionID: "s2", SKU: "DEF", Used: 7},  // no receivable for sku → leak
		{CustomerID: "c2", SubscriptionID: "s3", SKU: "", Used: 4},     // sku via sub lookup → EXO unbilled → leak
		{CustomerID: "c3", SubscriptionID: "", SKU: "", Used: 9},       // no sku, customer unbilled → leak
		{CustomerID: "c1", SubscriptionID: "s4", SKU: "ZERO", Used: 0}, // zero usage → skip
	}
	receivable := []Charge{
		{CustomerID: "c1", SKU: "E3", SubTotal: 100},
	}
	subs := []Subscription{
		{ID: "s3", CustomerID: "c2", CustomerName: "Beta", ProductName: "Exchange", SKU: "EXO"},
	}
	leaks := FindUsageLeaks(usage, receivable, subs)
	if len(leaks) != 3 {
		t.Fatalf("want 3 leaks, got %d: %+v", len(leaks), leaks)
	}
	// Sorted by Used descending: c3 (9), c1/DEF (7), c2/EXO (4).
	if leaks[0].CustomerID != "c3" || leaks[0].Reason != "no-receivable-for-customer" {
		t.Errorf("leak[0]: %+v", leaks[0])
	}
	if leaks[1].SKU != "DEF" || leaks[1].Reason != "no-receivable-for-sku" {
		t.Errorf("leak[1]: %+v", leaks[1])
	}
	if leaks[2].SKU != "EXO" || leaks[2].ProductName != "Exchange" || leaks[2].CustomerName != "Beta" {
		t.Errorf("leak[2] should resolve SKU and names via subscription: %+v", leaks[2])
	}
}

func TestFindUsageLeaksUnresolvableSKUBilledCustomerSkipped(t *testing.T) {
	usage := []Usage{{CustomerID: "c1", SubscriptionID: "unknown", SKU: "", Used: 5}}
	receivable := []Charge{{CustomerID: "c1", SKU: "E3", SubTotal: 100}}
	if leaks := FindUsageLeaks(usage, receivable, nil); len(leaks) != 0 {
		t.Fatalf("unresolvable SKU on a billed customer is not provable, got %+v", leaks)
	}
}
