// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package analytics

import (
	"testing"
	"time"
)

var insightsNow = time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)

func inv(id, custID, custName, txnDate, dueDate string, total, balance float64) map[string]any {
	return map[string]any{
		"Id": id, "TxnDate": txnDate, "DueDate": dueDate,
		"TotalAmt": total, "Balance": balance,
		"CustomerRef": map[string]any{"value": custID, "name": custName},
	}
}

func pay(id, custID, custName, txnDate string, total, unapplied float64, linkedInvoiceIDs ...string) map[string]any {
	var lines []any
	for _, lid := range linkedInvoiceIDs {
		lines = append(lines, map[string]any{
			"LinkedTxn": []any{map[string]any{"TxnId": lid, "TxnType": "Invoice"}},
		})
	}
	return map[string]any{
		"Id": id, "TxnDate": txnDate, "TotalAmt": total, "UnappliedAmt": unapplied,
		"CustomerRef": map[string]any{"value": custID, "name": custName},
		"Line":        lines,
	}
}

func TestCustomerProfitability(t *testing.T) {
	invoices := []map[string]any{
		inv("i1", "c1", "Acme", "2026-05-01", "2026-05-31", 1000, 0),
		inv("i2", "c1", "Acme", "2026-05-10", "2026-06-09", 500, 500),
		inv("i3", "c2", "Globex", "2026-05-15", "2026-06-14", 200, 200),
	}
	payments := []map[string]any{
		pay("p1", "c1", "Acme", "2026-05-21", 1000, 0, "i1"), // paid i1 in 20 days
	}
	customers := []map[string]any{
		{"Id": "c1", "DisplayName": "Acme"},
		{"Id": "c2", "DisplayName": "Globex"},
	}
	rows := CustomerProfitability(invoices, payments, customers, 0)
	if len(rows) != 2 {
		t.Fatalf("want 2 rows, got %d", len(rows))
	}
	acme := rows[0]
	if acme.Name != "Acme" || acme.Invoiced != 1500 || acme.Paid != 1000 || acme.Net != 500 || acme.OpenBalance != 500 {
		t.Fatalf("acme row wrong: %+v", acme)
	}
	if acme.AvgDaysToPay != 20 || acme.PaidLinks != 1 {
		t.Fatalf("acme days-to-pay wrong: %+v", acme)
	}
	globex := rows[1]
	if globex.Paid != 0 || globex.AvgDaysToPay != 0 {
		t.Fatalf("globex row wrong: %+v", globex)
	}
	// limit honored
	if got := CustomerProfitability(invoices, payments, customers, 1); len(got) != 1 {
		t.Fatalf("limit 1: want 1 row, got %d", len(got))
	}
}

func TestDSO(t *testing.T) {
	invoices := []map[string]any{
		inv("i1", "c1", "Acme", "2026-05-20", "2026-06-19", 900, 300), // in window
		inv("i2", "c1", "Acme", "2025-01-01", "2025-01-31", 400, 100), // out of window
	}
	payments := []map[string]any{
		pay("p1", "c1", "Acme", "2026-05-30", 600, 0, "i1"), // 10 days
	}
	rep := DSO(invoices, payments, 90, insightsNow)
	if rep.ARBalance != 400 {
		t.Fatalf("AR balance: want 400, got %v", rep.ARBalance)
	}
	if rep.CreditSales != 900 {
		t.Fatalf("credit sales: want 900 (window only), got %v", rep.CreditSales)
	}
	want := round2(400.0 / 900.0 * 90)
	if rep.DSO != want {
		t.Fatalf("dso: want %v, got %v", want, rep.DSO)
	}
	if rep.AvgDaysToPay != 10 {
		t.Fatalf("avg days to pay: want 10, got %v", rep.AvgDaysToPay)
	}
	if len(rep.SlowPayers) != 1 || rep.SlowPayers[0].Name != "Acme" {
		t.Fatalf("slow payers wrong: %+v", rep.SlowPayers)
	}
	// zero credit sales must not produce NaN/Inf
	empty := DSO(nil, nil, 90, insightsNow)
	if empty.DSO != 0 {
		t.Fatalf("empty dso: want 0, got %v", empty.DSO)
	}
}

