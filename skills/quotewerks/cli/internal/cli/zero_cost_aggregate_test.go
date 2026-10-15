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
