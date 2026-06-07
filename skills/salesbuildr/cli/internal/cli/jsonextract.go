// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"strconv"
	"strings"
)

// decodeObj unmarshals a raw JSON object into a generic map. Returns an empty
// map (never nil) on error so the g* accessors are always safe to call.
func decodeObj(raw json.RawMessage) map[string]any {
	m := map[string]any{}
	_ = json.Unmarshal(raw, &m)
	return m
}

// gstr returns a string field, coercing numbers/bools to their literal form.
func gstr(m map[string]any, key string) string {
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(t)
	}
	return ""
}

// gnum returns a numeric field. Accepts JSON numbers and numeric strings
// ("1.91"), which Salesbuildr occasionally uses for money fields.
func gnum(m map[string]any, key string) float64 {
	v, ok := m[key]
	if !ok || v == nil {
		return 0
	}
	switch t := v.(type) {
	case float64:
		return t
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(t), 64)
		if err == nil {
			return f
		}
	}
	return 0
}

// gobjName extracts the "name" (then "displayValue") of a nested object field.
func gobjName(m map[string]any, key string) string {
	sub, ok := m[key].(map[string]any)
	if !ok {
		return ""
	}
	if s := gstr(sub, "name"); s != "" {
		return s
	}
	return gstr(sub, "displayValue")
}

// gobjID extracts the "id" of a nested object field.
func gobjID(m map[string]any, key string) string {
	sub, ok := m[key].(map[string]any)
	if !ok {
		return ""
	}
	return gstr(sub, "id")
}

// gobjNum extracts a numeric subfield of a nested object field.
func gobjNum(m map[string]any, key, sub string) float64 {
	o, ok := m[key].(map[string]any)
	if !ok {
		return 0
	}
	return gnum(o, sub)
}

// gArray returns the elements of an array field as raw JSON messages.
func gArray(m map[string]any, key string) []json.RawMessage {
	v, ok := m[key].([]any)
	if !ok {
		return nil
	}
	out := make([]json.RawMessage, 0, len(v))
	for _, el := range v {
		b, err := json.Marshal(el)
		if err == nil {
			out = append(out, b)
		}
	}
	return out
}

// statusLabel normalizes an opportunity status, which the API may encode as a
// string, an object ({name|displayValue}), or only a statusId.
func statusLabel(v any, fallbackID string) string {
	switch t := v.(type) {
	case string:
		if t != "" {
			return t
		}
	case map[string]any:
		if s := gstr(t, "name"); s != "" {
			return s
		}
		if s := gstr(t, "displayValue"); s != "" {
			return s
		}
	}
	return fallbackID
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
