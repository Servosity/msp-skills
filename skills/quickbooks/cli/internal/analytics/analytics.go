// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

// Package analytics holds the pure, offline financial-analytics logic that powers
// QuickBooks Online's transcendence commands (ar-aging, ap-aging, balances,
// stale invoices, unapplied payments, top customers, vendor spend, dupes).
//
// Every function operates on decoded JSON objects (map[string]any), so it is
// independent of the CLI, the store, and the network — and fully unit-testable.
// All numeric accessors are json.Number-safe: the store decodes with
// json.Decoder.UseNumber(), so amounts arrive as json.Number, not float64.
package analytics

import (
	"encoding/json"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Num reads a numeric field from a decoded QBO object, tolerating json.Number,
// float64, int, and numeric strings (QBO sometimes encodes amounts as strings).
// Missing/unparseable values return 0.
func Num(obj map[string]any, key string) float64 {
	return toFloat(obj[key])
}

func toFloat(v any) float64 {
	switch n := v.(type) {
	case nil:
		return 0
	case json.Number:
		f, _ := n.Float64()
		return f
	case float64:
		return n
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(n), 64)
		if err != nil {
			return 0
		}
		return f
	default:
		return 0
	}
}

// Str reads a string field, tolerating non-string scalars.
func Str(obj map[string]any, key string) string {
	switch s := obj[key].(type) {
	case nil:
		return ""
	case string:
		return s
	case json.Number:
		return s.String()
	case bool:
		return strconv.FormatBool(s)
	default:
		return ""
	}
}

// Nested descends a chain of object keys, e.g. Nested(obj, "CustomerRef", "name").
// Returns "" if any segment is missing or not an object.
func Nested(obj map[string]any, path ...string) string {
	cur := obj
	for i, p := range path {
		if cur == nil {
			return ""
		}
		v, ok := cur[p]
		if !ok {
			return ""
		}
		if i == len(path)-1 {
			switch s := v.(type) {
			case string:
				return s
			case json.Number:
				return s.String()
			default:
				return ""
			}
		}
		next, ok := v.(map[string]any)
		if !ok {
			return ""
		}
		cur = next
	}
	return ""
}