func TestCashForecast(t *testing.T) {
	invoices := []map[string]any{
		inv("i1", "c1", "Acme", "2026-06-01", "2026-06-08", 100, 100), // week 1
		inv("i2", "c1", "Acme", "2026-06-01", "2026-05-30", 50, 50),   // overdue
		inv("i3", "c1", "Acme", "2026-06-01", "2026-09-01", 75, 75),   // beyond window
		inv("i4", "c1", "Acme", "2026-06-01", "2026-06-08", 100, 0),   // paid: excluded
	}
	bills := []map[string]any{
		{"Id": "b1", "DueDate": "2026-06-10", "Balance": 40.0, "VendorRef": map[string]any{"value": "v1", "name": "Vend"}},
	}
	rep := CashForecast(invoices, bills, 4, insightsNow)
	if rep.OverdueInflows != 50 {
		t.Fatalf("overdue inflows: want 50, got %v", rep.OverdueInflows)
	}
	if rep.BeyondInflows != 75 {
		t.Fatalf("beyond inflows: want 75, got %v", rep.BeyondInflows)
	}
	if len(rep.Rows) != 4 {
		t.Fatalf("want 4 weekly rows, got %d", len(rep.Rows))
	}
	if rep.Rows[0].Inflows != 100 || rep.Rows[0].Outflows != 40 || rep.Rows[0].Net != 60 {
		t.Fatalf("week 1 wrong: %+v", rep.Rows[0])
	}
	if rep.NetInWindow != 60 {
		t.Fatalf("net in window: want 60, got %v", rep.NetInWindow)
	}
	// exactly N zero-filled rows even with no data (absence-of-correctness)
	zero := CashForecast(nil, nil, 3, insightsNow)
	if len(zero.Rows) != 3 || zero.NetInWindow != 0 {
		t.Fatalf("empty forecast wrong: %+v", zero)
	}
}

func jeLine(posting, account string, amt float64) map[string]any {
	return map[string]any{
		"Amount": amt,
		"JournalEntryLineDetail": map[string]any{
			"PostingType": posting,
			"AccountRef":  map[string]any{"value": "a1", "name": account},
		},
	}
}

func TestCheckJournalEntries(t *testing.T) {
	entries := []map[string]any{
		{"Id": "j1", "DocNumber": "JE-1", "TxnDate": "2026-06-01",
			"Line": []any{jeLine("Debit", "Rent", 100), jeLine("Credit", "Cash", 100)}},
		{"Id": "j2", "DocNumber": "JE-2", "TxnDate": "2026-06-02",
			"Line": []any{jeLine("Debit", "Rent", 100), jeLine("Credit", "Cash", 90)}},
		{"Id": "j3", "DocNumber": "JE-3", "TxnDate": "2026-06-03",
			"Line": []any{jeLine("Debit", "Suspense", 10), jeLine("Credit", "Cash", 10)}},
	}
	rep := CheckJournalEntries(entries)
	if rep.Checked != 3 {
		t.Fatalf("checked: want 3, got %d", rep.Checked)
	}
	if rep.Unbalanced != 1 {
		t.Fatalf("unbalanced: want 1, got %d", rep.Unbalanced)
	}
	if rep.SuspenseHits != 1 {
		t.Fatalf("suspense hits: want 1, got %d", rep.SuspenseHits)
	}
	if len(rep.Findings) != 2 {
		t.Fatalf("findings: want 2 (j2, j3), got %d: %+v", len(rep.Findings), rep.Findings)
	}
	// balanced books return zero findings, not fabricated drift
	clean := CheckJournalEntries(entries[:1])
	if len(clean.Findings) != 0 {
		t.Fatalf("clean GL must return no findings, got %+v", clean.Findings)
	}
}

