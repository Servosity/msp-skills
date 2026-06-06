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

	"quickbooks-pp-cli/internal/store"
)

// seedNovelStore builds a temp store with a small, internally consistent set
// of QBO objects so every novel command has data to chew on.
func seedNovelStore(t *testing.T) string {
	t.Helper()
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
	seed("invoices", "i1", map[string]any{"Id": "i1", "DocNumber": "INV-1", "TxnDate": "2026-04-01", "DueDate": "2026-04-30", "TotalAmt": 500.0, "Balance": 500.0,
		"CustomerRef": map[string]any{"value": "c1", "name": "Acme Inc"}})
	seed("invoices", "i2", map[string]any{"Id": "i2", "DocNumber": "INV-2", "TxnDate": "2026-05-01", "DueDate": "2026-07-15", "TotalAmt": 300.0, "Balance": 300.0,
		"CustomerRef": map[string]any{"value": "c3", "name": "Globex"}})
	seed("invoices", "i3", map[string]any{"Id": "i3", "DocNumber": "INV-3", "TxnDate": "2026-05-01", "DueDate": "2026-05-31", "TotalAmt": 200.0, "Balance": 0.0,
		"CustomerRef": map[string]any{"value": "c1", "name": "Acme Inc"}})
	seed("payments", "p1", map[string]any{"Id": "p1", "TxnDate": "2026-05-11", "TotalAmt": 200.0, "UnappliedAmt": 50.0,
		"CustomerRef": map[string]any{"value": "c1", "name": "Acme Inc"},
		"Line":        []any{map[string]any{"LinkedTxn": []any{map[string]any{"TxnId": "i3", "TxnType": "Invoice"}}}}})
	seed("bills", "b1", map[string]any{"Id": "b1", "DocNumber": "BILL-1", "TxnDate": "2026-05-20", "DueDate": "2026-06-20", "TotalAmt": 120.0, "Balance": 120.0,
		"VendorRef": map[string]any{"value": "v1", "name": "Vendor One"}})
	seed("journal-entries", "j1", map[string]any{"Id": "j1", "DocNumber": "JE-1", "TxnDate": "2026-05-31",
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
	out := runNovel(t, dbPath, "cash-forecast", "--weeks", "8")
	var rep map[string]any
	if err := json.Unmarshal([]byte(out), &rep); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	weeks, ok := rep["by_week"].([]any)
	if !ok || len(weeks) != 8 {
		t.Fatalf("by_week must have exactly 8 rows: %+v", rep["by_week"])
	}
	if fmt.Sprint(rep["overdue_inflows"]) != "500" { // i1 overdue
		t.Fatalf("overdue_inflows wrong: %+v", rep)
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
