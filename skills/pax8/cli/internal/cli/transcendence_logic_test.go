// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written (Phase 3): table-driven tests for the novel-feature pure helpers.

package cli

import (
	"encoding/json"
	"testing"
	"time"
)

func TestPax8Num(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want float64
		ok   bool
	}{
		{"float", float64(12.5), 12.5, true},
		{"json number", json.Number("9.99"), 9.99, true},
		{"numeric string", "42", 42, true},
		{"money string", "1234.56", 1234.56, true},
		{"empty string", "", 0, false},
		{"non-numeric string", "abc", 0, false},
		{"nil", nil, 0, false},
		{"bool", true, 0, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := pax8Num(c.in)
			if ok != c.ok {
				t.Fatalf("ok = %v, want %v", ok, c.ok)
			}
			if ok && got != c.want {
				t.Fatalf("got %v, want %v", got, c.want)
			}
		})
	}
}

func TestPax8FieldAndNum(t *testing.T) {
	obj := map[string]any{
		"price":       "19.99",
		"quantity":    float64(3),
		"companyName": "Acme MSP",
	}
	if got := pax8FieldStr(obj, "name", "companyName"); got != "Acme MSP" {
		t.Fatalf("fallback key lookup failed: got %q", got)
	}
	if v, ok := pax8FieldNum(obj, "price"); !ok || v != 19.99 {
		t.Fatalf("price num = %v ok=%v", v, ok)
	}
	if v, ok := pax8FieldNum(obj, "missing", "quantity"); !ok || v != 3 {
		t.Fatalf("quantity fallback num = %v ok=%v", v, ok)
	}
	if got := pax8FieldStr(obj, "nope"); got != "" {
		t.Fatalf("missing field should be empty, got %q", got)
	}
}

func TestMrrIsActive(t *testing.T) {
	active := []string{"", "Active", "active", "ENABLED", "trial"}
	inactive := []string{"Cancelled", "cancelled", "PendingCancel", "suspended", "deleted"}
	for _, s := range active {
		if !mrrIsActive(s) {
			t.Errorf("expected %q to be active", s)
		}
	}
	for _, s := range inactive {
		if mrrIsActive(s) {
			t.Errorf("expected %q to be inactive", s)
		}
	}
}

func TestParseSinceWindow(t *testing.T) {
	cases := []struct {
		in      string
		want    time.Duration
		wantErr bool
	}{
		{"7d", 7 * 24 * time.Hour, false},
		{"24h", 24 * time.Hour, false},
		{"2w", 14 * 24 * time.Hour, false},
		{"30", 30 * 24 * time.Hour, false},
		{"", 30 * 24 * time.Hour, false},
		{"bogus", 0, true},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			got, err := parseSinceWindow(c.in)
			if c.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q", c.in)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != c.want {
				t.Fatalf("got %v, want %v", got, c.want)
			}
		})
	}
}

func TestParseAnyDate(t *testing.T) {
	good := []string{
		"2026-05-28T12:00:00Z",
		"2026-05-28T12:00:00",
		"2026-05-28",
	}
	for _, s := range good {
		if _, ok := parseAnyDate(s); !ok {
			t.Errorf("expected %q to parse", s)
		}
	}
	if _, ok := parseAnyDate(""); ok {
		t.Error("empty string should not parse")
	}
	if _, ok := parseAnyDate("not-a-date"); ok {
		t.Error("garbage should not parse")
	}
}
