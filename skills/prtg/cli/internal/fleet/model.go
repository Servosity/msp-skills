// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: JSON helpers shared by the fleet commands.

package fleet

import (
	"encoding/json"
	"strings"
	"time"

	"prtg-pp-cli/internal/cliutil"
)

// SecretField reports a property name that looks like a credential (the one
// rule in cliutil, shared with the local store).
func SecretField(name string) bool { return cliutil.SecretFieldName(name) }

// Redacted replaces credential-bearing values with "[redacted]".
const Redacted = cliutil.RedactedValue

// Redact walks a decoded JSON value and masks credential fields in place.
func Redact(v any) any { return cliutil.RedactSecretFields(v) }

// RedactJSON masks credential fields in a JSON body.
func RedactJSON(body json.RawMessage) (json.RawMessage, error) {
	var v any
	if err := json.Unmarshal(body, &v); err != nil {
		return nil, err
	}
	return json.Marshal(Redact(v))
}

// Items returns the rows of a body: a bare array (every PRTG v2 list), or the
// body itself as a single row. A scalar becomes {"value": scalar}.
func Items(body json.RawMessage) []map[string]any {
	var arr []json.RawMessage
	if err := json.Unmarshal(body, &arr); err == nil {
		out := make([]map[string]any, 0, len(arr))
		for _, raw := range arr {
			var row map[string]any
			if err := json.Unmarshal(raw, &row); err != nil {
				var scalar any
				if json.Unmarshal(raw, &scalar) == nil {
					row = map[string]any{"value": scalar}
				}
			}
			if row != nil {
				out = append(out, row)
			}
		}
		return out
	}
	var obj map[string]any
	if err := json.Unmarshal(body, &obj); err == nil && obj != nil {
		return []map[string]any{obj}
	}
	var scalar any
	if json.Unmarshal(body, &scalar) == nil && scalar != nil {
		return []map[string]any{{"value": scalar}}
	}
	return []map[string]any{}
}

// ParseTime reads a PRTG ISO-8601 timestamp. Empty values and placeholders
// (year 1, and PRTG's "zero date" 1899-12-30) are unset.
func ParseTime(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04:05", "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			if t.Year() < 1900 {
				return time.Time{}, false
			}
			return t, true
		}
	}
	return time.Time{}, false
}

// Str reads a string field from a decoded row ("" when absent or not a string).
func Str(row map[string]any, key string) string {
	if s, ok := row[key].(string); ok {
		return s
	}
	return ""
}
