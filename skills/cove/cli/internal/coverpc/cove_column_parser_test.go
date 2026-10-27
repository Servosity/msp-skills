// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Codex review round 2, class "one exact column parser": a Cove column value
// is an integer only when its decimal or exponent form is exactly one. A
// fraction, NaN/Inf, overflow or garbage is not reported, never rounded and
// never read as zero.
package coverpc

import (
	"encoding/json"
	"testing"
)

// Round 2 P1: D9F00="4.9999999999999999" rounded to 5 (Completed) through a
// float64 conversion and `fleet health` counted the device healthy.
func TestFractionalStatusIsNotAnInteger(t *testing.T) {
	if n, ok := SettingInt(map[string]string{"D9F00": "4.9999999999999999"}, "D9F00"); ok {
		t.Fatalf("fractional status parsed as %d", n)
	}
	if class := ClassifyStatus(SettingInt(map[string]string{"D9F00": "4.9999999999999999"}, "D9F00")); class != StatusUnknown {
		t.Fatalf("fractional status classified %q, want unknown", class)
	}
}

func TestColumnIntegerParsingIsExact(t *testing.T) {
	cases := []struct {
		raw  string
		want int64
		ok   bool
	}{
		{"5", 5, true},
		{" 5 ", 5, true},
		{"+5", 5, true},
		{"-3", -3, true},
		{"5.0", 5, true},
		{"5.", 5, true},
		{"2e9", 2000000000, true},
		{"2E+9", 2000000000, true},
		{"1.792e9", 1792000000, true},
		{"1792800000.000", 1792800000, true},
		{"1000e-3", 1, true},
		{"0.0e5", 0, true},
		{"-0", 0, true},
		{"9223372036854775807", 9223372036854775807, true},
		{"-9223372036854775808", -9223372036854775808, true},
		{"9.223372036854775807e18", 9223372036854775807, true},
		{"123456789012345678", 123456789012345678, true},
		{"1.23456789012345678e17", 123456789012345678, true},
		// Not an exact integer: never rounded, never zero.
		{"4.9999999999999999", 0, false},
		{"5.0000000000000001", 0, false},
		{"1792800000.5", 0, false},
		{"1.7928000005e9", 0, false},
		{"1e-3", 0, false},
		{".5", 0, false},
		{"9223372036854775808", 0, false},
		{"-9223372036854775809", 0, false},
		{"9.3e18", 0, false},
		{"1e19", 0, false},
		{"1e99999999999999999999", 0, false},
		{"NaN", 0, false},
		{"Inf", 0, false},
		{"-Infinity", 0, false},
		{"0x10", 0, false},
		{"1_000", 0, false},
		{"0b1", 0, false},
		{"10/2", 0, false},
		{"", 0, false},
		{".", 0, false},
		{"e5", 0, false},
		{"5 6", 0, false},
		{"five", 0, false},
	}
	for _, c := range cases {
		got, ok := SettingInt(map[string]string{"X": c.raw}, "X")
		if ok != c.ok || got != c.want {
			t.Errorf("SettingInt(%q) = %d, %v; want %d, %v", c.raw, got, ok, c.want, c.ok)
		}
	}
}

// A JSON number keeps its exact decimal form through the shared parser, so a
// fractional status, an id past 2^53 and a large byte count all survive.
func TestColumnTextKeepsJSONNumbersExact(t *testing.T) {
	raw := json.RawMessage(`{"result":[{"AccountId":9007199254740993,"PartnerId":1.5,"Settings":[{"D9F00":4.9999999999999999},{"I14":123456789012345678},{"D9F09":1.7928e9}]}]}`)
	rows, err := Rows(raw)
	if err != nil || len(rows) != 1 {
		t.Fatalf("Rows: %v", err)
	}
	if id, ok := RowInt(rows[0], "AccountId"); !ok || id != 9007199254740993 {
		t.Fatalf("AccountId = %d, %v", id, ok)
	}
	if id, ok := RowInt(rows[0], "PartnerId"); ok {
		t.Fatalf("fractional PartnerId read as %d", id)
	}
	s := FlattenSettings(rows[0]["Settings"])
	if n, ok := SettingInt(s, "D9F00"); ok {
		t.Fatalf("fractional JSON-number status read as %d (text %q)", n, s["D9F00"])
	}
	if n, ok := SettingInt(s, "I14"); !ok || n != 123456789012345678 {
		t.Fatalf("I14 = %d, %v (text %q)", n, ok, s["I14"])
	}
	if n, ok := SettingInt(s, "D9F09"); !ok || n != 1792800000 {
		t.Fatalf("D9F09 = %d, %v (text %q)", n, ok, s["D9F09"])
	}
	// A float64 past 2^53 is no longer exact: it is not reported.
	if text, ok := ColumnText(float64(9007199254740993)); ok {
		t.Fatalf("inexact float64 rendered as %q", text)
	}
	if text, ok := ColumnText(float64(1792800000)); !ok || text != "1792800000" {
		t.Fatalf("exact float64 = %q, %v", text, ok)
	}
}

// Counts and byte sizes are never negative: a negative value is not
// reported, so it can never invent a shrink or wrap a total.
func TestSettingCountRejectsNegatives(t *testing.T) {
	if n, ok := SettingCount(map[string]string{"I14": "-1"}, "I14"); ok {
		t.Fatalf("negative byte count read as %d", n)
	}
	if n, ok := SettingCount(map[string]string{"I14": "0"}, "I14"); !ok || n != 0 {
		t.Fatalf("reported zero = %d, %v", n, ok)
	}
	if n, ok := SettingCount(map[string]string{"I14": "2.5e3"}, "I14"); !ok || n != 2500 {
		t.Fatalf("2.5e3 = %d, %v", n, ok)
	}
	if _, ok := SettingCount(map[string]string{}, "I14"); ok {
		t.Fatal("absent column reported")
	}
}
