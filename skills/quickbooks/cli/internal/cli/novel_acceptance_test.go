// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"quickbooks-pp-cli/internal/store"
)

// novelSeedDay is the reference day every seeded fixture date hangs off.
//
// The novel commands read the wall clock (analytics.CashForecast, StaleInvoices,
// Aging and friends all take a `now`, and the Cobra layer hands them
// time.Now()), so absolute fixture dates rot: a due date authored as "future"
// silently becomes overdue once the calendar passes it. Anchoring every fixture
// to today keeps the seeded books in the same relative shape on every run, on
// any date, forever.
//
// The calendar day is the local one (matching what the commands call "today"),
// but it is anchored in UTC so day arithmetic never trips over a daylight-saving
// transition: in zones that skip local midnight on a spring-forward day
// (America/Santiago, Asia/Beirut), AddDate on a local midnight normalizes back
// to 23:00 the previous day and every derived date string would slip by one.
func novelSeedDay() time.Time {
	y, m, d := time.Now().Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// seedDate renders a whole-day offset from the reference day as a QBO date.
// Negative is the past, positive the future.
func seedDate(ref time.Time, offsetDays int) string {
	return ref.AddDate(0, 0, offsetDays).Format("2006-01-02")
}

// Offsets, in days from the reference day. Chosen so every assertion below is
// decided by a wide margin (no fixture sits on a bucket, window or overdue
// boundary), which also keeps them stable across time zones - fixture dates
// parse as UTC midnight while the command floors "today" in local time, a skew
// of at most +/-14h.
const (
	inv1TxnOff  = -60 // INV-1: open, comfortably overdue
	inv1DueOff  = -45
	inv2TxnOff  = -30 // INV-2: open, due inside the 8-week forecast window
	inv2DueOff  = +10
	inv3TxnOff  = -40 // INV-3: fully paid by p1
	inv3DueOff  = -10
	pay1TxnOff  = -30 // p1 pays i3 => realized days-to-pay of exactly 10
	bill1TxnOff = -20 // BILL-1: open, due inside the forecast window
	bill1DueOff = +24
	je1TxnOff   = -30
)

// seedNovelStore builds a temp store with a small, internally consistent set
// of QBO objects so every novel command has data to chew on.
func seedNovelStore(t *testing.T) string {
	t.Helper()
	ref := novelSeedDay()
	dbPath := filepath.Join(t.TempDir(), "data.db")
	db, err := store.OpenWithContext(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer db.Close()
	seed := func(rt, id string, doc map[string]any) {
		raw, _ := json.Marshal(doc)
		if err := db.Upsert(rt, id, raw); err != nil {
			t.Fatalf("seed %s/%s: %v", rt, id, err)
		}
	}
	seed("customers", "c1", map[string]any{"Id": "c1", "DisplayName": "Acme Inc", "Balance": 500.0})
	seed("customers", "c2", map[string]any{"Id": "c2", "DisplayName": "Acme, Inc."})
	seed("customers", "c3", map[string]any{"Id": "c3", "DisplayName": "Globex", "Active": false})
	seed("vendors", "v1", map[string]any{"Id": "v1", "DisplayName": "Vendor One"})
	seed("invoices", "i1", map[string]any{"Id": "i1", "DocNumber": "INV-1", "TxnDate": seedDate(ref, inv1TxnOff), "DueDate": seedDate(ref, inv1DueOff), "TotalAmt": 500.0, "Balance": 500.0,
		"CustomerRef": map[string]any{"value": "c1", "name": "Acme Inc"}})
	seed("invoices", "i2", map[string]any{"Id": "i2", "DocNumber": "INV-2", "TxnDate": seedDate(ref, inv2TxnOff), "DueDate": seedDate(ref, inv2DueOff), "TotalAmt": 300.0, "Balance": 300.0,
		"CustomerRef": map[string]any{"value": "c3", "name": "Globex"}})
	seed("invoices", "i3", map[string]any{"Id": "i3", "DocNumber": "INV-3", "TxnDate": seedDate(ref, inv3TxnOff), "DueDate": seedDate(ref, inv3DueOff), "TotalAmt": 200.0, "Balance": 0.0,
		"CustomerRef": map[string]any{"value": "c1", "name": "Acme Inc"}})
	seed("payments", "p1", map[string]any{"Id": "p1", "TxnDate": seedDate(ref, pay1TxnOff), "TotalAmt": 200.0, "UnappliedAmt": 50.0,
		"CustomerRef": map[string]any{"value": "c1", "name": "Acme Inc"},
		"Line":        []any{map[string]any{"LinkedTxn": []any{map[string]any{"TxnId": "i3", "TxnType": "Invoice"}}}}})
	seed("bills", "b1", map[string]any{"Id": "b1", "DocNumber": "BILL-1", "TxnDate": seedDate(ref, bill1TxnOff), "DueDate": seedDate(ref, bill1DueOff), "TotalAmt": 120.0, "Balance": 120.0,
		"VendorRef": map[string]any{"value": "v1", "name": "Vendor One"}})
	seed("journal-entries", "j1", map[string]any{"Id": "j1", "DocNumber": "JE-1", "TxnDate": seedDate(ref, je1TxnOff),
		"Line": []any{
			map[string]any{"Amount": 100.0, "JournalEntryLineDetail": map[string]any{"PostingType": "Debit", "AccountRef": map[string]any{"value": "a1", "name": "Rent"}}},
			map[string]any{"Amount": 90.0, "JournalEntryLineDetail": map[string]any{"PostingType": "Credit", "AccountRef": map[string]any{"value": "a2", "name": "Cash"}}},
		}})
	return dbPath
}

// runNovel executes one command path through the real root command tree with
// --json output and returns stdout.
func runNovel(t *testing.T, dbPath string, args ...string) string {
	t.Helper()
	flags := &rootFlags{}
	root := newRootCmd(flags)
	var out, errBuf bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errBuf)
	full := append(args, "--db", dbPath, "--json")
	root.SetArgs(full)
	if err := root.Execute(); err != nil {
		t.Fatalf("%v failed: %v (stderr: %s)", args, err, errBuf.String())
	}
	return out.String()
}

func TestNovelCustomerProfitabilityAcceptance(t *testing.T) {
	dbPath := seedNovelStore(t)
	out := runNovel(t, dbPath, "customer-profitability")
	var rows []map[string]any
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if len(rows) == 0 {
		t.Fatal("no profitability rows for seeded data")
	}
	top := rows[0]
	if top["name"] != "Acme Inc" {
		t.Fatalf("top customer wrong: %+v", top)
	}
	// invoiced 700 (i1 500 + i3 200), paid 200, days-to-pay 10 from p1→i3 link
	if fmt.Sprint(top["invoiced"]) != "700" || fmt.Sprint(top["paid"]) != "200" || fmt.Sprint(top["avg_days_to_pay"]) != "10" {
		t.Fatalf("acme numbers wrong: %+v", top)
	}
}

func TestNovelDsoAcceptance(t *testing.T) {
	dbPath := seedNovelStore(t)
	out := runNovel(t, dbPath, "dso", "--days", "365")
	var rep map[string]any
	if err := json.Unmarshal([]byte(out), &rep); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if fmt.Sprint(rep["ar_balance"]) != "800" { // 500 + 300 open
		t.Fatalf("ar_balance wrong: %+v", rep)
	}
	if fmt.Sprint(rep["avg_days_to_pay"]) != "10" {
		t.Fatalf("avg_days_to_pay wrong: %+v", rep)
	}
	if rep["dso_days"] == nil {
		t.Fatalf("dso_days missing: %+v", rep)
	}
}

func TestNovelCashForecastAcceptance(t *testing.T) {
	dbPath := seedNovelStore(t)
	// Bracket the command with two clock reads so the as_of assertion below
	// cannot race a midnight crossing in either direction.
	dayBefore := novelSeedDay().Format("2006-01-02")
	out := runNovel(t, dbPath, "cash-forecast", "--weeks", "8")
	dayAfter := novelSeedDay().Format("2006-01-02")
	var rep map[string]any
	if err := json.Unmarshal([]byte(out), &rep); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	weeks, ok := rep["by_week"].([]any)
	if !ok || len(weeks) != 8 {
		t.Fatalf("by_week must have exactly 8 rows: %+v", rep["by_week"])
	}
	// i1 (due 45 days back, still open) is the only overdue inflow; b1 is not
	// yet due, so nothing is overdue on the outflow side.
	if fmt.Sprint(rep["overdue_inflows"]) != "500" {
		t.Fatalf("overdue_inflows wrong: %+v", rep)
	}
	if fmt.Sprint(rep["overdue_outflows"]) != "0" {
		t.Fatalf("overdue_outflows wrong: %+v", rep)
	}
	// i2 (+10d, 300 in) and b1 (+24d, 120 out) both fall inside the 8-week
	// window, so nothing spills past it and the net is 300 - 120.
	if fmt.Sprint(rep["beyond_window_inflows"]) != "0" || fmt.Sprint(rep["beyond_window_outflows"]) != "0" {
		t.Fatalf("nothing should land beyond the window: %+v", rep)
	}
	if fmt.Sprint(rep["net_in_window"]) != "180" {
		t.Fatalf("net_in_window wrong: %+v", rep)
	}
	// Bucketing: i2 lands in forward week 1, b1 in forward week 3.
	wk := func(i int) map[string]any { return weeks[i].(map[string]any) }
	if fmt.Sprint(wk(1)["inflows"]) != "300" {
		t.Fatalf("i2 must bucket into forward week 1: %+v", wk(1))
	}
	if fmt.Sprint(wk(3)["outflows"]) != "120" {
		t.Fatalf("b1 must bucket into forward week 3: %+v", wk(3))
	}
	// as_of is the local calendar day the command read off the wall clock, and
	// that read happened strictly between dayBefore and dayAfter. Writing the
	// three reads as t0 <= t1 <= t2 (dayBefore, the command's own time.Now(),
	// dayAfter), flooring to whole days is monotonic, so
	// dayBefore <= as_of <= dayAfter always holds - including when local
	// midnight falls before the command runs (dayBefore == as_of - 1, upper
	// bound satisfied) or after it returns (dayAfter == as_of + 1, lower bound
	// satisfied). The window can only ever be one day wide, since the command
	// takes far less than 24h. Comparing the ISO-8601 strings is the same as
	// comparing the days: zero-padded YYYY-MM-DD sorts chronologically.
	asOf := fmt.Sprint(rep["as_of"])
	if asOf < dayBefore || asOf > dayAfter {
		t.Fatalf("as_of %q outside [%s, %s]: %+v", asOf, dayBefore, dayAfter, rep)
	}
}

func TestNovelJournalEntriesCheckAcceptance(t *testing.T) {
	dbPath := seedNovelStore(t)
	out := runNovel(t, dbPath, "journal-entries", "check")
	var rep map[string]any
	if err := json.Unmarshal([]byte(out), &rep); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if fmt.Sprint(rep["unbalanced"]) != "1" {
		t.Fatalf("unbalanced count wrong: %+v", rep)
	}
}

func TestNovelReconcileAcceptance(t *testing.T) {
	dbPath := seedNovelStore(t)
	out := runNovel(t, dbPath, "reconcile")
	var rep map[string]any
	if err := json.Unmarshal([]byte(out), &rep); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if rep["clean"] != false {
		t.Fatalf("seeded books must not be clean: %+v", rep)
	}
	var cats []string
	for _, f := range rep["findings"].([]any) {
		cats = append(cats, fmt.Sprint(f.(map[string]any)["category"]))
	}
	joined := strings.Join(cats, ",")
	for _, want := range []string{"unapplied-payments", "duplicate-customers", "journal-entry-anomalies", "inactive-on-open-transactions"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing category %q in %v", want, cats)
		}
	}
}

