// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Package cadate parses and formats the date shapes the CloudAlly V2 API
// uses: compact yyyyMMddHHmmss integers on task and entity records, loose
// strings on the partner-wide status feed, and the user-facing --date
// grammar accepted by the restore commands. All times are UTC.
package cadate

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// CompactLayout is the yyyyMMddHHmmss layout the API encodes as an int64.
// The restore body's date field uses the same encoding. Keep this the single
// source of truth for the format.
const CompactLayout = "20060102150405"

// compactDigits is the number of digits in a compact timestamp.
const compactDigits = 14

// dateOnlyDigits is the number of digits in a bare yyyyMMdd value.
const dateOnlyDigits = 8

// unixMillisDigits and unixSecondsDigits identify epoch encodings that some
// feeds use for the same field.
const (
	unixMillisDigits  = 13
	unixSecondsDigits = 10
)

var (
	// ErrEmpty is returned for an empty input string.
	ErrEmpty = errors.New("empty date")
	// ErrFormat is returned when no supported layout matches.
	ErrFormat = errors.New("unrecognized date format")
)

// userLayouts are the clock-bearing layouts accepted by ParseUserDate, tried
// in order after the compact and date-only forms.
var userLayouts = []string{
	time.RFC3339,
	"2006-01-02T15:04:05",
	"2006-01-02T15:04",
	"2006-01-02 15:04:05",
	"2006-01-02 15:04",
}

// looseLayouts are the string layouts ParseLoose tries after the digit forms.
var looseLayouts = []string{
	time.RFC3339Nano,
	time.RFC3339,
	"2006-01-02 15:04:05",
	"2006-01-02T15:04:05",
	"2006-01-02 15:04",
	"2006-01-02",
	"02/01/2006 15:04:05",
}

// ParseCompact converts a yyyyMMddHHmmss integer into a UTC time.
func ParseCompact(v int64) (time.Time, error) {
	if v <= 0 {
		return time.Time{}, fmt.Errorf("compact date %d: %w", v, ErrFormat)
	}
	s := strconv.FormatInt(v, 10)
	if len(s) != compactDigits {
		return time.Time{}, fmt.Errorf("compact date %d: want %d digits, got %d", v, compactDigits, len(s))
	}
	t, err := time.ParseInLocation(CompactLayout, s, time.UTC)
	if err != nil {
		return time.Time{}, fmt.Errorf("compact date %d: %w", v, err)
	}
	return t, nil
}

// FormatCompact encodes t (converted to UTC) as a yyyyMMddHHmmss integer.
func FormatCompact(t time.Time) int64 {
	v, _ := strconv.ParseInt(t.UTC().Format(CompactLayout), 10, 64)
	return v
}

// ParseUserDate parses the --date grammar: a bare YYYY-MM-DD (cutoff is the
// last second of that day), an RFC3339 or YYYY-MM-DDTHH:MM timestamp, or a
// raw 14-digit yyyyMMddHHmmss value. dateOnly reports which form matched.
func ParseUserDate(s string) (cutoff time.Time, dateOnly bool, err error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false, ErrEmpty
	}
	if isDigits(s) {
		switch len(s) {
		case compactDigits:
			v, _ := strconv.ParseInt(s, 10, 64)
			t, perr := ParseCompact(v)
			return t, false, perr
		case dateOnlyDigits:
			t, perr := time.ParseInLocation("20060102", s, time.UTC)
			if perr != nil {
				return time.Time{}, false, fmt.Errorf("date %q: %w", s, perr)
			}
			return endOfDay(t), true, nil
		default:
			return time.Time{}, false, fmt.Errorf("date %q: %w (want YYYY-MM-DD, RFC3339, or 14 digits)", s, ErrFormat)
		}
	}
	if t, perr := time.ParseInLocation("2006-01-02", s, time.UTC); perr == nil {
		return endOfDay(t), true, nil
	}
	for _, layout := range userLayouts {
		if t, perr := time.ParseInLocation(layout, s, time.UTC); perr == nil {
			return t.UTC(), false, nil
		}
	}
	return time.Time{}, false, fmt.Errorf("date %q: %w (want YYYY-MM-DD, RFC3339, or 14 digits)", s, ErrFormat)
}

// ParseLoose parses the status-feed strings defensively: compact digits
// first, then epoch digits, then RFC3339 and the common space-separated
// forms. Callers surface a failure as UNKNOWN_DATE rather than aborting.
func ParseLoose(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, ErrEmpty
	}
	if isDigits(s) {
		v, perr := strconv.ParseInt(s, 10, 64)
		if perr != nil {
			return time.Time{}, fmt.Errorf("date %q: %w", s, ErrFormat)
		}
		switch len(s) {
		case compactDigits:
			return ParseCompact(v)
		case dateOnlyDigits:
			t, derr := time.ParseInLocation("20060102", s, time.UTC)
			if derr != nil {
				return time.Time{}, fmt.Errorf("date %q: %w", s, derr)
			}
			return t, nil
		case unixMillisDigits:
			return time.UnixMilli(v).UTC(), nil
		case unixSecondsDigits:
			return time.Unix(v, 0).UTC(), nil
		default:
			return time.Time{}, fmt.Errorf("date %q: %w", s, ErrFormat)
		}
	}
	for _, layout := range looseLayouts {
		if t, perr := time.ParseInLocation(layout, s, time.UTC); perr == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("date %q: %w", s, ErrFormat)
}

// NewestAtOrBefore picks the newest backup at or before cutoff from a
// sub-source to compact-date map. An empty subSource considers every key;
// otherwise only the key matching case-insensitively is considered.
// Unparseable values are skipped. Ties resolve to the lexically smallest
// sub-source name so the result is deterministic.
func NewestAtOrBefore(dates map[string]int64, cutoff time.Time, subSource string) (string, int64, time.Time, bool) {
	keys := make([]string, 0, len(dates))
	for k := range dates {
		if subSource != "" && !strings.EqualFold(k, subSource) {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var (
		bestKey string
		bestRaw int64
		bestT   time.Time
		found   bool
	)
	for _, k := range keys {
		raw := dates[k]
		t, err := ParseCompact(raw)
		if err != nil {
			continue
		}
		if t.After(cutoff) {
			continue
		}
		if !found || t.After(bestT) {
			bestKey, bestRaw, bestT, found = k, raw, t, true
		}
	}
	return bestKey, bestRaw, bestT, found
}

func endOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 23, 59, 59, 0, time.UTC)
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
