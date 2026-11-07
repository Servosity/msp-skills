// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

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

// --- Calendar days ---------------------------------------------------------
//
// QBO's TxnDate and DueDate are calendar dates (date-only fields in Intuit's
// Invoice, Bill and Payment references), not instants. Every date computation
// in this package goes through the three helpers below: a written date and
// "today" (now's date in now's zone) both become UTC midnights of their
// calendar dates, and days between them are whole calendar days. A run at
// 23:00 in New York or 08:00 in Sydney therefore ages, buckets, windows and
// forecasts exactly as a run at noon does, and no daylight-saving transition
// can shift a count.

// dateOf is the calendar date written in s, as a UTC midnight. A datetime
// keeps the date it was written on, in its own offset.
func dateOf(s string) (time.Time, bool) {
	t, ok := ParseDate(s)
	if !ok {
		return time.Time{}, false
	}
	return civilDay(t), true
}

// todayOf is now's calendar date in now's own zone, as a UTC midnight.
func todayOf(now time.Time) time.Time {
	return civilDay(now)
}

func civilDay(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// daysFrom is the whole calendar days from a to b (both from dateOf/todayOf).
func daysFrom(a, b time.Time) int {
	return int(b.Sub(a) / (24 * time.Hour))
}

// DaysPastDue returns whole calendar days from dueDate to today in now's zone
// (positive = overdue). A not-yet-due item returns a negative number; an
// unparseable date returns 0.
func DaysPastDue(dueDate string, now time.Time) int {
	d, ok := dateOf(dueDate)
	if !ok {
		return 0
	}
	return daysFrom(d, todayOf(now))
}

// --- Money -----------------------------------------------------------------

// Money is one transaction amount, in the company's home currency.
type Money struct {
	// Written is the amount as the record wrote it, in its own currency.
	// Whether a record is open (a debt, an unapplied credit) is decided on
	// it, never on Home: a foreign remainder can round to 0.00 at home.
	Written float64
	// Home is the amount in home currency: what every total sums.
	Home float64
	// Currency is the transaction's CurrencyRef, set only when the
	// transaction is not in home currency.
	Currency string
	// Foreign is the amount as written, in Currency (0 when Currency is "").
	Foreign float64
}

// HomeAmount reads a money field of a QBO transaction (Invoice, Bill,
// Payment) in home currency. Every money aggregate in this package reads
// transaction amounts through it, so no total ever sums two currencies.
// Intuit's Invoice, Bill and Payment references: with multicurrency on, every
// amount on a transaction is in its CurrencyRef currency, ExchangeRate is the
// number of home-currency units per unit of that currency, and HomeBalance /
// HomeTotalAmt carry QuickBooks' own conversion. QuickBooks' Home<field> wins;
// else the amount times ExchangeRate; with no ExchangeRate (multicurrency
// off) the amount is home currency as written.
func HomeAmount(obj map[string]any, field string) Money {
	amt := Num(obj, field)
	rate, hasRate := numberOK(obj["ExchangeRate"])
	hasRate = hasRate && rate > 0
	home, hasHome := numberOK(obj["Home"+field])
	switch {
	case hasHome:
	case hasRate:
		home = round2(amt * rate)
	default:
		return Money{Written: amt, Home: amt}
	}
	if (hasRate && rate != 1) || home != amt {
		currency := strings.TrimSpace(Nested(obj, "CurrencyRef", "value"))
		if currency == "" {
			currency = "(unspecified)"
		}
		return Money{Written: amt, Home: home, Currency: currency, Foreign: amt}
	}
	return Money{Written: amt, Home: home}
}

// Open reports whether the record still carries an amount (an open balance,
// an unapplied credit), judged on the amount as written.
func (m Money) Open() bool { return m.Written > 0 }

// numberOK is toFloat that also reports whether v held a number at all.
func numberOK(v any) (float64, bool) {
	switch n := v.(type) {
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	case float64:
		return n, true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(n), 64)
		return f, err == nil
	default:
		return 0, false
	}
}

