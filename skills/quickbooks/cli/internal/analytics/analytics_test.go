// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package analytics

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// decode mirrors store.DecodeJSONObject: json.Number for all numerics, which is
// exactly the shape these helpers must survive (the UseNumber int-trap).
func decode(t *testing.T, s string) map[string]any {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(s))
	dec.UseNumber()
	var m map[string]any
	if err := dec.Decode(&m); err != nil {
		t.Fatalf("decode %q: %v", s, err)
	}
	return m
}

func decodeMany(t *testing.T, ss ...string) []map[string]any {
	out := make([]map[string]any, 0, len(ss))
	for _, s := range ss {
		out = append(out, decode(t, s))
	}
	return out
}

func TestNumIsJSONNumberSafe(t *testing.T) {
	obj := decode(t, `{"Balance":"1234.50","TotalAmt":42,"Zero":0,"Missing":null}`)
	if got := Num(obj, "Balance"); got != 1234.50 {
		t.Errorf("Num Balance (string-encoded) = %v, want 1234.50", got)
	}
	if got := Num(obj, "TotalAmt"); got != 42 {
		t.Errorf("Num TotalAmt (json.Number) = %v, want 42", got)
	}
	if got := Num(obj, "Missing"); got != 0 {
		t.Errorf("Num Missing = %v, want 0", got)
	}
	if got := Num(obj, "Absent"); got != 0 {
		t.Errorf("Num Absent = %v, want 0", got)
	}
}

func TestStrAndNested(t *testing.T) {
	obj := decode(t, `{"DisplayName":"Acme","Id":"12","CustomerRef":{"value":"7","name":"Beta Co"},"PrimaryEmailAddr":{"Address":"a@b.com"}}`)
	if got := Str(obj, "DisplayName"); got != "Acme" {
		t.Errorf("Str DisplayName = %q", got)
	}
	if got := Str(obj, "Id"); got != "12" {
		t.Errorf("Str Id = %q", got)
	}
	if got := Nested(obj, "CustomerRef", "name"); got != "Beta Co" {
		t.Errorf("Nested CustomerRef.name = %q", got)
	}
	if got := Nested(obj, "PrimaryEmailAddr", "Address"); got != "a@b.com" {
		t.Errorf("Nested email = %q", got)
	}
	if got := Nested(obj, "CustomerRef", "missing"); got != "" {
		t.Errorf("Nested missing leaf = %q, want empty", got)
	}
	if got := Nested(obj, "Nope", "x"); got != "" {
		t.Errorf("Nested missing root = %q, want empty", got)
	}
}

func TestDaysPastDueAndBucket(t *testing.T) {
	now := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		due    string
		want   int
		bucket string
	}{
		{"2026-05-30", 0, "0-30"},
		{"2026-06-30", -31, "0-30"}, // not yet due → 0-30
		{"2026-05-01", 29, "0-30"},
		{"2026-04-15", 45, "31-60"},
		{"2026-03-01", 90, "61-90"},
		{"2026-01-01", 149, "90+"},
		{"", 0, "0-30"},
	}
	for _, c := range cases {
		if got := DaysPastDue(c.due, now); got != c.want {
			t.Errorf("DaysPastDue(%q) = %d, want %d", c.due, got, c.want)
		}
		if got := bucketFor(DaysPastDue(c.due, now)); got != c.bucket {
			t.Errorf("bucket(%q) = %q, want %q", c.due, got, c.bucket)
		}
	}
}

func TestAgingRollup(t *testing.T) {
	now := time.Date(2026, 5, 30, 0, 0, 0, 0, time.UTC)
	invoices := decodeMany(t,
		`{"Id":"1","Balance":"100","DueDate":"2026-05-20","CustomerRef":{"value":"A","name":"Acme"}}`,
		`{"Id":"2","Balance":"50","DueDate":"2026-02-01","CustomerRef":{"value":"A","name":"Acme"}}`,
		`{"Id":"3","Balance":"0","DueDate":"2026-01-01","CustomerRef":{"value":"B","name":"Beta"}}`, // skipped: paid
		`{"Id":"4","Balance":"200","DueDate":"2026-05-25","CustomerRef":{"value":"B","name":"Beta"}}`,
	)
	rep := Aging(invoices, "DueDate", "TxnDate", "Balance", "CustomerRef", now)
	if rep.Count != 3 {
		t.Errorf("open_count = %d, want 3 (zero-balance excluded)", rep.Count)
	}
	if rep.Total != 350 {
		t.Errorf("total = %v, want 350", rep.Total)
	}
	if rep.Buckets["0-30"] != 300 { // 100 (10d) + 200 (5d)
		t.Errorf("bucket 0-30 = %v, want 300", rep.Buckets["0-30"])
	}
	if rep.Buckets["90+"] != 50 { // invoice 2 is ~118 days past due
		t.Errorf("bucket 90+ = %v, want 50", rep.Buckets["90+"])
	}
	if len(rep.Entities) != 2 {
		t.Fatalf("by_entity len = %d, want 2", len(rep.Entities))
	}
	if rep.Entities[0].Name != "Beta" || rep.Entities[0].Total != 200 {
		t.Errorf("top entity = %+v, want Beta/200", rep.Entities[0])
	}
}

