// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: response shapes and redaction shared by the fleet commands.

package fleet

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"jamf-pro-pp-cli/internal/cliutil"
)

// Items returns the rows of a body in either Jamf API's shape: the Jamf Pro
// API page {"totalCount":N,"results":[...]}, a Classic API list
// {"policies":[...]}, a Classic object {"policy":{...}}, a bare array, or the
// body itself as a single row. A null list member is an error, never a nil
// row (a nil row would crash the caller and abort the whole fleet report).
func Items(body json.RawMessage) ([]map[string]any, error) {
	var arr []json.RawMessage
	if err := json.Unmarshal(body, &arr); err == nil {
		return rowsOf(arr)
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(body, &obj); err != nil {
		var scalar any
		if json.Unmarshal(body, &scalar) == nil && scalar != nil {
			return []map[string]any{{"value": scalar}}, nil
		}
		return []map[string]any{}, nil
	}
	if raw, ok := obj["results"]; ok {
		if err := json.Unmarshal(raw, &arr); err == nil {
			return rowsOf(arr)
		}
	}
	if len(obj) == 1 {
		for _, raw := range obj {
			if err := json.Unmarshal(raw, &arr); err == nil {
				return rowsOf(arr)
			}
			var inner map[string]any
			if err := json.Unmarshal(raw, &inner); err == nil && inner != nil {
				return []map[string]any{inner}, nil
			}
		}
	}
	return rowsOf([]json.RawMessage{body})
}

func rowsOf(arr []json.RawMessage) ([]map[string]any, error) {
	out := make([]map[string]any, 0, len(arr))
	for i, raw := range arr {
		var row map[string]any
		if err := json.Unmarshal(raw, &row); err != nil {
			var scalar any
			if json.Unmarshal(raw, &scalar) == nil {
				out = append(out, map[string]any{"value": scalar})
			}
			continue
		}
		if row == nil {
			return nil, fmt.Errorf("unexpected response: list entry %d is null", i)
		}
		out = append(out, row)
	}
	return out, nil
}

// The credential rule and redaction funnel live in cliutil (jamf_redact.go),
// shared with the credential holder, the output chokepoint and the local
// store; these names keep the fleet commands' call sites.
const (
	Redacted             = cliutil.Redacted
	RedactedDigestPrefix = cliutil.RedactedDigestPrefix
)

// SecretField reports a property name that looks like a credential.
func SecretField(name string) bool { return cliutil.SecretField(name) }

// Redact is the one output funnel every fleet result passes through.
func Redact(v any) any { return cliutil.Redact(v) }

// RedactString applies the string funnel to free text (key unknown).
func RedactString(s string) string { return cliutil.RedactString(s) }

// Digest is the printable stand-in for an opaque document's contents.
func Digest(s string) string { return cliutil.Digest(s) }

// RedactJSON masks credential fields in a raw JSON body.
func RedactJSON(body json.RawMessage) (json.RawMessage, error) { return cliutil.RedactJSON(body) }

// ParseTime reads a Jamf timestamp (RFC 3339, or the Classic API's
// "2006-01-02 15:04:05" / epoch-millisecond forms). Empty values are unset.
func ParseTime(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	if ms, err := strconv.ParseInt(s, 10, 64); err == nil && ms > 0 {
		return time.UnixMilli(ms).UTC(), true
	}
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04:05.000Z0700", "2006-01-02T15:04:05", "2006-01-02 15:04:05", "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			if t.Year() <= 1970 {
				return time.Time{}, false
			}
			return t, true
		}
	}
	return time.Time{}, false
}

// DaysUntil is whole days from now to t (negative once t has passed).
func DaysUntil(t, now time.Time) int {
	d := t.Sub(now)
	days := int(d / (24 * time.Hour))
	if d < 0 && d%(24*time.Hour) != 0 {
		days--
	}
	return days
}

// CompareVersions compares dotted numeric versions like 11.10.1 or 15.4.
// Non-numeric suffixes ("11.10.1-t1736" ) are ignored per part; missing
// parts compare as 0.
func CompareVersions(a, b string) int {
	pa, pb := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(pa) || i < len(pb); i++ {
		x, y := part(pa, i), part(pb, i)
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	return 0
}

func part(p []string, i int) int {
	if i >= len(p) {
		return 0
	}
	s := strings.TrimSpace(p[i])
	end := 0
	for end < len(s) && s[end] >= '0' && s[end] <= '9' {
		end++
	}
	n, err := strconv.Atoi(s[:end])
	if err != nil {
		return 0
	}
	return n
}
