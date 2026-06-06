// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Execution + scoring tests: multi-tenant hygiene rollup.

package cli

import (
	"encoding/json"
	"testing"
)

func TestNovelAuditSummaryRuns(t *testing.T) {
	out := runNovelCmd(t, newNovelAuditSummaryCmd)
	var rows []hygieneSummaryRow
	if err := json.Unmarshal(out, &rows); err != nil {
		t.Fatalf("output is not a JSON array: %v\n%s", err, out)
	}
	if len(rows) != 0 {
		t.Errorf("expected empty result on empty mirror, got %d rows", len(rows))
	}
}

func TestNovelAuditSummaryLimitRuns(t *testing.T) {
	out := runNovelCmd(t, newNovelAuditSummaryCmd, "--limit", "5", "--password-age", "90d", "--article-age", "180d", "--expire-within", "60d")
	var rows []hygieneSummaryRow
	if err := json.Unmarshal(out, &rows); err != nil {
		t.Fatalf("output is not a JSON array: %v\n%s", err, out)
	}
}

func TestHygieneScore(t *testing.T) {
	cases := []struct {
		name string
		subs []float64
		want float64
	}{
		{"no dimensions", nil, 0},
		{"single perfect", []float64{100}, 100},
		{"single zero", []float64{0}, 0},
		{"average of two", []float64{50, 100}, 75},
		{"average of four rounds to one decimal", []float64{100, 50, 25, 0}, 43.8},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := hygieneScore(tc.subs); got != tc.want {
				t.Errorf("hygieneScore(%v) = %v, want %v", tc.subs, got, tc.want)
			}
		})
	}
}

func TestRatioScore(t *testing.T) {
	cases := []struct {
		name string
		bad  int
		tot  int
		want float64
	}{
		{"no data is healthy", 0, 0, 100},
		{"all healthy", 0, 10, 100},
		{"all bad", 10, 10, 0},
		{"half bad", 5, 10, 50},
		{"one of four", 1, 4, 75},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ratioScore(tc.bad, tc.tot); got != tc.want {
				t.Errorf("ratioScore(%d, %d) = %v, want %v", tc.bad, tc.tot, got, tc.want)
			}
		})
	}
}