func TestStaleInvoices(t *testing.T) {
	now := time.Date(2026, 5, 30, 0, 0, 0, 0, time.UTC)
	invoices := decodeMany(t,
		`{"Id":"1","DocNumber":"INV-1","Balance":"100","DueDate":"2026-05-25","CustomerRef":{"name":"Acme"}}`,  // 5d, not stale
		`{"Id":"2","DocNumber":"INV-2","Balance":"1000","DueDate":"2026-03-01","CustomerRef":{"name":"Beta"}}`, // ~90d
		`{"Id":"3","DocNumber":"INV-3","Balance":"500","DueDate":"2026-01-01","CustomerRef":{"name":"Gamma"}}`, // ~149d
	)
	rows := StaleInvoices(invoices, 30, now)
	if len(rows) != 2 {
		t.Fatalf("stale count = %d, want 2", len(rows))
	}
	// Gamma: 500*149=74500 vs Beta: 1000*90=90000 → Beta first.
	if rows[0].DocNumber != "INV-2" {
		t.Errorf("highest priority = %q, want INV-2", rows[0].DocNumber)
	}
	if rows[0].PriorityScore <= rows[1].PriorityScore {
		t.Errorf("priority not sorted desc: %v <= %v", rows[0].PriorityScore, rows[1].PriorityScore)
	}
}

func TestUnappliedPayments(t *testing.T) {
	payments := decodeMany(t,
		`{"Id":"1","TxnDate":"2026-05-01","TotalAmt":"500","UnappliedAmt":"0","CustomerRef":{"name":"Acme"}}`,
		`{"Id":"2","TxnDate":"2026-05-02","TotalAmt":"500","UnappliedAmt":"120.5","CustomerRef":{"name":"Beta"}}`,
	)
	rows := UnappliedPayments(payments)
	if len(rows) != 1 || rows[0].ID != "2" || rows[0].UnappliedAmt != 120.5 {
		t.Fatalf("unapplied = %+v, want one row id=2 unapplied=120.5", rows)
	}
}

func TestTopByField(t *testing.T) {
	customers := decodeMany(t,
		`{"Id":"1","DisplayName":"Acme","Balance":"100"}`,
		`{"Id":"2","DisplayName":"Beta","Balance":"900"}`,
		`{"Id":"3","DisplayName":"Gamma","Balance":"500"}`,
	)
	top := TopByField(customers, "Balance", "DisplayName", 2)
	if len(top) != 2 {
		t.Fatalf("limit not applied: %d", len(top))
	}
	if top[0].Name != "Beta" || top[1].Name != "Gamma" {
		t.Errorf("ranking = %+v, want Beta then Gamma", top)
	}
}

func TestSumByRef(t *testing.T) {
	bills := decodeMany(t,
		`{"Id":"1","TxnDate":"2026-01-15","TotalAmt":"100","VendorRef":{"value":"V","name":"VendCo"}}`,
		`{"Id":"2","TxnDate":"2026-02-20","TotalAmt":"300","VendorRef":{"value":"V","name":"VendCo"}}`,
		`{"Id":"3","TxnDate":"2025-12-01","TotalAmt":"999","VendorRef":{"value":"W","name":"Other"}}`, // before since
	)
	since := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	rows := SumByRef(bills, "TotalAmt", "VendorRef", "TxnDate", since, true)
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1 (pre-since excluded)", len(rows))
	}
	if rows[0].Name != "VendCo" || rows[0].Total != 400 || rows[0].Count != 2 {
		t.Errorf("spend = %+v, want VendCo/400/2", rows[0])
	}
}

func TestBalances(t *testing.T) {
	now := time.Date(2026, 5, 30, 0, 0, 0, 0, time.UTC)
	accounts := decodeMany(t,
		`{"Id":"1","AccountType":"Bank","CurrentBalance":"5000"}`,
		`{"Id":"2","AccountType":"Accounts Receivable","CurrentBalance":"1200"}`,
	)
	customers := decodeMany(t, `{"Id":"1","Balance":"1200"}`, `{"Id":"2","Balance":"300"}`)
	vendors := decodeMany(t, `{"Id":"1","Balance":"700"}`)
	rep := Balances(accounts, customers, vendors, now)
	if rep.BankTotal != 5000 {
		t.Errorf("bank = %v, want 5000", rep.BankTotal)
	}
	if rep.CustomerReceivables != 1500 {
		t.Errorf("AR = %v, want 1500", rep.CustomerReceivables)
	}
	if rep.VendorPayables != 700 {
		t.Errorf("AP = %v, want 700", rep.VendorPayables)
	}
	if rep.AccountsByType["Bank"] != 5000 {
		t.Errorf("by_type Bank = %v", rep.AccountsByType["Bank"])
	}
}

func TestDupes(t *testing.T) {
	customers := decodeMany(t,
		`{"Id":"1","DisplayName":"Acme, Inc."}`,
		`{"Id":"2","DisplayName":"ACME Inc"}`,
		`{"Id":"3","DisplayName":"Beta","PrimaryEmailAddr":{"Address":"x@y.com"}}`,
		`{"Id":"4","DisplayName":"Beta LLC","PrimaryEmailAddr":{"Address":"X@Y.com"}}`,
		`{"Id":"5","DisplayName":"Unique Co"}`,
	)
	groups := Dupes(customers)
	if len(groups) != 2 {
		t.Fatalf("dupe groups = %d, want 2", len(groups))
	}
	// Name dupe (Acme) and email dupe (Beta) both surface; Unique Co excluded.
	foundName, foundEmail := false, false
	for _, g := range groups {
		if g.Key == "name:acmeinc" && len(g.Members) == 2 {
			foundName = true
		}
		if g.Key == "email:x@y.com" && len(g.Members) == 2 {
			foundEmail = true
		}
	}
	if !foundName || !foundEmail {
		t.Errorf("expected name+email dupe groups, got %+v", groups)
	}
}
