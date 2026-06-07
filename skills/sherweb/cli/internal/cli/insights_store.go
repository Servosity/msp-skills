// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"sherweb-pp-cli/internal/insights"
	"sherweb-pp-cli/internal/store"
)

// Resource types the transcendence commands read from the local store. The base
// `sync` command populates customers (rtCustomers) and the `distributor` table
// (payable charges); `deep-sync` fans out the per-customer endpoints into the
// normalized rows below so the cross-entity joins have data to work on.
const (
	rtCustomers       = "service-provider"  // GET /service-provider/v1/customers (base sync)
	rtSubscriptions   = "subscription"      // per-customer, populated by deep-sync
	rtReceivable      = "receivable-charge" // per-customer, populated by deep-sync
	rtUsage           = "usage"             // per-customer, populated by deep-sync
	rtPayableSnapshot = "payable-snapshot"  // append-only payable snapshots for drift
)

// openInsightStore opens the local store read-only at the default path (or the
// SHERWEB_DB override that defaultDBPath honors). Callers must Close().
func openInsightStore(ctx context.Context) (*store.Store, error) {
	dbPath := defaultDBPath("sherweb-cli")
	s, err := store.OpenWithContext(ctx, dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening local store at %s: %w (run 'sherweb-cli sync' and 'sherweb-cli deep-sync' first)", dbPath, err)
	}
	return s, nil
}

// --- defensive JSON field access -------------------------------------------

func asMap(raw json.RawMessage) map[string]any {
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil
	}
	return m
}

// pick returns the first present, non-nil value among the given keys
// (case-insensitive).
func pick(m map[string]any, keys ...string) any {
	if m == nil {
		return nil
	}
	lower := make(map[string]any, len(m))
	for k, v := range m {
		lower[strings.ToLower(k)] = v
	}
	for _, k := range keys {
		if v, ok := lower[strings.ToLower(k)]; ok && v != nil {
			return v
		}
	}
	return nil
}

func pstr(m map[string]any, keys ...string) string {
	switch v := pick(m, keys...).(type) {
	case string:
		return v
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(v)
	}
	return ""
}

func pnum(m map[string]any, keys ...string) float64 {
	switch v := pick(m, keys...).(type) {
	case float64:
		return v
	case string:
		f, _ := strconv.ParseFloat(strings.TrimSpace(v), 64)
		return f
	}
	return 0
}

// customerNameFromTags extracts a CustomerDisplayName-style tag from a Sherweb
// charge's tags array ([{name,value}]).
func customerFromTags(m map[string]any) (id, name string) {
	tags, ok := pick(m, "tags").([]any)
	if !ok {
		return "", ""
	}
	for _, t := range tags {
		tm, ok := t.(map[string]any)
		if !ok {
			continue
		}
		n := strings.ToLower(pstr(tm, "name"))
		v := pstr(tm, "value")
		switch {
		case strings.Contains(n, "customerid"):
			id = v
		case strings.Contains(n, "customerdisplayname"), strings.Contains(n, "customername"):
			name = v
		}
	}
	return id, name
}

// chargeFromMap builds an insights.Charge from a raw charge object, resolving the
// customer from explicit fields first, then from tags.
func chargeFromMap(m map[string]any) insights.Charge {
	id := pstr(m, "customerId", "customer_id")
	name := pstr(m, "customerName", "customerDisplayName", "customer_name")
	if id == "" || name == "" {
		tid, tname := customerFromTags(m)
		if id == "" {
			id = tid
		}
		if name == "" {
			name = tname
		}
	}
	return insights.Charge{
		CustomerID:   id,
		CustomerName: name,
		ProductName:  pstr(m, "productName", "product_name", "product"),
		SKU:          pstr(m, "sku"),
		ChargeName:   pstr(m, "chargeName", "charge_name", "name"),
		BillingCycle: pstr(m, "billingCycleType", "billingCycle", "billing_cycle"),
		Quantity:     pnum(m, "quantity", "qty"),
		SubTotal:     pnum(m, "subTotal", "sub_total", "amount", "total"),
		Currency:     pstr(m, "currency"),
	}
}

// flattenChargeRows turns stored rows — each of which may be a bare charge or a
// PayableCharges-style envelope with a charges[] array — into flat charge lines.
func flattenChargeRows(rows []json.RawMessage) []insights.Charge {
	out := []insights.Charge{}
	for _, raw := range rows {
		m := asMap(raw)
		if m == nil {
			continue
		}
		if arr, ok := pick(m, "charges").([]any); ok {
			for _, c := range arr {
				if cm, ok := c.(map[string]any); ok {
					out = append(out, chargeFromMap(cm))
				}
			}
			continue
		}
		if arr, ok := pick(m, "items").([]any); ok {
			for _, c := range arr {
				if cm, ok := c.(map[string]any); ok {
					out = append(out, chargeFromMap(cm))
				}
			}
			continue
		}
		out = append(out, chargeFromMap(m))
	}
	return out
}

