// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package pipeintel

import (
	"testing"
	"time"
)

func TestParseSince(t *testing.T) {
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		in   string
		want time.Time
		ok   bool
	}{
		{"24h", now.Add(-24 * time.Hour), true},
		{"30m", now.Add(-30 * time.Minute), true},
		{"7d", now.AddDate(0, 0, -7), true},
		{"2w", now.AddDate(0, 0, -14), true},
		{"2026-05-01", time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC), true},
		{"2026-05-01 14:30:00", time.Date(2026, 5, 1, 14, 30, 0, 0, time.UTC), true},
		{"", time.Time{}, false},
		{"banana", time.Time{}, false},
	}
	for _, c := range cases {
		got, err := ParseSince(c.in, now)
		if c.ok && err != nil {
			t.Errorf("ParseSince(%q) unexpected error: %v", c.in, err)
			continue
		}
		if !c.ok && err == nil {
			t.Errorf("ParseSince(%q) expected error, got %v", c.in, got)
			continue
		}
		if c.ok && !got.Equal(c.want) {
			t.Errorf("ParseSince(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestPeriodRange(t *testing.T) {
	now := time.Date(2026, 5, 29, 9, 0, 0, 0, time.UTC) // a Friday in Q2
	cases := []struct {
		period     string
		start, end string
		ok         bool
	}{
		{"this-month", "2026-05-01", "2026-06-01", true},
		{"this-quarter", "2026-04-01", "2026-07-01", true},
		{"next-quarter", "2026-07-01", "2026-10-01", true},
		{"this-year", "2026-01-01", "2027-01-01", true},
		{"today", "2026-05-29", "2026-05-30", true},
		{"never", "", "", false},
	}
	for _, c := range cases {
		s, e, ok := PeriodRange(c.period, now)
		if ok != c.ok {
			t.Errorf("PeriodRange(%q) ok = %v, want %v", c.period, ok, c.ok)
			continue
		}
		if !ok {
			continue
		}
		if s.Format("2006-01-02") != c.start || e.Format("2006-01-02") != c.end {
			t.Errorf("PeriodRange(%q) = [%s,%s), want [%s,%s)", c.period,
				s.Format("2006-01-02"), e.Format("2006-01-02"), c.start, c.end)
		}
	}
}

func TestMedian(t *testing.T) {
	cases := []struct {
		in   []float64
		want float64
	}{
		{nil, 0},
		{[]float64{5}, 5},
		{[]float64{1, 2, 3}, 2},
		{[]float64{1, 2, 3, 4}, 2.5},
		{[]float64{10, 1, 5}, 5}, // unsorted input
	}
	for _, c := range cases {
		if got := Median(c.in); got != c.want {
			t.Errorf("Median(%v) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestNormalizePhone(t *testing.T) {
	cases := []struct{ in, want string }{
		{"+1 (415) 555-0100", "4155550100"},
		{"415-555-0100", "4155550100"},
		{"4155550100", "4155550100"},
		{"555-0100", "5550100"},
		{"123", ""},                         // too short
		{"", ""},                            // empty
		{"00 1 415 555 0100", "4155550100"}, // long with country/exit code -> last 10
	}
	for _, c := range cases {
		if got := NormalizePhone(c.in); got != c.want {
			t.Errorf("NormalizePhone(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestNormalizeName(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Acme, Inc.", "acme"},
		{"acme inc", "acme"},
		{"  ACME   Corporation ", "acme"},
		{"Globex LLC", "globex"},
		{"Initech", "initech"},
		{"A & B Co", "a b"},
		{"", ""},
	}
	for _, c := range cases {
		if got := NormalizeName(c.in); got != c.want {
			t.Errorf("NormalizeName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
