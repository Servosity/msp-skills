// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored helpers shared by the novel transcendence commands
// (reconcile, payments unpaid, subs changed, pipeline, pipeline stale,
// company show). Kept in a separate file so regenerations preserve it.

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"appdirect-pp-cli/internal/store"
)

// Resource-type keys as written by the syncer into the generic resources table.
const (
	rtSubscriptions = "billing-v1-subscriptions"
	rtInvoices      = "billing-v1-invoices"
	rtPayments      = "billing-v1-payments"
	rtCompanies     = "account-v2-companies"
	rtUsers         = "account" // bare "account" resource_type -> /account/v2/users (see sync.go path map); NOT account-v2-users
	rtOpportunities = "assisted-sales-v1-opportunities"
)

// loadResourceObjects returns every synced row of the given resource_type
// decoded into a generic JSON object map.
func loadResourceObjects(ctx context.Context, db *store.Store, resourceType string) ([]map[string]any, error) {
	rows, err := db.DB().QueryContext(ctx,
		`SELECT data FROM resources WHERE resource_type = ?`, resourceType)
	if err != nil {
		return nil, fmt.Errorf("querying %s: %w", resourceType, err)
	}
	defer rows.Close()

	out := make([]map[string]any, 0)
	for rows.Next() {
		var data []byte
		if err := rows.Scan(&data); err != nil {
			return nil, fmt.Errorf("scanning %s row: %w", resourceType, err)
		}
		obj, err := store.DecodeJSONObject(data)
		if err != nil {
			continue // tolerate non-object rows
		}
		out = append(out, obj)
	}
	return out, rows.Err()
}

// novelStr extracts a string field from a decoded JSON object. Numeric IDs
// (json.Number from store.DecodeJSONObject's UseNumber, or float64 from plain
// unmarshal) render as clean integer strings, never scientific notation.
func novelStr(m map[string]any, key string) string {
	switch v := m[key].(type) {
	case string:
		return v
	case json.Number:
		return v.String()
	case float64:
		if v == float64(int64(v)) {
			return fmt.Sprintf("%d", int64(v))
		}
		return fmt.Sprintf("%v", v)
	}
	return ""
}

// novelNum extracts a numeric field from a decoded object, tolerating both
// json.Number (UseNumber decoding) and float64 (plain unmarshal).
func novelNum(m map[string]any, key string) (float64, bool) {
	switch v := m[key].(type) {
	case float64:
		return v, true
	case json.Number:
		f, err := v.Float64()
		return f, err == nil
	}
	return 0, false
}

// novelEpochMS extracts an epoch-milliseconds field as a time.Time.
func novelEpochMS(m map[string]any, key string) (time.Time, bool) {
	v, ok := novelNum(m, key)
	if !ok || v <= 0 {
		return time.Time{}, false
	}
	return time.UnixMilli(int64(v)).UTC(), true
}

// novelNested returns a nested object field (e.g. LinkWS or expanded entity).
func novelNested(m map[string]any, key string) map[string]any {
	if v, ok := m[key].(map[string]any); ok {
		return v
	}
	return nil
}

// novelRefID extracts the identifier from a LinkWS-style reference
// ({"href","id"}) or an expanded entity ({"uuid"|"id"|"companyId",...}).
func novelRefID(m map[string]any, key string) string {
	ref := novelNested(m, key)
	if ref == nil {
		return ""
	}
	for _, k := range []string{"id", "uuid", "companyId"} {
		if s := novelStr(ref, k); s != "" {
			return s
		}
	}
	return ""
}

// novelRefName extracts a display name from an expanded reference when the
// syncer's expand parameter inlined the full entity.
func novelRefName(m map[string]any, key string) string {
	ref := novelNested(m, key)
	if ref == nil {
		return ""
	}
	return novelStr(ref, "name")
}

// companyNameIndex builds uuid->name from synced account-v2-companies rows.
func companyNameIndex(companies []map[string]any) map[string]string {
	idx := make(map[string]string, len(companies))
	for _, c := range companies {
		id := novelStr(c, "uuid")
		if id == "" {
			id = novelStr(c, "id")
		}
		if id != "" {
			idx[id] = novelStr(c, "name")
		}
	}
	return idx
}

// isoOrEmpty formats a time as RFC3339, or "" for the zero value.
func isoOrEmpty(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}

// ageDays returns whole days elapsed since t, never negative.
func ageDays(t time.Time, now time.Time) int {
	if t.IsZero() || t.After(now) {
		return 0
	}
	return int(now.Sub(t).Hours() / 24)
}