// ParseDate parses the common QBO date shapes (YYYY-MM-DD, RFC3339, and a few
// datetime variants). The second return is false when no layout matched.
func ParseDate(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	layouts := []string{
		"2006-01-02",
		time.RFC3339,
		"2006-01-02T15:04:05",
		"2006-01-02T15:04:05Z07:00",
		"2006-01-02 15:04:05",
	}
	for _, l := range layouts {
		if t, err := time.Parse(l, s); err == nil {
			return t, true
		}
	}
	// Some QBO datetimes carry a trailing timezone offset without a colon.
	if len(s) >= 10 {
		if t, err := time.Parse("2006-01-02", s[:10]); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// DaysPastDue returns whole days between dueDate and now (positive = overdue).
// A not-yet-due item returns a negative number; an unparseable date returns 0.
func DaysPastDue(dueDate string, now time.Time) int {
	d, ok := ParseDate(dueDate)
	if !ok {
		return 0
	}
	return int(math.Floor(now.Sub(d).Hours() / 24))
}

// --- Aging -----------------------------------------------------------------

// Bucket names, in display order. "0-30" absorbs not-yet-due items (negative
// days past due) so the four-bucket headline holds.
var AgingBuckets = []string{"0-30", "31-60", "61-90", "90+"}

func bucketFor(daysPastDue int) string {
	switch {
	case daysPastDue <= 30:
		return "0-30"
	case daysPastDue <= 60:
		return "31-60"
	case daysPastDue <= 90:
		return "61-90"
	default:
		return "90+"
	}
}

// AgingEntityRow is one customer (AR) or vendor (AP) roll-up.
type AgingEntityRow struct {
	Name    string             `json:"name"`
	ID      string             `json:"id"`
	Total   float64            `json:"total"`
	Buckets map[string]float64 `json:"buckets"`
}

// AgingReport is the full aging output.
type AgingReport struct {
	AsOf     string             `json:"as_of"`
	Field    string             `json:"aging_by"`
	Total    float64            `json:"total_outstanding"`
	Count    int                `json:"open_count"`
	Buckets  map[string]float64 `json:"buckets"`
	Entities []AgingEntityRow   `json:"by_entity"`
}

// Aging buckets open balances by age and rolls them up per entity (customer for
// AR, vendor for AP). Only rows with balance > 0 are counted. nameKeys/idKeys are
// the nested ref path to the counterparty (e.g. "CustomerRef","name").
func Aging(items []map[string]any, dueField, fallbackDateField, amtField string, refKey string, now time.Time) AgingReport {
	rep := AgingReport{
		AsOf:    now.Format("2006-01-02"),
		Field:   amtField,
		Buckets: map[string]float64{},
	}
	for _, b := range AgingBuckets {
		rep.Buckets[b] = 0
	}
	byEntity := map[string]*AgingEntityRow{}
	for _, it := range items {
		bal := Num(it, amtField)
		if bal <= 0 {
			continue
		}
		due := Str(it, dueField)
		if due == "" && fallbackDateField != "" {
			due = Str(it, fallbackDateField)
		}
		bkt := bucketFor(DaysPastDue(due, now))
		name := Nested(it, refKey, "name")
		id := Nested(it, refKey, "value")
		if name == "" {
			name = "(unspecified)"
		}
		key := id + "|" + name
		row := byEntity[key]
		if row == nil {
			row = &AgingEntityRow{Name: name, ID: id, Buckets: map[string]float64{}}
			for _, bb := range AgingBuckets {
				row.Buckets[bb] = 0
			}
			byEntity[key] = row
		}
		row.Total += bal
		row.Buckets[bkt] += bal
		rep.Total += bal
		rep.Buckets[bkt] += bal
		rep.Count++
	}
	rep.Entities = make([]AgingEntityRow, 0, len(byEntity))
	for _, r := range byEntity {
		rep.Entities = append(rep.Entities, *r)
	}
	sort.Slice(rep.Entities, func(i, j int) bool {
		if rep.Entities[i].Total != rep.Entities[j].Total {
			return rep.Entities[i].Total > rep.Entities[j].Total
		}
		return rep.Entities[i].Name < rep.Entities[j].Name
	})
	return rep
}

// --- Stale invoices --------------------------------------------------------

// StaleRow is one overdue invoice, scored for collection priority.
type StaleRow struct {
	ID            string  `json:"id"`
	DocNumber     string  `json:"doc_number"`
	Customer      string  `json:"customer"`
	Balance       float64 `json:"balance"`
	DueDate       string  `json:"due_date"`
	DaysPastDue   int     `json:"days_past_due"`
	PriorityScore float64 `json:"priority_score"`
}

// StaleInvoices returns open invoices more than minDays past due, ranked by
// age * balance (largest, oldest debts first).
func StaleInvoices(items []map[string]any, minDays int, now time.Time) []StaleRow {
	out := []StaleRow{}
	for _, it := range items {
		bal := Num(it, "Balance")
		if bal <= 0 {
			continue
		}
		due := Str(it, "DueDate")
		if due == "" {
			due = Str(it, "TxnDate")
		}
		dpd := DaysPastDue(due, now)
		if dpd <= minDays {
			continue
		}
		out = append(out, StaleRow{
			ID:            Str(it, "Id"),
			DocNumber:     Str(it, "DocNumber"),
			Customer:      Nested(it, "CustomerRef", "name"),
			Balance:       bal,
			DueDate:       due,
			DaysPastDue:   dpd,
			PriorityScore: bal * float64(dpd),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].PriorityScore != out[j].PriorityScore {
			return out[i].PriorityScore > out[j].PriorityScore
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// --- Unapplied payments ----------------------------------------------------

// UnappliedRow is one payment with unapplied funds.
type UnappliedRow struct {
	ID           string  `json:"id"`
	TxnDate      string  `json:"txn_date"`
	Customer     string  `json:"customer"`
	TotalAmt     float64 `json:"total_amt"`
	UnappliedAmt float64 `json:"unapplied_amt"`
}

// UnappliedPayments returns payments with UnappliedAmt > 0, largest first.
func UnappliedPayments(items []map[string]any) []UnappliedRow {
	out := []UnappliedRow{}
	for _, it := range items {
		ua := Num(it, "UnappliedAmt")
		if ua <= 0 {
			continue
		}
		out = append(out, UnappliedRow{
			ID:           Str(it, "Id"),
			TxnDate:      Str(it, "TxnDate"),
			Customer:     Nested(it, "CustomerRef", "name"),
			TotalAmt:     Num(it, "TotalAmt"),
			UnappliedAmt: ua,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].UnappliedAmt != out[j].UnappliedAmt {
			return out[i].UnappliedAmt > out[j].UnappliedAmt
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// --- Top ranking -----------------------------------------------------------

// RankedRow is one ranked entity.
type RankedRow struct {
	ID    string  `json:"id"`
	Name  string  `json:"name"`
	Value float64 `json:"value"`
}

// TopByField ranks objects by a scalar field (descending) and returns the top
// limit rows. limit <= 0 returns all.
func TopByField(items []map[string]any, valueField, nameField string, limit int) []RankedRow {
	out := make([]RankedRow, 0, len(items))
	for _, it := range items {
		out = append(out, RankedRow{
			ID:    Str(it, "Id"),
			Name:  Str(it, nameField),
			Value: Num(it, valueField),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Value != out[j].Value {
			return out[i].Value > out[j].Value
		}
		return out[i].Name < out[j].Name
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// SumByRef sums a numeric field grouped by a nested ref (e.g. invoices by
// CustomerRef, bills by VendorRef), optionally filtered to txnDateField >= since.
// Returns rows ranked by total descending.
type SpendRow struct {
	ID    string  `json:"id"`
	Name  string  `json:"name"`
	Total float64 `json:"total"`
	Count int     `json:"count"`
}

func SumByRef(items []map[string]any, amtField, refKey, txnDateField string, since time.Time, hasSince bool) []SpendRow {
	agg := map[string]*SpendRow{}
	for _, it := range items {
		if hasSince {
			if t, ok := ParseDate(Str(it, txnDateField)); ok && t.Before(since) {
				continue
			}
		}
		name := Nested(it, refKey, "name")
		id := Nested(it, refKey, "value")
		if name == "" {
			name = "(unspecified)"
		}
		key := id + "|" + name
		row := agg[key]
		if row == nil {
			row = &SpendRow{ID: id, Name: name}
			agg[key] = row
		}
		row.Total += Num(it, amtField)
		row.Count++
	}
	out := make([]SpendRow, 0, len(agg))
	for _, r := range agg {
		out = append(out, *r)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Total != out[j].Total {
			return out[i].Total > out[j].Total
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// --- Balances snapshot -----------------------------------------------------

// BalancesReport is a cash/AR/AP snapshot spanning accounts + customers + vendors.
type BalancesReport struct {
	AsOf                string             `json:"as_of"`
	BankTotal           float64            `json:"bank_total"`
	AccountsByType      map[string]float64 `json:"accounts_by_type"`
	CustomerReceivables float64            `json:"customer_receivables_total"`
	VendorPayables      float64            `json:"vendor_payables_total"`
	CustomerCount       int                `json:"customer_count"`
	VendorCount         int                `json:"vendor_count"`
	AccountCount        int                `json:"account_count"`
}

// Balances rolls account balances up by AccountType, totals bank balances, and
// sums customer/vendor open balances into one snapshot.
func Balances(accounts, customers, vendors []map[string]any, now time.Time) BalancesReport {
	rep := BalancesReport{
		AsOf:           now.Format("2006-01-02"),
		AccountsByType: map[string]float64{},
	}
	for _, a := range accounts {
		at := Str(a, "AccountType")
		bal := Num(a, "CurrentBalance")
		if at == "" {
			at = "(unclassified)"
		}
		rep.AccountsByType[at] += bal
		if strings.Contains(strings.ToLower(at), "bank") {
			rep.BankTotal += bal
		}
		rep.AccountCount++
	}
	for _, c := range customers {
		rep.CustomerReceivables += Num(c, "Balance")
		rep.CustomerCount++
	}
	for _, v := range vendors {
		rep.VendorPayables += Num(v, "Balance")
		rep.VendorCount++
	}
	return rep
}

// --- Duplicate detection ---------------------------------------------------

// DupeMember is one record in a duplicate group.
type DupeMember struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	Email       string `json:"email"`
}

// DupeGroup is a set of records sharing a normalized name or email.
type DupeGroup struct {
	Key     string       `json:"key"`
	Members []DupeMember `json:"members"`
}

// normalizeName lowercases and strips all non-alphanumeric characters so
// "Acme, Inc." and "ACME Inc" collide.
func normalizeName(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Dupes groups name-list records (customers/vendors) that share a normalized
// DisplayName or a non-empty PrimaryEmailAddr.Address. Only groups with more
// than one member are returned, ordered by group size descending.
func Dupes(items []map[string]any) []DupeGroup {
	type rec struct {
		member DupeMember
		norm   string
		email  string
	}
	recs := make([]rec, 0, len(items))
	for _, it := range items {
		dn := Str(it, "DisplayName")
		email := strings.ToLower(strings.TrimSpace(Nested(it, "PrimaryEmailAddr", "Address")))
		recs = append(recs, rec{
			member: DupeMember{ID: Str(it, "Id"), DisplayName: dn, Email: email},
			norm:   normalizeName(dn),
			email:  email,
		})
	}
	groups := map[string]*DupeGroup{}
	add := func(key string, m DupeMember) {
		g := groups[key]
		if g == nil {
			g = &DupeGroup{Key: key}
			groups[key] = g
		}
		for _, ex := range g.Members {
			if ex.ID == m.ID {
				return
			}
		}
		g.Members = append(g.Members, m)
	}
	// Name collisions.
	byNorm := map[string][]rec{}
	for _, r := range recs {
		if r.norm != "" {
			byNorm[r.norm] = append(byNorm[r.norm], r)
		}
	}
	for k, rs := range byNorm {
		if len(rs) < 2 {
			continue
		}
		for _, r := range rs {
			add("name:"+k, r.member)
		}
	}
	// Email collisions (catches differently-named records sharing an address).
	byEmail := map[string][]rec{}
	for _, r := range recs {
		if r.email != "" {
			byEmail[r.email] = append(byEmail[r.email], r)
		}
	}
	for k, rs := range byEmail {
		if len(rs) < 2 {
			continue
		}
		for _, r := range rs {
			add("email:"+k, r.member)
		}
	}
	out := make([]DupeGroup, 0, len(groups))
	for _, g := range groups {
		if len(g.Members) < 2 {
			continue
		}
		sort.Slice(g.Members, func(i, j int) bool { return g.Members[i].ID < g.Members[j].ID })
		out = append(out, *g)
	}
	sort.Slice(out, func(i, j int) bool {
		if len(out[i].Members) != len(out[j].Members) {
			return len(out[i].Members) > len(out[j].Members)
		}
		return out[i].Key < out[j].Key
	})
	return out
}
