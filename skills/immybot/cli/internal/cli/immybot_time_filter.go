// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored (NOT generated): the one reader of a mirrored row's time for
// every age or window filter in the novel commands. Each resource's documented
// date fields are listed here in the order a filter reads them, and a row is
// placed in time by the first one that parses.
//
// Fail closed: a row with no usable time has no age. It never passes --since or
// --older-than and is never put in an age bucket as if it were young; commands
// count it as undated and say so, so it is not silently dropped either.
//
// pp:data-source local

package cli

import (
	"database/sql"
	"strings"
	"time"
)

// immyActionTimeFields is the order a maintenance action's time is read in
// (GetMaintenanceActionResponse; all four are optional date-times):
//   - endTime: when the action finished, which is when its result was decided.
//     A failure happened when the action ended.
//   - startTime: when it ran, for an action with no recorded end (still
//     running, or ended without one).
//   - updatedDateUTC: the row's last write, which records the result; a proxy
//     when neither execution time is recorded.
//   - createdDateUTC: when the action was planned, the earliest bound; last.
var immyActionTimeFields = []string{"endTime", "startTime", "updatedDateUTC", "createdDateUTC"}

// immyOnboardingTimeFields: GetOnboardingComputerResponse documents one date,
// updatedDate, and it is optional.
var immyOnboardingTimeFields = []string{"updatedDate"}

// immyTimeColumns is the SELECT fragment for fields, in order. The names are
// the constants above, never user input.
func immyTimeColumns(fields []string) string {
	cols := make([]string, 0, len(fields))
	for _, f := range fields {
		cols = append(cols, "json_extract(data,'$."+f+"')")
	}
	return strings.Join(cols, ", ")
}

// immyTimeScan holds one row's scanned immyTimeColumns.
type immyTimeScan struct {
	fields []string
	vals   []sql.NullString
}

func newImmyTimeScan(fields []string) *immyTimeScan {
	return &immyTimeScan{fields: fields, vals: make([]sql.NullString, len(fields))}
}

// dest returns the scan destinations for immyTimeColumns(fields).
func (s *immyTimeScan) dest() []any {
	out := make([]any, len(s.vals))
	for i := range s.vals {
		out[i] = &s.vals[i]
	}
	return out
}

// raw returns one scanned field as mirrored, trimmed, whether or not it parses.
func (s *immyTimeScan) raw(field string) string {
	for i, f := range s.fields {
		if f == field {
			return strings.TrimSpace(nullStr(s.vals[i]))
		}
	}
	return ""
}

// time places the scanned row in time.
func (s *immyTimeScan) time() immyRowTime {
	for i, v := range s.vals {
		if t, ok := immyParseTimestamp(nullStr(v)); ok {
			return immyRowTime{At: t, Field: s.fields[i], Raw: strings.TrimSpace(v.String), Known: true}
		}
	}
	return immyRowTime{}
}

// immyRowTime is a row's time as a filter reads it: the first documented field
// that parsed, or Known false when none did.
type immyRowTime struct {
	At    time.Time
	Field string
	Raw   string
	Known bool
}

// NotBefore reports whether the row passes --since: its time is known and at
// or after cutoff.
func (t immyRowTime) NotBefore(cutoff time.Time) bool {
	return t.Known && !t.At.Before(cutoff)
}

// AgeAtLeast reports whether the row passes --older-than: its time is known
// and at least minAge before now.
func (t immyRowTime) AgeAtLeast(now time.Time, minAge time.Duration) bool {
	return t.Known && now.Sub(t.At) >= minAge
}

// immyParseTimestamp is immyParseTime for a filter: it also rejects .NET's
// DateTime.MinValue and MaxValue (years 1 and 9999), which a never-set
// non-nullable DateTime serializes as. Read as a time, MinValue would make a
// row two thousand years old and MaxValue would make it newer than any cutoff.
func immyParseTimestamp(raw string) (time.Time, bool) {
	t, ok := immyParseTime(raw)
	if !ok || t.Year() <= 1 || t.Year() >= 9999 {
		return time.Time{}, false
	}
	return t, true
}

// immyUndatedBucket is the age bucket of a row whose time is unknown.
const immyUndatedBucket = "unknown"
