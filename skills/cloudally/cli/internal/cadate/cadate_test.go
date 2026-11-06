// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cadate

import (
	"testing"
	"time"
)

func utc(y int, m time.Month, d, hh, mm, ss int) time.Time {
	return time.Date(y, m, d, hh, mm, ss, 0, time.UTC)
}

func TestParseCompact(t *testing.T) {
	cases := []struct {
		name    string
		in      int64
		want    time.Time
		wantErr bool
	}{
		{"valid", 20261001153000, utc(2026, 10, 1, 15, 30, 0), false},
		{"midnight", 20260101000000, utc(2026, 1, 1, 0, 0, 0), false},
		{"zero", 0, time.Time{}, true},
		{"too short", 2026100115, time.Time{}, true},
		{"bad month", 20261301000000, time.Time{}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseCompact(tc.in)
			if (err != nil) != tc.wantErr {
				t.Fatalf("ParseCompact(%d) err = %v, wantErr %v", tc.in, err, tc.wantErr)
			}
			if !tc.wantErr && !got.Equal(tc.want) {
				t.Fatalf("ParseCompact(%d) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestFormatCompactRoundTrip(t *testing.T) {
	cases := []time.Time{
		utc(2026, 10, 1, 15, 30, 0),
		utc(1999, 12, 31, 23, 59, 59),
		time.Date(2026, 10, 1, 12, 0, 0, 0, time.FixedZone("plus2", 2*3600)),
	}
	for _, in := range cases {
		raw := FormatCompact(in)
		back, err := ParseCompact(raw)
		if err != nil {
			t.Fatalf("round trip %v: %v", in, err)
		}
		if !back.Equal(in) {
			t.Fatalf("round trip %v: got %v via %d", in, back, raw)
		}
	}
	if got := FormatCompact(utc(2026, 10, 1, 15, 30, 0)); got != 20261001153000 {
		t.Fatalf("FormatCompact = %d", got)
	}
}

func TestParseUserDate(t *testing.T) {
	cases := []struct {
		name     string
		in       string
		want     time.Time
		dateOnly bool
		wantErr  bool
	}{
		{"date only is end of day", "2026-10-01", utc(2026, 10, 1, 23, 59, 59), true, false},
		{"rfc3339 utc", "2026-10-01T08:15:00Z", utc(2026, 10, 1, 8, 15, 0), false, false},
		{"rfc3339 offset", "2026-10-01T10:15:00+02:00", utc(2026, 10, 1, 8, 15, 0), false, false},
		{"short clock", "2026-10-01T08:15", utc(2026, 10, 1, 8, 15, 0), false, false},
		{"space clock", "2026-10-01 08:15", utc(2026, 10, 1, 8, 15, 0), false, false},
		{"compact digits", "20261001081500", utc(2026, 10, 1, 8, 15, 0), false, false},
		{"eight digits", "20261001", utc(2026, 10, 1, 23, 59, 59), true, false},
		{"whitespace trimmed", "  2026-10-01 ", utc(2026, 10, 1, 23, 59, 59), true, false},
		{"empty", "", time.Time{}, false, true},
		{"garbage", "yesterday", time.Time{}, false, true},
		{"wrong digit count", "202610011", time.Time{}, false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, dateOnly, err := ParseUserDate(tc.in)
			if (err != nil) != tc.wantErr {
				t.Fatalf("ParseUserDate(%q) err = %v, wantErr %v", tc.in, err, tc.wantErr)
			}
			if tc.wantErr {
				return
			}
			if !got.Equal(tc.want) || dateOnly != tc.dateOnly {
				t.Fatalf("ParseUserDate(%q) = %v,%v want %v,%v", tc.in, got, dateOnly, tc.want, tc.dateOnly)
			}
		})
	}
}

func TestParseLoose(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		want    time.Time
		wantErr bool
	}{
		{"compact digits", "20261001081500", utc(2026, 10, 1, 8, 15, 0), false},
		{"rfc3339", "2026-10-01T08:15:00Z", utc(2026, 10, 1, 8, 15, 0), false},
		{"rfc3339 nano", "2026-10-01T08:15:00.123Z", utc(2026, 10, 1, 8, 15, 0).Add(123 * time.Millisecond), false},
		{"space separated", "2026-10-01 08:15:00", utc(2026, 10, 1, 8, 15, 0), false},
		{"date only", "2026-10-01", utc(2026, 10, 1, 0, 0, 0), false},
		{"unix millis", "1790000000000", time.UnixMilli(1790000000000).UTC(), false},
		{"unix seconds", "1790000000", time.Unix(1790000000, 0).UTC(), false},
		{"empty", "", time.Time{}, true},
		{"word", "never", time.Time{}, true},
		{"odd digits", "123", time.Time{}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseLoose(tc.in)
			if (err != nil) != tc.wantErr {
				t.Fatalf("ParseLoose(%q) err = %v, wantErr %v", tc.in, err, tc.wantErr)
			}
			if !tc.wantErr && !got.Equal(tc.want) {
				t.Fatalf("ParseLoose(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestNewestAtOrBefore(t *testing.T) {
	dates := map[string]int64{
		"MAIL":     20261001060000,
		"CALENDAR": 20261001120000,
		"CONTACTS": 20260930180000,
		"BROKEN":   42,
	}
	cases := []struct {
		name      string
		cutoff    time.Time
		subSource string
		wantKey   string
		wantRaw   int64
		wantOK    bool
	}{
		{"newest across all", utc(2026, 10, 1, 23, 59, 59), "", "CALENDAR", 20261001120000, true},
		{"cutoff excludes newest", utc(2026, 10, 1, 8, 0, 0), "", "MAIL", 20261001060000, true},
		{"cutoff equal is inclusive", utc(2026, 10, 1, 6, 0, 0), "", "MAIL", 20261001060000, true},
		{"sub-source case-insensitive", utc(2026, 10, 1, 23, 59, 59), "mail", "MAIL", 20261001060000, true},
		{"sub-source too new", utc(2026, 9, 30, 12, 0, 0), "mail", "", 0, false},
		{"unknown sub-source", utc(2026, 10, 1, 23, 59, 59), "sites", "", 0, false},
		{"nothing before cutoff", utc(2026, 9, 1, 0, 0, 0), "", "", 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			key, raw, ts, ok := NewestAtOrBefore(dates, tc.cutoff, tc.subSource)
			if ok != tc.wantOK || key != tc.wantKey || raw != tc.wantRaw {
				t.Fatalf("NewestAtOrBefore = %q,%d,%v,%v want %q,%d,%v", key, raw, ts, ok, tc.wantKey, tc.wantRaw, tc.wantOK)
			}
			if ok {
				want, _ := ParseCompact(raw)
				if !ts.Equal(want) {
					t.Fatalf("time %v does not match raw %d", ts, raw)
				}
			}
		})
	}
	t.Run("tie picks lexically smallest key", func(t *testing.T) {
		tied := map[string]int64{"ZETA": 20261001060000, "ALPHA": 20261001060000}
		key, _, _, ok := NewestAtOrBefore(tied, utc(2026, 10, 2, 0, 0, 0), "")
		if !ok || key != "ALPHA" {
			t.Fatalf("tie = %q,%v", key, ok)
		}
	})
	t.Run("nil map", func(t *testing.T) {
		if _, _, _, ok := NewestAtOrBefore(nil, utc(2026, 10, 2, 0, 0, 0), ""); ok {
			t.Fatal("expected no result for nil map")
		}
	})
}
