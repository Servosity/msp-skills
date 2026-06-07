// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package insights

import "testing"

func TestComputeMargin(t *testing.T) {
	payable := []Charge{
		{CustomerID: "c1", CustomerName: "Acme", SubTotal: 100},
		{CustomerID: "c1", CustomerName: "Acme", SubTotal: 50},
		{CustomerID: "c2", CustomerName: "Globex", SubTotal: 200},
	}
	receivable := []Charge{
		{CustomerID: "c1", CustomerName: "Acme", SubTotal: 250},
		{CustomerID: "c2", CustomerName: "Globex", SubTotal: 180}, // negative margin
	}
	got := ComputeMargin(payable, receivable)
	if len(got) != 2 {
		t.Fatalf("want 2 customers, got %d", len(got))
	}
	// Sorted by margin ascending: Globex (180-200=-20) before Acme (250-150=100).
	if got[0].CustomerID != "c2" {
		t.Fatalf("want worst-margin customer first (c2), got %s", got[0].CustomerID)
	}
	if got[0].Margin != -20 {
		t.Errorf("c2 margin: want -20, got %v", got[0].Margin)
	}
	if got[1].CustomerID != "c1" || got[1].Margin != 100 {
		t.Errorf("c1: want margin 100, got id=%s margin=%v", got[1].CustomerID, got[1].Margin)
	}
	if got[1].Payable != 150 || got[1].Receivable != 250 {
		t.Errorf("c1 payable/receivable: want 150/250, got %v/%v", got[1].Payable, got[1].Receivable)
	}
	if got[1].MarginPct != 40 { // 100/250*100
		t.Errorf("c1 marginPct: want 40, got %v", got[1].MarginPct)
	}
}

func TestComputeMargin_OneSidedAndEmpty(t *testing.T) {
	if got := ComputeMargin(nil, nil); len(got) != 0 {
		t.Fatalf("empty inputs must yield empty result, got %d", len(got))
	}
	// Payable only (customer owes nothing) → negative margin, no panic on /0.
	got := ComputeMargin([]Charge{{CustomerID: "c1", SubTotal: 30}}, nil)
	if len(got) != 1 || got[0].Margin != -30 || got[0].MarginPct != 0 {
		t.Fatalf("payable-only: want margin -30 pct 0, got %+v", got)
	}
}

func TestComputeDrift(t *testing.T) {
	prior := []Charge{
		{CustomerID: "c1", SKU: "M365", ChargeName: "license", SubTotal: 100},
		{CustomerID: "c1", SKU: "OLD", ChargeName: "legacy", SubTotal: 40}, // vanishes
	}
	current := []Charge{
		{CustomerID: "c1", SKU: "M365", ChargeName: "license", SubTotal: 130}, // changed +30
		{CustomerID: "c2", SKU: "NEW", ChargeName: "addon", SubTotal: 75},     // appeared
	}
	got := ComputeDrift(current, prior)
	if len(got) != 3 {
		t.Fatalf("want 3 drift entries (changed, vanished, appeared), got %d: %+v", len(got), got)
	}
	// Sorted by abs(delta) desc: M365 +30, OLD -40 (abs 40) actually largest → order: OLD(-40), M365(+30), NEW... NEW current 75 prior 0 delta +75 largest.
	byChange := map[string]DriftEntry{}
	for _, e := range got {
		byChange[e.Change] = e
	}
	if byChange["changed"].Delta != 30 {
		t.Errorf("changed delta: want 30, got %v", byChange["changed"].Delta)
	}
	if byChange["vanished"].CurrentSubTotal != 0 || byChange["vanished"].PriorSubTotal != 40 {
		t.Errorf("vanished: want prior 40 current 0, got %+v", byChange["vanished"])
	}
	if byChange["appeared"].PriorSubTotal != 0 || byChange["appeared"].CurrentSubTotal != 75 {
		t.Errorf("appeared: want prior 0 current 75, got %+v", byChange["appeared"])
	}
}

func TestComputeDrift_NoChangeYieldsEmpty(t *testing.T) {
	same := []Charge{{CustomerID: "c1", SKU: "M365", ChargeName: "license", SubTotal: 100}}
	if got := ComputeDrift(same, same); len(got) != 0 {
		t.Fatalf("identical snapshots must yield no drift, got %d: %+v", len(got), got)
	}
	if got := ComputeDrift(nil, nil); len(got) != 0 {
		t.Fatalf("empty snapshots must yield no drift, got %d", len(got))
	}
}

func TestFindOrphans(t *testing.T) {
	subs := []Subscription{
		{ID: "s1", CustomerID: "c1", ProductName: "M365", Status: "active"},   // c1 billed → not orphan
		{ID: "s2", CustomerID: "c2", ProductName: "EDR", Status: "active"},    // c2 not billed → orphan
		{ID: "s3", CustomerID: "c3", ProductName: "Bkp", Status: "suspended"}, // inactive → skip
	}
	receivable := []Charge{{CustomerID: "c1", SubTotal: 50}}
	got := FindOrphans(subs, receivable)
	if len(got) != 1 {
		t.Fatalf("want 1 orphan, got %d: %+v", len(got), got)
	}
	if got[0].SubscriptionID != "s2" {
		t.Errorf("want orphan s2, got %s", got[0].SubscriptionID)
	}
}