// recordCurrency is a name-list record's CurrencyRef ("" when it names none).
// Name-list balances (Customer.Balance, Vendor.Balance, Account.CurrentBalance)
// are in that currency and carry no exchange rate (Intuit Customer, Vendor and
// Account references), so they are only ever summed within one currency.
func recordCurrency(obj map[string]any) string {
	return strings.TrimSpace(Nested(obj, "CurrencyRef", "value"))
}

// unspecifiedCurrency labels a record that names no currency once the records
// beside it span several.
const unspecifiedCurrency = "(unspecified)"

// spansCurrencies reports whether records name more than one currency.
func spansCurrencies(items []map[string]any) bool {
	first := ""
	for _, it := range items {
		c := recordCurrency(it)
		if c == "" {
			continue
		}
		if first == "" {
			first = c
		} else if c != first {
			return true
		}
	}
	return false
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
		m := HomeAmount(it, amtField)
		if !m.Open() {
			continue
		}
		bal := m.Home
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
	ID             string  `json:"id"`
	DocNumber      string  `json:"doc_number"`
	Customer       string  `json:"customer"`
	Balance        float64 `json:"balance"`
	DueDate        string  `json:"due_date"`
	DaysPastDue    int     `json:"days_past_due"`
	PriorityScore  float64 `json:"priority_score"`
	Currency       string  `json:"currency,omitempty"`
	ForeignBalance float64 `json:"foreign_balance,omitempty"`
}