func TestNovelAgingDeltaAcceptance(t *testing.T) {
	dbPath := seedNovelStore(t)
	// First run: baseline.
	out := runNovel(t, dbPath, "aging-delta")
	var first map[string]any
	if err := json.Unmarshal([]byte(out), &first); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if first["first_run"] != true {
		t.Fatalf("first run must report first_run=true: %+v", first)
	}
	// Second run: identical books → report with zero changes (no fabricated drift).
	out2 := runNovel(t, dbPath, "aging-delta")
	var second map[string]any
	if err := json.Unmarshal([]byte(out2), &second); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out2)
	}
	if second["first_run"] != false {
		t.Fatalf("second run must not be first_run: %+v", second)
	}
	rep := second["report"].(map[string]any)
	if changes, ok := rep["changes"].([]any); ok && len(changes) != 0 {
		t.Fatalf("identical books must yield no changes, got %+v", changes)
	}
}

// Negative test: novel local commands must reject --data-source live.
func TestNovelRejectsLiveDataSource(t *testing.T) {
	dbPath := seedNovelStore(t)
	flags := &rootFlags{}
	root := newRootCmd(flags)
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"reconcile", "--db", dbPath, "--json", "--data-source", "live"})
	if err := root.Execute(); err == nil {
		t.Fatal("reconcile --data-source live must fail (local-only command)")
	}
}
