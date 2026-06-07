// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

// Package pipeintel holds the pure, table-testable logic behind the Pipedrive
// CLI's pipeline-intelligence commands (stale, forecast, aging, digest,
// changes, dupes, leaderboard): time-window parsing, period date ranges,
// median computation, and contact-name normalization. Keeping this logic out
// of the Cobra command files makes it unit-testable without a database or a
// live API.
package pipeintel

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

var sinceDurationRe = regexp.MustCompile(`^(\d+)\s*([mhdw])$`)

// ParseSince converts a "--since" value into an absolute cutoff timestamp,
// relative to now. It accepts:
//   - relative durations: "30m", "24h", "7d", "2w" (minutes/hours/days/weeks)
//   - calendar dates: "2026-05-01"
//   - datetimes: "2026-05-01 14:30:00" or RFC3339 "2026-05-01T14:30:00Z"
//
// The returned time is the lower bound; callers select rows whose timestamp is
// >= the returned cutoff.
func ParseSince(s string, now time.Time) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, fmt.Errorf("empty --since value")
	}
	if m := sinceDurationRe.FindStringSubmatch(strings.ToLower(s)); m != nil {
		var n int
		_, _ = fmt.Sscanf(m[1], "%d", &n)
		switch m[2] {
		case "m":
			return now.Add(-time.Duration(n) * time.Minute), nil
		case "h":
			return now.Add(-time.Duration(n) * time.Hour), nil
		case "d":
			return now.AddDate(0, 0, -n), nil
		case "w":
			return now.AddDate(0, 0, -7*n), nil
		}
	}
	// Absolute forms.
	for _, layout := range []string{
		time.RFC3339,
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
		"2006-01-02",
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unrecognized --since %q: use a duration (24h, 7d, 2w) or a date (2026-05-01)", s)
}

// PeriodRange returns the [start, end) bounds for a named forecast period,
// relative to now. Recognized periods: today, this-week, this-month,
// this-quarter, this-year, next-month, next-quarter. ok is false for an
// unrecognized period.
func PeriodRange(period string, now time.Time) (start, end time.Time, ok bool) {
	y, mo, _ := now.Date()
	loc := now.Location()
	startOfDay := time.Date(y, mo, now.Day(), 0, 0, 0, 0, loc)
	startOfMonth := time.Date(y, mo, 1, 0, 0, 0, 0, loc)
	switch strings.ToLower(strings.TrimSpace(period)) {
	case "today":
		return startOfDay, startOfDay.AddDate(0, 0, 1), true
	case "this-week":
		// ISO week: start on Monday.
		offset := (int(now.Weekday()) + 6) % 7
		ws := startOfDay.AddDate(0, 0, -offset)
		return ws, ws.AddDate(0, 0, 7), true
	case "this-month":
		return startOfMonth, startOfMonth.AddDate(0, 1, 0), true
	case "next-month":
		ns := startOfMonth.AddDate(0, 1, 0)
		return ns, ns.AddDate(0, 1, 0), true
	case "this-quarter":
		qStartMonth := time.Month((int(mo)-1)/3*3 + 1)
		qs := time.Date(y, qStartMonth, 1, 0, 0, 0, 0, loc)
		return qs, qs.AddDate(0, 3, 0), true
	case "next-quarter":
		qStartMonth := time.Month((int(mo)-1)/3*3 + 1)
		qs := time.Date(y, qStartMonth, 1, 0, 0, 0, 0, loc).AddDate(0, 3, 0)
		return qs, qs.AddDate(0, 3, 0), true
	case "this-year":
		ys := time.Date(y, 1, 1, 0, 0, 0, 0, loc)
		return ys, ys.AddDate(1, 0, 0), true
	}
	return time.Time{}, time.Time{}, false
}

// Median returns the median of xs. For an even count it averages the two
// middle values. Returns 0 for an empty slice. The input is not mutated.
func Median(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	c := make([]float64, len(xs))
	copy(c, xs)
	sort.Float64s(c)
	n := len(c)
	if n%2 == 1 {
		return c[n/2]
	}
	return (c[n/2-1] + c[n/2]) / 2
}

var nonDigit = regexp.MustCompile(`\D+`)

// NormalizePhone reduces a phone number to a comparable key: digits only, with
// the trailing 10 digits kept so a leading country code (+1) doesn't defeat the
// match. Returns "" for fewer than 7 digits — too short to be a reliable
// duplicate signal.
func NormalizePhone(s string) string {
	digits := nonDigit.ReplaceAllString(s, "")
	if len(digits) < 7 {
		return ""
	}
	if len(digits) > 10 {
		digits = digits[len(digits)-10:]
	}
	return digits
}

var nonAlnum = regexp.MustCompile(`[^a-z0-9]+`)

// NormalizeName lowercases, strips punctuation, drops common company suffixes,
// and collapses whitespace so that "Acme, Inc." and "acme inc" collapse to the
// same key for duplicate detection. Returns "" for input that normalizes to
// nothing.
func NormalizeName(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = nonAlnum.ReplaceAllString(s, " ")
	fields := strings.Fields(s)
	// Drop trailing legal/company-suffix tokens that don't disambiguate.
	suffixes := map[string]bool{
		"inc": true, "incorporated": true, "llc": true, "ltd": true,
		"limited": true, "corp": true, "corporation": true, "co": true,
		"gmbh": true, "plc": true, "sa": true, "ag": true, "bv": true,
	}
	for len(fields) > 1 && suffixes[fields[len(fields)-1]] {
		fields = fields[:len(fields)-1]
	}
	return strings.Join(fields, " ")
}