func TestFindOrphans_NoneWhenAllBilled(t *testing.T) {
	subs := []Subscription{{ID: "s1", CustomerID: "c1", Status: "active"}}
	receivable := []Charge{{CustomerID: "c1", SubTotal: 10}}
	if got := FindOrphans(subs, receivable); len(got) != 0 {
		t.Fatalf("all-billed must yield no orphans, got %d", len(got))
	}
	if got := FindOrphans(nil, nil); len(got) != 0 {
		t.Fatalf("empty must yield no orphans, got %d", len(got))
	}
}

func TestRollupFleet(t *testing.T) {
	subs := []Subscription{
		{CustomerID: "c1", ProductName: "Microsoft 365", SKU: "M365-E3", Quantity: 10},
		{CustomerID: "c2", ProductName: "Microsoft 365", SKU: "M365-E3", Quantity: 5},
		{CustomerID: "c1", ProductName: "Backup", SKU: "BKP-1", Quantity: 3},
	}
	got := RollupFleet(subs, "", "")
	if len(got) != 2 {
		t.Fatalf("want 2 fleet rows, got %d", len(got))
	}
	// Sorted by total seats desc: M365 (15) first.
	if got[0].SKU != "M365-E3" || got[0].TotalSeats != 15 || got[0].CustomerCount != 2 || got[0].SubscriptionCount != 2 {
		t.Errorf("M365 rollup wrong: %+v", got[0])
	}
	// product filter
	f := RollupFleet(subs, "backup", "")
	if len(f) != 1 || f[0].SKU != "BKP-1" {
		t.Errorf("product filter failed: %+v", f)
	}
	// non-matching filter → empty
	if n := RollupFleet(subs, "nonexistent", ""); len(n) != 0 {
		t.Errorf("non-matching filter must be empty, got %d", len(n))
	}
}

func TestRightSize(t *testing.T) {
	subs := []Subscription{
		{ID: "s1", CustomerID: "c1", ProductName: "M365", SKU: "E3", Quantity: 10},
		{ID: "s2", CustomerID: "c2", ProductName: "EDR", SKU: "EDR1", Quantity: 5},
		{ID: "s3", CustomerID: "c3", ProductName: "NoUsage", SKU: "NU", Quantity: 9}, // no usage → skipped
	}
	usage := []Usage{
		{SubscriptionID: "s1", Used: 4}, // over-provisioned (10-4=6)
		{SKU: "EDR1", Used: 8},          // under-provisioned (5-8=-3), matched by SKU
	}
	got := RightSize(subs, usage)
	if len(got) != 2 {
		t.Fatalf("want 2 rows (s3 has no usage), got %d: %+v", len(got), got)
	}
	// Sorted by abs(delta) desc: s1 delta 6 first.
	if got[0].SubscriptionID != "s1" || got[0].Delta != 6 || got[0].Recommendation != "over-provisioned" {
		t.Errorf("s1 wrong: %+v", got[0])
	}
	if got[1].SubscriptionID != "s2" || got[1].Delta != -3 || got[1].Recommendation != "under-provisioned" {
		t.Errorf("s2 wrong: %+v", got[1])
	}
}

func TestRightSize_EmptyUsage(t *testing.T) {
	subs := []Subscription{{ID: "s1", Quantity: 10}}
	if got := RightSize(subs, nil); len(got) != 0 {
		t.Fatalf("no usage data must yield no rows, got %d", len(got))
	}
}

func TestPreviewAmendment(t *testing.T) {
	sub := Subscription{ID: "s1", ProductName: "M365", Quantity: 10, UnitPrice: 12.50, Currency: "USD"}
	got := PreviewAmendment(sub, 25)
	if got.DeltaQuantity != 15 {
		t.Errorf("deltaQuantity: want 15, got %v", got.DeltaQuantity)
	}
	if got.MonthlyDelta != 187.5 { // 15 * 12.5
		t.Errorf("monthlyDelta: want 187.5, got %v", got.MonthlyDelta)
	}
	if got.CurrentMonthly != 125 || got.NewMonthly != 312.5 {
		t.Errorf("monthly totals wrong: current=%v new=%v", got.CurrentMonthly, got.NewMonthly)
	}
}

func TestPreviewAmendment_Downsize(t *testing.T) {
	sub := Subscription{ID: "s1", Quantity: 10, UnitPrice: 10}
	got := PreviewAmendment(sub, 4)
	if got.DeltaQuantity != -6 || got.MonthlyDelta != -60 {
		t.Errorf("downsize: want delta -6 monthlyDelta -60, got %v/%v", got.DeltaQuantity, got.MonthlyDelta)
	}
}