// StaleInvoices returns open invoices more than minDays past due, ranked by
// age * balance (largest, oldest debts first).
func StaleInvoices(items []map[string]any, minDays int, now time.Time) []StaleRow {
	out := []StaleRow{}
	for _, it := range items {
		m := HomeAmount(it, "Balance")
		if !m.Open() {
			continue
		}
		bal := m.Home
		due := Str(it, "DueDate")
		if due == "" {
			due = Str(it, "TxnDate")
		}
		dpd := DaysPastDue(due, now)
		if dpd <= minDays {
			continue
		}
		out = append(out, StaleRow{
			ID:             Str(it, "Id"),
			DocNumber:      Str(it, "DocNumber"),
			Customer:       Nested(it, "CustomerRef", "name"),
			Balance:        bal,
			DueDate:        due,
			DaysPastDue:    dpd,
			PriorityScore:  bal * float64(dpd),
			Currency:       m.Currency,
			ForeignBalance: m.Foreign,
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
	ID                  string  `json:"id"`
	TxnDate             string  `json:"txn_date"`
	Customer            string  `json:"customer"`
	TotalAmt            float64 `json:"total_amt"`
	UnappliedAmt        float64 `json:"unapplied_amt"`
	Currency            string  `json:"currency,omitempty"`
	ForeignUnappliedAmt float64 `json:"foreign_unapplied_amt,omitempty"`
}

// UnappliedPayments returns payments with UnappliedAmt > 0, largest first.
func UnappliedPayments(items []map[string]any) []UnappliedRow {
	out := []UnappliedRow{}
	for _, it := range items {
		m := HomeAmount(it, "UnappliedAmt")
		if !m.Open() {
			continue
		}
		ua := m.Home
		out = append(out, UnappliedRow{
			ID:                  Str(it, "Id"),
			TxnDate:             Str(it, "TxnDate"),
			Customer:            Nested(it, "CustomerRef", "name"),
			TotalAmt:            HomeAmount(it, "TotalAmt").Home,
			UnappliedAmt:        ua,
			Currency:            m.Currency,
			ForeignUnappliedAmt: m.Foreign,
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
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Value    float64 `json:"value"`
	Currency string  `json:"currency,omitempty"`
}

// TopByField ranks name-list records (customers, vendors) by a balance field
// (descending) and returns the top limit rows. limit <= 0 returns all. The
// field is in each record's own currency (recordCurrency), so when the records
// span currencies they are ranked within each currency, every row says which,
// and limit applies per currency: no row is ever ranked against an amount in
// another currency.
func TopByField(items []map[string]any, valueField, nameField string, limit int) []RankedRow {
	mixed := spansCurrencies(items)
	out := make([]RankedRow, 0, len(items))
	for _, it := range items {
		row := RankedRow{
			ID:    Str(it, "Id"),
			Name:  Str(it, nameField),
			Value: Num(it, valueField),
		}
		if mixed {
			row.Currency = recordCurrency(it)
			if row.Currency == "" {
				row.Currency = unspecifiedCurrency
			}
		}
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Currency != out[j].Currency {
			return out[i].Currency < out[j].Currency
		}
		if out[i].Value != out[j].Value {
			return out[i].Value > out[j].Value
		}
		return out[i].Name < out[j].Name
	})
	if limit > 0 {
		kept := out[:0]
		perCurrency := map[string]int{}
		for _, r := range out {
			if perCurrency[r.Currency] < limit {
				perCurrency[r.Currency]++
				kept = append(kept, r)
			}
		}
		out = kept
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
	sinceDay := civilDay(since)
	for _, it := range items {
		if hasSince {
			if d, ok := dateOf(Str(it, txnDateField)); ok && d.Before(sinceDay) {
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
		row.Total += HomeAmount(it, amtField).Home
		row.Count++
	}
	out := make([]SpendRow, 0, len(agg))
	for _, r := range agg {
		r.Total = round2(r.Total)
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
	AsOf string `json:"as_of"`
	// The four money figures below are null (accounts_by_type empty) when the
	// records span more than one currency; ByCurrency then holds them per
	// currency, and no figure sums two currencies.
	BankTotal           *float64           `json:"bank_total"`
	AccountsByType      map[string]float64 `json:"accounts_by_type"`
	CustomerReceivables *float64           `json:"customer_receivables_total"`
	VendorPayables      *float64           `json:"vendor_payables_total"`
	CustomerCount       int                `json:"customer_count"`
	VendorCount         int                `json:"vendor_count"`
	AccountCount        int                `json:"account_count"`
	// ByCurrency is set only when the records span more than one currency.
	ByCurrency map[string]*CurrencyBalances `json:"by_currency,omitempty"`
}

// CurrencyBalances is the balances snapshot of the records in one currency.
type CurrencyBalances struct {
	BankTotal           float64            `json:"bank_total"`
	AccountsByType      map[string]float64 `json:"accounts_by_type"`
	CustomerReceivables float64            `json:"customer_receivables_total"`
	VendorPayables      float64            `json:"vendor_payables_total"`
}

// Balances rolls account balances up by AccountType, totals bank balances, and
// sums customer/vendor open balances into one snapshot. These are name-list
// balances in each record's own currency (recordCurrency): when the records
// span currencies, every figure is given per currency instead.
func Balances(accounts, customers, vendors []map[string]any, now time.Time) BalancesReport {
	rep := BalancesReport{
		AsOf:           now.Format("2006-01-02"),
		AccountsByType: map[string]float64{},
	}
	all := make([]map[string]any, 0, len(accounts)+len(customers)+len(vendors))
	all = append(append(append(all, accounts...), customers...), vendors...)
	mixed := spansCurrencies(all)
	by := map[string]*CurrencyBalances{}
	bucket := func(obj map[string]any) *CurrencyBalances {
		key := ""
		if mixed {
			key = recordCurrency(obj)
			if key == "" {
				key = unspecifiedCurrency
			}
		}
		b := by[key]
		if b == nil {
			b = &CurrencyBalances{AccountsByType: map[string]float64{}}
			by[key] = b
		}
		return b
	}
	for _, a := range accounts {
		at := Str(a, "AccountType")
		bal := Num(a, "CurrentBalance")
		if at == "" {
			at = "(unclassified)"
		}
		b := bucket(a)
		b.AccountsByType[at] += bal
		if strings.Contains(strings.ToLower(at), "bank") {
			b.BankTotal += bal
		}
		rep.AccountCount++
	}
	for _, c := range customers {
		bucket(c).CustomerReceivables += Num(c, "Balance")
		rep.CustomerCount++
	}
	for _, v := range vendors {
		bucket(v).VendorPayables += Num(v, "Balance")
		rep.VendorCount++
	}
	if mixed {
		rep.ByCurrency = by
		return rep
	}
	one := by[""]
	if one == nil {
		one = &CurrencyBalances{AccountsByType: map[string]float64{}}
	}
	rep.BankTotal = &one.BankTotal
	rep.AccountsByType = one.AccountsByType
	rep.CustomerReceivables = &one.CustomerReceivables
	rep.VendorPayables = &one.VendorPayables
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
