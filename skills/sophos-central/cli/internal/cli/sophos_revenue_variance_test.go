// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/sophos-central/handfixes.json
// under the "revenue-variance-per-product" entry.

package cli

import "testing"

// TestRevenueVarianceNotSummedAcrossProducts: ten machines with ten
// protection and ten encryption seats is not a ten-seat discrepancy.
func TestRevenueVarianceNotSummedAcrossProducts(t *testing.T) {
	lines := []billingLine{
		{AccountID: "t1", ProductCode: "ixe", BillableQuantity: 10, UnitPrice: "1.00", UnitPriceCurrency: "USD"},
		{AccountID: "t1", ProductCode: "encryption", BillableQuantity: 10, UnitPrice: "1.00", UnitPriceCurrency: "USD"},
	}
	view := buildBilling("2026-09", lines, map[string]string{"t1": "Acme"}, map[string]int{"t1": 10})
	if len(view.Items) != 1 || view.Items[0].Variance == nil || *view.Items[0].Variance != 0 {
		t.Fatalf("customer variance = %v, want 0", view.Items[0].Variance)
	}
	for _, p := range view.Items[0].Products {
		if p.BillableMinusDeployed == nil || *p.BillableMinusDeployed != 0 {
			t.Fatalf("product %s variance = %v, want 0", p.Product, p.BillableMinusDeployed)
		}
	}
}
