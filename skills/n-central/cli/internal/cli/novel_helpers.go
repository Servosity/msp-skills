// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0.
// Hand-written novel-feature support code. Not generated.

package cli

import (
	"encoding/json"
	"net/url"
	"path/filepath"
	"strings"
	"time"
)

// unwrapData unwraps an N-central response envelope of the shape
// {"data":[...], ...} into the inner array. It is tolerant: a bare top-level
// array is returned as-is, and common alternate wrapper keys are tried so a
// minor envelope drift does not silently return zero rows. A single object is
// returned wrapped in a one-element slice so callers always iterate.
func unwrapData(raw json.RawMessage) []json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	// Bare array.
	var arr []json.RawMessage
	if json.Unmarshal(raw, &arr) == nil {
		return arr
	}
	// Object envelope: try canonical N-central key first, then fallbacks.
	var obj map[string]json.RawMessage
	if json.Unmarshal(raw, &obj) == nil {
		for _, key := range []string{"data", "results", "items", "records", "entries"} {
			if inner, ok := obj[key]; ok {
				var items []json.RawMessage
				if json.Unmarshal(inner, &items) == nil {
					return items
				}
			}
		}
		// Not an envelope we recognize — treat the object itself as one row.
		return []json.RawMessage{raw}
	}
	return nil
}

// decodeObj decodes a JSON object into a generic map, preserving numbers as
// json.Number so large integer IDs survive without scientific-notation
// corruption. Returns nil on any decode failure so callers can skip the row.
func decodeObj(raw json.RawMessage) map[string]any {
	var obj map[string]any
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.UseNumber()
	if dec.Decode(&obj) != nil {
		return nil
	}
	return obj
}

// firstField returns the first non-nil value for any of the candidate keys,
// matched case-insensitively against the object's keys. It also descends one
// level into a nested "_extra" object, which N-central active-issue rows use
// to carry denormalized display fields.
func firstField(obj map[string]any, keys ...string) any {
	if obj == nil {
		return nil
	}
	lower := make(map[string]any, len(obj))
	for k, v := range obj {
		lower[strings.ToLower(k)] = v
	}
	for _, k := range keys {
		if v, ok := lower[strings.ToLower(k)]; ok && v != nil {
			return v
		}
	}
	return nil
}

// extraField pulls a value from a nested "_extra" object on an active-issue
// row, matched case-insensitively. Returns nil when absent.
func extraField(obj map[string]any, keys ...string) any {
	if obj == nil {
		return nil
	}
	for k, v := range obj {
		if strings.EqualFold(k, "_extra") {
			if extra, ok := v.(map[string]any); ok {
				return firstField(extra, keys...)
			}
		}
	}
	return nil
}

// asString renders a JSON-decoded scalar (string, json.Number, bool, nil) as a
// trimmed string. Objects/arrays render as compact JSON. Returns "" for nil.
func asString(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(t)
	case json.Number:
		return strings.TrimSpace(t.String())
	case bool:
		if t {
			return "true"
		}
		return "false"
	default:
		b, err := json.Marshal(t)
		if err != nil {
			return ""
		}
		return string(b)
	}
}

// asInt renders a JSON-decoded scalar as an int, tolerating json.Number,
// float64, and numeric strings. Returns (0, false) when it can't.
func asInt(v any) (int, bool) {
	switch t := v.(type) {
	case json.Number:
		if n, err := t.Int64(); err == nil {
			return int(n), true
		}
		if f, err := t.Float64(); err == nil {
			return int(f), true
		}
	case float64:
		return int(t), true
	case int:
		return t, true
	case int64:
		return int(t), true
	case string:
		var num json.Number = json.Number(strings.TrimSpace(t))
		if n, err := num.Int64(); err == nil {
			return int(n), true
		}
	}
	return 0, false
}

// hostFromBaseURL extracts a short server label from a base URL, e.g.
// "https://acme.ncod.n-able.com/api" -> "acme.ncod.n-able.com". Falls back to
// the raw string when it can't be parsed.
func hostFromBaseURL(baseURL string) string {
	if baseURL == "" {
		return ""
	}
	u, err := url.Parse(baseURL)
	if err != nil || u.Host == "" {
		return strings.TrimSpace(baseURL)
	}
	return u.Host
}

// serverLabelForDB derives a stable server label for a DB file path: the file
// basename with its extension stripped. Used to tag fanout results.
func serverLabelForDB(dbPath string) string {
	base := filepath.Base(dbPath)
	if ext := filepath.Ext(base); ext != "" {
		base = strings.TrimSuffix(base, ext)
	}
	return base
}

// joinPath joins non-empty segments with " > " to form a human-readable
// location path (server > so > customer > site). Empty segments are skipped so
// a missing middle tier doesn't leave a dangling separator.
func joinPath(segments ...string) string {
	out := make([]string, 0, len(segments))
	for _, s := range segments {
		s = strings.TrimSpace(s)
		if s != "" {
			out = append(out, s)
		}
	}
	return strings.Join(out, " > ")
}

// parseDateFlag parses a YYYY-MM-DD date flag. Returns (zero, false) when the
// string is empty or unparseable.
func parseDateFlag(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t, true
	}
	return time.Time{}, false
}

// parseFlexibleTime attempts to parse a timestamp from an API field that may be
// RFC3339, a date, or a unix-epoch number (seconds or milliseconds). Returns
// (zero, false) on failure.
func parseFlexibleTime(v any) (time.Time, bool) {
	switch t := v.(type) {
	case json.Number:
		if n, err := t.Int64(); err == nil {
			return epochToTime(n), true
		}
	case float64:
		return epochToTime(int64(t)), true
	case string:
		s := strings.TrimSpace(t)
		if s == "" {
			return time.Time{}, false
		}
		for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02 15:04:05", "2006-01-02"} {
			if parsed, err := time.Parse(layout, s); err == nil {
				return parsed, true
			}
		}
		// Numeric string (epoch).
		var num json.Number = json.Number(s)
		if n, err := num.Int64(); err == nil {
			return epochToTime(n), true
		}
	}
	return time.Time{}, false
}

// equalFoldTrim compares two strings case-insensitively after trimming
// surrounding whitespace on both.
func equalFoldTrim(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}

// joinComma joins parts with ", ".
func joinComma(parts []string) string {
	return strings.Join(parts, ", ")
}

// epochToTime interprets n as unix seconds when it looks like seconds, or
// milliseconds when it's large enough to be an ms timestamp.
func epochToTime(n int64) time.Time {
	// Values past ~year 2286 in seconds are almost certainly milliseconds.
	if n > 1e12 {
		return time.UnixMilli(n).UTC()
	}
	return time.Unix(n, 0).UTC()
}
