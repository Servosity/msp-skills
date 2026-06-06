// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel-feature helpers (Phase 3). Shared by the reconcile, mrr,
// overage, since, spend, and company-show commands. Not generator-managed.

package cli

import (
	"context"
	"encoding/json"
	"strconv"

	"pax8-pp-cli/internal/store"
)

// pax8ListObjects loads every row of a resource table's `data` column and
// decodes each into a generic JSON object. Rows that fail to decode are
// skipped, mirroring analytics.go's tolerant aggregation.
func pax8ListObjects(db *store.Store, resourceType string) ([]map[string]any, error) {
	raws, err := db.List(resourceType, 0)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(raws))
	for _, raw := range raws {
		var obj map[string]any
		if err := json.Unmarshal(raw, &obj); err != nil {
			continue
		}
		out = append(out, obj)
	}
	return out, nil
}

// pax8QueryObjects runs a SELECT returning a single `data` JSON column and
// decodes each row into a generic JSON object.
func pax8QueryObjects(ctx context.Context, db *store.Store, query string, args ...any) ([]map[string]any, error) {
	rows, err := db.DB().QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var obj map[string]any
		if err := json.Unmarshal(raw, &obj); err == nil {
			out = append(out, obj)
		}
	}
	return out, rows.Err()
}

// pax8Num coerces a decoded-JSON value (number or numeric string) to float64.
// Pax8 encodes some money fields as JSON strings, so the string path matters.
func pax8Num(v any) (float64, bool) {
	switch t := v.(type) {
	case float64:
		return t, true
	case json.Number:
		f, err := t.Float64()
		return f, err == nil
	case string:
		if t == "" {
			return 0, false
		}
		f, err := strconv.ParseFloat(t, 64)
		return f, err == nil
	}
	return 0, false
}

// pax8Str coerces a decoded-JSON value to a display string.
func pax8Str(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case json.Number:
		return t.String()
	case bool:
		return strconv.FormatBool(t)
	}
	return ""
}

// pax8Field returns the first present, non-nil value (a present empty string is returned as-is) among the supplied key spellings
// (Pax8 responses are camelCase; the local typed columns are snake_case).
func pax8Field(obj map[string]any, keys ...string) any {
	for _, k := range keys {
		if v, ok := obj[k]; ok && v != nil {
			return v
		}
	}
	return nil
}

// pax8FieldStr is pax8Field + pax8Str.
func pax8FieldStr(obj map[string]any, keys ...string) string {
	return pax8Str(pax8Field(obj, keys...))
}

// pax8FieldNum is pax8Field + pax8Num.
func pax8FieldNum(obj map[string]any, keys ...string) (float64, bool) {
	return pax8Num(pax8Field(obj, keys...))
}

// pax8NameByID builds an id->name lookup from a list of store objects, trying
// idKeys then nameKeys in order. Shared by reconcile, spend, mrr, and since.
func pax8NameByID(objs []map[string]any, idKeys, nameKeys []string) map[string]string {
	nameByID := make(map[string]string, len(objs))
	for _, o := range objs {
		if id := pax8FieldStr(o, idKeys...); id != "" {
			nameByID[id] = pax8FieldStr(o, nameKeys...)
		}
	}
	return nameByID
}