func TestReconcile(t *testing.T) {
	payments := []map[string]any{pay("p1", "c1", "Acme", "2026-06-01", 100, 25)}
	customers := []map[string]any{
		{"Id": "c1", "DisplayName": "Acme Inc"},
		{"Id": "c9", "DisplayName": "Acme, Inc."},
		{"Id": "c2", "DisplayName": "Globex", "Active": false},
	}
	vendors := []map[string]any{{"Id": "v1", "DisplayName": "Vend"}}
	jes := []map[string]any{{"Id": "j2", "Line": []any{jeLine("Debit", "Rent", 100), jeLine("Credit", "Cash", 90)}}}
	invoices := []map[string]any{inv("i1", "c2", "Globex", "2026-06-01", "2026-07-01", 100, 100)}
	rep := Reconcile(payments, customers, vendors, jes, invoices, nil, nil, insightsNow)
	if rep.Clean {
		t.Fatal("report must not be clean")
	}
	cats := map[string]int{}
	for _, f := range rep.Findings {
		cats[f.Category] = f.Count
	}
	if cats["unapplied-payments"] != 1 {
		t.Fatalf("unapplied: %+v", cats)
	}
	if cats["duplicate-customers"] != 1 {
		t.Fatalf("dupes: %+v", cats)
	}
	if cats["journal-entry-anomalies"] != 1 {
		t.Fatalf("je: %+v", cats)
	}
	if cats["inactive-on-open-transactions"] != 1 {
		t.Fatalf("inactive refs: %+v", cats)
	}
	// clean books → clean report, no fabricated findings
	cleanRep := Reconcile(nil, nil, nil, nil, nil, nil, nil, insightsNow)
	if !cleanRep.Clean || len(cleanRep.Findings) != 0 {
		t.Fatalf("clean books must produce clean report: %+v", cleanRep)
	}
}

func TestAgingDelta(t *testing.T) {
	prevAR := Aging([]map[string]any{
		inv("i1", "c1", "Acme", "2026-04-01", "2026-05-01", 100, 100), // 29d overdue at prev time → 0-30
	}, "DueDate", "TxnDate", "Balance", "CustomerRef", time.Date(2026, 5, 30, 0, 0, 0, 0, time.UTC))
	currAR := Aging([]map[string]any{
		inv("i1", "c1", "Acme", "2026-04-01", "2026-05-01", 100, 100), // now 36d overdue → slipped to 31-60
		inv("i9", "c3", "Initech", "2026-06-01", "2026-07-01", 250, 250),
	}, "DueDate", "TxnDate", "Balance", "CustomerRef", insightsNow)
	prev := AgingSnapshot{TakenAt: "2026-05-30", AR: prevAR, AP: AgingReport{Buckets: map[string]float64{}}}
	curr := AgingSnapshot{TakenAt: "2026-06-06", AR: currAR, AP: AgingReport{Buckets: map[string]float64{}}}
	rep := AgingDelta(prev, curr)
	if rep.ARTotalDelta != 250 {
		t.Fatalf("AR total delta: want 250, got %v", rep.ARTotalDelta)
	}
	var acme, initech *EntityDelta
	for i := range rep.Changes {
		switch rep.Changes[i].Name {
		case "Acme":
			acme = &rep.Changes[i]
		case "Initech":
			initech = &rep.Changes[i]
		}
	}
	if initech == nil || !initech.New || initech.Delta != 250 {
		t.Fatalf("initech delta wrong: %+v", initech)
	}
	if acme == nil || !acme.BucketSlipped || acme.PrevWorstBucket != "0-30" || acme.CurrWorstBucket != "31-60" {
		t.Fatalf("acme slip wrong: %+v", acme)
	}
	if rep.Slipped != 1 {
		t.Fatalf("slipped count: want 1, got %d", rep.Slipped)
	}
	// identical snapshots → no changes (absence-of-correctness)
	same := AgingDelta(curr, curr)
	if len(same.Changes) != 0 {
		t.Fatalf("identical snapshots must yield no changes, got %+v", same.Changes)
	}
}

// TestCashForecastNonUTC locks the day-floor invariant: week boundaries must
// agree with as_of in the caller's timezone, not the UTC-truncated day.
func TestCashForecastNonUTC(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skip("tz database unavailable")
	}
	// 09:00 local = 13:00/14:00 UTC — UTC truncation lands on the same date,
	// so also test 21:00 local where UTC has rolled to the NEXT day.
	for _, hour := range []int{9, 21} {
		now := time.Date(2026, 6, 6, hour, 0, 0, 0, loc)
		rep := CashForecast(nil, nil, 2, now)
		if rep.AsOf != rep.Rows[0].WeekStart {
			t.Fatalf("hour %d: as_of %s != first week_start %s", hour, rep.AsOf, rep.Rows[0].WeekStart)
		}
	}
}