// --- loaders ----------------------------------------------------------------

// loadPayableCharges reads payable charges from the dedicated `distributor`
// table (populated by `sync`). When month is non-empty (YYYY-MM) it restricts to
// rows whose billing period falls in that month.
func loadPayableCharges(s *store.Store, month string) ([]insights.Charge, error) {
	q := `SELECT data FROM distributor`
	var args []any
	if strings.TrimSpace(month) != "" {
		q += ` WHERE substr(period_from,1,7) = ? OR substr(period_to,1,7) = ?`
		args = append(args, month, month)
	}
	rows, err := s.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var raws []json.RawMessage
	for rows.Next() {
		var data []byte
		if err := rows.Scan(&data); err != nil {
			return nil, err
		}
		raws = append(raws, json.RawMessage(data))
	}
	return flattenChargeRows(raws), rows.Err()
}

// loadResourceRows reads every stored row of a resource type. It deliberately
// bypasses store.List, whose limit<=0 path applies a default 200-row cap that
// would silently truncate fleet-wide joins.
func loadResourceRows(s *store.Store, rt string) ([]json.RawMessage, error) {
	rows, err := s.Query(`SELECT data FROM resources WHERE resource_type = ? ORDER BY updated_at DESC`, rt)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []json.RawMessage
	for rows.Next() {
		var data []byte
		if err := rows.Scan(&data); err != nil {
			return nil, err
		}
		out = append(out, json.RawMessage(data))
	}
	return out, rows.Err()
}

func loadReceivableCharges(s *store.Store) ([]insights.Charge, error) {
	rows, err := loadResourceRows(s, rtReceivable)
	if err != nil {
		return nil, err
	}
	return flattenChargeRows(rows), nil
}

func subscriptionFromMap(m map[string]any) insights.Subscription {
	return insights.Subscription{
		ID:           pstr(m, "id", "subscriptionId", "subscription_id"),
		CustomerID:   pstr(m, "customerId", "customer_id"),
		CustomerName: pstr(m, "customerName", "customerDisplayName"),
		ProductName:  pstr(m, "productName", "product_name", "product"),
		SKU:          pstr(m, "sku"),
		Quantity:     pnum(m, "quantity", "qty", "seats"),
		BillingCycle: pstr(m, "billingCycle", "billingCycleType"),
		Status:       pstr(m, "status"),
		UnitPrice:    pnum(m, "unitPrice", "unit_price", "price"),
		Currency:     pstr(m, "currency"),
	}
}

func loadSubscriptions(s *store.Store) ([]insights.Subscription, error) {
	rows, err := loadResourceRows(s, rtSubscriptions)
	if err != nil {
		return nil, err
	}
	out := []insights.Subscription{}
	for _, raw := range rows {
		m := asMap(raw)
		if m == nil {
			continue
		}
		if arr, ok := pick(m, "items").([]any); ok {
			for _, it := range arr {
				if im, ok := it.(map[string]any); ok {
					out = append(out, subscriptionFromMap(im))
				}
			}
			continue
		}
		out = append(out, subscriptionFromMap(m))
	}
	return out, nil
}

func loadUsage(s *store.Store) ([]insights.Usage, error) {
	rows, err := loadResourceRows(s, rtUsage)
	if err != nil {
		return nil, err
	}
	out := []insights.Usage{}
	for _, raw := range rows {
		m := asMap(raw)
		if m == nil {
			continue
		}
		consume := func(um map[string]any) {
			out = append(out, insights.Usage{
				CustomerID:     pstr(um, "customerId", "customer_id"),
				SubscriptionID: pstr(um, "subscriptionId", "subscription_id"),
				SKU:            pstr(um, "sku"),
				Used:           pnum(um, "used", "usage", "quantity", "consumedUnits"),
			})
		}
		if arr, ok := pick(m, "items").([]any); ok {
			for _, it := range arr {
				if im, ok := it.(map[string]any); ok {
					consume(im)
				}
			}
			continue
		}
		consume(m)
	}
	return out, nil
}

// loadPayableSnapshots returns payable-charge snapshots ordered oldest→newest by
// their snapshot id (an RFC3339-ish timestamp written by deep-sync).
func loadPayableSnapshots(s *store.Store) (ids []string, byID map[string][]insights.Charge, err error) {
	rows, err := s.Query(`SELECT id, data FROM resources WHERE resource_type = ? ORDER BY id ASC`, rtPayableSnapshot)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	byID = map[string][]insights.Charge{}
	for rows.Next() {
		var id string
		var data []byte
		if err := rows.Scan(&id, &data); err != nil {
			return nil, nil, err
		}
		ids = append(ids, id)
		byID[id] = flattenChargeRows([]json.RawMessage{json.RawMessage(data)})
	}
	sort.Strings(ids)
	return ids, byID, rows.Err()
}
