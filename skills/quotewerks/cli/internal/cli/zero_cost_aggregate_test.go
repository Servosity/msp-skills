// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored: a zero cost on a priced line is unknown, never 100% margin.

package cli

import "testing"

func TestZeroCostLineIsUnknownInTotals(t *testing.T) {
	lines := []map[string]any{
		{"LineType": 1.0, "QtyBase": 1.0, "UnitPrice": 100.0, "UnitCost": 80.0, "ExtendedPrice": 100.0, "ExtendedCost": 80.0},
		{"LineType": 1.0, "QtyBase": 1.0, "UnitPrice": 100.0, "UnitCost": 0.0, "ExtendedPrice": 100.0, "ExtendedCost": 0.0},
	}
	got := sumLines(lines)
	if got.UnknownCostLines != 1 || got.MarginPct != nil {
		t.Fatalf("zero-cost line counted as known: unknown=%d margin=%v", got.UnknownCostLines, got.MarginPct)
	}
	if costMissing(0, 0) {
		t.Fatal("a $0 line with $0 cost is not a missing cost")
	}
}

func TestCostDriftZeroQuotedCostHasNoImpact(t *testing.T) {
	h := newQWHarness(t)
	h.fake.mu.Lock()
	h.fake.tables["DocumentItems"] = append(h.fake.tables["DocumentItems"], map[string]any{
		"RecGUID": guid(106), "ID": 106.0, "DocRecGUID": guid(1), "LineType": 1.0, "LineNumberActual": 5.0,
		"Manufacturer": "Acme", "ManufacturerPartNumber": "SVC-1", "Description": "Setup (no cost)",
		"QtyBase": 2.0, "UnitPrice": 300.0, "UnitCost": 0.0, "ExtendedPrice": 600.0, "ExtendedCost": 0.0,
	})
	h.fake.mu.Unlock()
	var mirrored map[string]any
	if code := h.runJSON(&mirrored, "mirror", "--db", h.db); code != 0 {
		t.Fatalf("mirror exit %d: %v", code, mirrored)
	}
	var drift struct {
		Rows []driftRow `json:"rows"`
	}
	h.runJSON(&drift, "cost-drift", "--db", h.db)
	for _, r := range drift.Rows {
		if r.ItemID == guid(106) {
			if r.ExtendedImpact != nil || r.DeltaPct != nil || r.MarginQuoted != nil {
				t.Fatalf("zero quoted cost reported a definite change: %+v", r)
			}
			return
		}
	}
	t.Fatalf("zero-cost line with a catalog cost did not drift: %+v", drift.Rows)
}
