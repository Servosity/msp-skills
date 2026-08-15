// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored: covers the four --since input families. See issue #219.

package cli

import (
	"testing"
	"time"
)

// TestParseSinceAcceptsRFC3339 is the regression. The flag help documents an
// RFC3339 timestamp, but parseSince folded its input to lowercase before
// parsing, and time.RFC3339's literal "T" and "Z" stopped matching.
func TestParseSinceAcceptsRFC3339(t *testing.T) {
	for _, in := range []string{
		"2026-08-01T00:00:00Z",
		"2026-08-01T12:34:56Z",
		"2026-08-01T12:34:56-04:00",
		"2026-08-01T12:34:56+05:30",
	} {
		got, err := parseSince(in)
		if err != nil {
			t.Errorf("parseSince(%q) = error %v, want a parsed time", in, err)
			continue
		}
		want, perr := time.Parse(time.RFC3339, in)
		if perr != nil {
			t.Fatalf("test input %q is not valid RFC3339: %v", in, perr)
		}
		if !got.Equal(want) {
			t.Errorf("parseSince(%q) = %s, want %s", in, got.Format(time.RFC3339), want.Format(time.RFC3339))
		}
	}
}

// TestParseSinceKeywordsStayCaseInsensitive guards the reason the fold existed:
// removing it outright would break these.
func TestParseSinceKeywordsStayCaseInsensitive(t *testing.T) {
	now := time.Now()
	for _, in := range []string{"today", "TODAY", "Today", " today "} {
		got, err := parseSince(in)
		if err != nil {
			t.Errorf("parseSince(%q) = error %v", in, err)
			continue
		}
		y, m, d := now.Date()
		want := time.Date(y, m, d, 0, 0, 0, 0, now.Location())
		if !got.Equal(want) {
			t.Errorf("parseSince(%q) = %s, want midnight today %s", in, got, want)
		}
	}
	for _, in := range []string{"yesterday", "YESTERDAY", "Yesterday"} {
		got, err := parseSince(in)
		if err != nil {
			t.Errorf("parseSince(%q) = error %v", in, err)
			continue
		}
		y, m, d := now.AddDate(0, 0, -1).Date()
		want := time.Date(y, m, d, 0, 0, 0, 0, now.Location())
		if !got.Equal(want) {
			t.Errorf("parseSince(%q) = %s, want midnight yesterday %s", in, got, want)
		}
	}
}

// TestParseSinceDurationsStayCaseInsensitive covers the other fold-dependent
// family: time.ParseDuration rejects uppercase unit letters on its own.
func TestParseSinceDurationsStayCaseInsensitive(t *testing.T) {
	cases := map[string]time.Duration{
		"24h": 24 * time.Hour,
		"24H": 24 * time.Hour,
		"90m": 90 * time.Minute,
		"90M": 90 * time.Minute,
	}
	for in, d := range cases {
		got, err := parseSince(in)
		if err != nil {
			t.Errorf("parseSince(%q) = error %v", in, err)
			continue
		}
		if delta := time.Since(got) - d; delta > 5*time.Second || delta < -5*time.Second {
			t.Errorf("parseSince(%q) is %v ago, want ~%v", in, time.Since(got), d)
		}
	}
	// Day suffix takes its own branch before ParseDuration.
	for _, in := range []string{"7d", "7D"} {
		got, err := parseSince(in)
		if err != nil {
			t.Errorf("parseSince(%q) = error %v", in, err)
			continue
		}
		// Compared as a calendar date, not elapsed hours: parseSince uses
		// AddDate, so a seven-day interval spanning a spring DST transition is
		// 167 hours, and an hours/24 assertion would report six days and fail
		// once a year in any DST-observing zone.
		want := time.Now().AddDate(0, 0, -7)
		if got.Year() != want.Year() || got.Month() != want.Month() || got.Day() != want.Day() {
			t.Errorf("parseSince(%q) = %s, want the calendar date %s",
				in, got.Format("2006-01-02"), want.Format("2006-01-02"))
		}
	}
}

// TestParseSinceAbsoluteFormats covers the remaining documented spellings,
// none of which contain letters and so are unaffected by the fold either way.
func TestParseSinceAbsoluteFormats(t *testing.T) {
	for _, in := range []string{
		"2026-08-01",
		"2026-08-01 09:30",
		"2026-08-01 09:30:15",
	} {
		if _, err := parseSince(in); err != nil {
			t.Errorf("parseSince(%q) = error %v, want a parsed time", in, err)
		}
	}
	// Bare clock time resolves to today at that time.
	got, err := parseSince("09:30")
	if err != nil {
		t.Fatalf("parseSince(\"09:30\") = error %v", err)
	}
	y, m, d := time.Now().Date()
	if got.Year() != y || got.Month() != m || got.Day() != d || got.Hour() != 9 || got.Minute() != 30 {
		t.Errorf("parseSince(\"09:30\") = %s, want today at 09:30", got)
	}
}

// TestParseSinceRejectsGarbage keeps the error path intact.
func TestParseSinceRejectsGarbage(t *testing.T) {
	for _, in := range []string{"not a time", "2026-13-45", "xyz"} {
		if _, err := parseSince(in); err == nil {
			t.Errorf("parseSince(%q) succeeded, want an error", in)
		}
	}
}
