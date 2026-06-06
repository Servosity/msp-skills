package xfin

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// msDateRe matches Xero's .NET-style date encoding: /Date(1714000000000)/ or
// /Date(1714000000000+1200)/ — milliseconds since the Unix epoch with an
// optional timezone offset suffix (which we ignore; the millis are UTC).
var msDateRe = regexp.MustCompile(`/Date\((-?\d+)(?:[+-]\d{4})?\)/`)

// isoLayouts are the ISO-8601 shapes Xero uses for some date fields.
var isoLayouts = []string{
	time.RFC3339,
	"2006-01-02T15:04:05",
	"2006-01-02T15:04:05.999",
	"2006-01-02",
}

// ParseXeroDate parses the date encodings Xero returns: the .NET
// /Date(ms+offset)/ form and several ISO-8601 forms. It returns the parsed
// time (UTC) and true on success, or the zero time and false when the string
// is empty or unparseable. Callers treat false as "no date".
func ParseXeroDate(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	if m := msDateRe.FindStringSubmatch(s); m != nil {
		ms, err := strconv.ParseInt(m[1], 10, 64)
		if err != nil {
			return time.Time{}, false
		}
		return time.UnixMilli(ms).UTC(), true
	}
	for _, layout := range isoLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC(), true
		}
	}
	return time.Time{}, false
}

// UpdatedDate extracts and parses the UpdatedDateUTC field common to every Xero
// entity, returning the time and whether it was present and parseable. Used by
// the `since` command to find records changed after a given moment.
func UpdatedDate(raw []byte) (time.Time, bool) {
	var s struct {
		UpdatedDateUTC string `json:"UpdatedDateUTC"`
	}
	if err := json.Unmarshal(raw, &s); err != nil {
		return time.Time{}, false
	}
	return ParseXeroDate(s.UpdatedDateUTC)
}

// daysBetween returns the whole-day difference (to - from), truncated to date
// boundaries so partial days don't push an invoice into the wrong bucket.
func daysBetween(from, to time.Time) int {
	fromDay := time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, time.UTC)
	toDay := time.Date(to.Year(), to.Month(), to.Day(), 0, 0, 0, 0, time.UTC)
	return int(toDay.Sub(fromDay).Hours() / 24)
}
