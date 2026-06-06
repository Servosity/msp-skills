// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"math"
	"testing"
)

func TestAggregate(t *testing.T) {
	nums := []float64{2, 8, 5}
	tests := []struct {
		agg  string
		want float64
	}{
		{"sum", 15},
		{"avg", 5},
		{"min", 2},
		{"max", 8},
	}
	for _, tt := range tests {
		if got := aggregate(tt.agg, nums); math.Abs(got-tt.want) > 1e-9 {
			t.Fatalf("aggregate(%s) = %v, want %v", tt.agg, got, tt.want)
		}
	}
	if got := aggregate("sum", nil); got != 0 {
		t.Fatalf("aggregate over empty slice = %v, want 0", got)
	}
}

func TestNumericValue(t *testing.T) {
	tests := []struct {
		name string
		row  string
		col  string
		want float64
		ok   bool
	}{
		{"json number", `{"Open Count": 42}`, "Open Count", 42, true},
		{"number-shaped string", `{"Open Count": "42.5"}`, "Open Count", 42.5, true},
		{"text value", `{"Open Count": "n/a"}`, "Open Count", 0, false},
		{"null", `{"Open Count": null}`, "Open Count", 0, false},
		{"missing column", `{"Other": 1}`, "Open Count", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := numericValue(json.RawMessage(tt.row), tt.col)
			if ok != tt.ok || math.Abs(got-tt.want) > 1e-9 {
				t.Fatalf("numericValue(%s, %q) = (%v,%v), want (%v,%v)", tt.row, tt.col, got, ok, tt.want, tt.ok)
			}
		})
	}
}
