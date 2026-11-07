// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"quickbooks-pp-cli/internal/store"
)

// seedPayablesStore seeds bills and invoices for the two reprint novels (bills
// due, invoices unsent). Dates are offsets from today, like seedNovelStore, and
// every boundary is cleared by days, so the result never depends on the clock.
func seedPayablesStore(t *testing.T) string {
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
	acme := map[string]any{"value": "v1", "name": "Acme Supply"}
	seed("bills", "b1", map[string]any{"Id": "b1", "DocNumber": "B-1", "DueDate": seedDate(ref, -5), "Balance": 100.0, "VendorRef": acme})
	seed("bills", "b2", map[string]any{"Id": "b2", "DocNumber": "B-2", "DueDate": seedDate(ref, 3), "Balance": 250.5, "VendorRef": acme})
	seed("bills", "b3", map[string]any{"Id": "b3", "DocNumber": "B-3", "DueDate": seedDate(ref, 4), "Balance": 40.0, "VendorRef": map[string]any{"value": "v2", "name": "Globex"}})
	seed("bills", "b4", map[string]any{"Id": "b4", "DocNumber": "B-4", "DueDate": seedDate(ref, 30), "Balance": 999.0, "VendorRef": acme})
	seed("bills", "b5", map[string]any{"Id": "b5", "DocNumber": "B-5", "DueDate": seedDate(ref, 1), "Balance": 0.0, "VendorRef": acme})
	seed("bills", "b6", map[string]any{"Id": "b6", "DocNumber": "B-6", "Balance": 75.0, "VendorRef": acme})
	cust := map[string]any{"value": "c1", "name": "Initech"}
	seed("invoices", "i1", map[string]any{"Id": "i1", "DocNumber": "INV-1", "Balance": 300.0, "EmailStatus": "NeedToSend", "PrintStatus": "NotSet", "CustomerRef": cust, "BillEmail": map[string]any{"Address": "ap@example.com"}})
	seed("invoices", "i2", map[string]any{"Id": "i2", "DocNumber": "INV-2", "Balance": 800.0, "EmailStatus": "NotSet", "PrintStatus": "NotSet", "CustomerRef": cust})
	seed("invoices", "i3", map[string]any{"Id": "i3", "DocNumber": "INV-3", "Balance": 500.0, "EmailStatus": "EmailSent", "CustomerRef": cust})
	seed("invoices", "i4", map[string]any{"Id": "i4", "DocNumber": "INV-4", "Balance": 450.0, "EmailStatus": "NotSet", "PrintStatus": "PrintComplete", "CustomerRef": cust})
	seed("invoices", "i5", map[string]any{"Id": "i5", "DocNumber": "INV-5", "Balance": 0.0, "EmailStatus": "NotSet", "CustomerRef": cust})
	seed("invoices", "i6", map[string]any{"Id": "i6", "DocNumber": "INV-6", "Balance": 60.0, "CustomerRef": cust})
	return dbPath
}

func TestNovelBillsDueAcceptance(t *testing.T) {
	dbPath := seedPayablesStore(t)
	var rep struct {
		TotalDue     float64 `json:"total_due"`
		OverdueTotal float64 `json:"overdue_total"`
		BillCount    int     `json:"bill_count"`
		ByVendor     []struct {
			VendorID string  `json:"vendor_id"`
			Total    float64 `json:"total"`
			Bills    int     `json:"bills"`
		} `json:"by_vendor"`
		Bills []struct {
			ID           string `json:"id"`
			DaysUntilDue int    `json:"days_until_due"`
			Overdue      bool   `json:"overdue"`
		} `json:"bills"`
		Undated []struct {
			ID string `json:"id"`
		} `json:"undated"`
	}
	out := runNovel(t, dbPath, "bills", "due", "--days", "7")
	if err := json.Unmarshal([]byte(out), &rep); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	// b1 overdue, b2 and b3 inside 7 days; b4 is beyond the window, b5 is paid,
	// b6 has no date and is listed as undated rather than guessed.
	if rep.BillCount != 3 || len(rep.Bills) != 3 || rep.Bills[0].ID != "b1" || rep.Bills[1].ID != "b2" || rep.Bills[2].ID != "b3" {
		t.Fatalf("bills in window wrong: %+v", rep.Bills)
	}
	if !rep.Bills[0].Overdue || rep.Bills[0].DaysUntilDue != -5 || rep.Bills[1].Overdue || rep.Bills[1].DaysUntilDue != 3 {
		t.Fatalf("due-day math wrong: %+v", rep.Bills)
	}
	if rep.TotalDue != 390.5 || rep.OverdueTotal != 100 {
		t.Fatalf("totals wrong: total_due=%v overdue_total=%v", rep.TotalDue, rep.OverdueTotal)
	}
	if len(rep.ByVendor) != 2 || rep.ByVendor[0].VendorID != "v1" || rep.ByVendor[0].Total != 350.5 || rep.ByVendor[0].Bills != 2 {
		t.Fatalf("vendor roll-up wrong: %+v", rep.ByVendor)
	}
	if len(rep.Undated) != 1 || rep.Undated[0].ID != "b6" {
		t.Fatalf("undated bill not surfaced: %+v", rep.Undated)
	}
	// A wider window pulls b4 in.
	out = runNovel(t, dbPath, "bills", "due", "--days", "45")
	if err := json.Unmarshal([]byte(out), &rep); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if rep.BillCount != 4 {
		t.Fatalf("--days 45 should include b4: %+v", rep.Bills)
	}
}

func TestNovelBillsDueRefusesNegativeDays(t *testing.T) {
	dbPath := seedPayablesStore(t)
	root := newRootCmd(&rootFlags{})
	root.SetArgs([]string{"bills", "due", "--days", "-1", "--db", dbPath, "--json"})
	if err := root.Execute(); err == nil {
		t.Fatal("bills due --days -1 should be refused")
	}
}

func TestNovelInvoicesUnsentAcceptance(t *testing.T) {
	dbPath := seedPayablesStore(t)
	var rep struct {
		Count            int     `json:"count"`
		TotalBalance     float64 `json:"total_balance"`
		StatusUnreported int     `json:"status_unreported"`
		Invoices         []struct {
			ID           string `json:"id"`
			HasBillEmail bool   `json:"has_bill_email"`
		} `json:"invoices"`
	}
	out := runNovel(t, dbPath, "invoices", "unsent")
	if err := json.Unmarshal([]byte(out), &rep); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	// i2 (800, NotSet) and i1 (300, NeedToSend) are open and undelivered. i3 was
	// emailed, i4 printed, i5 is paid, i6 reports no EmailStatus at all.
	if rep.Count != 2 || rep.Invoices[0].ID != "i2" || rep.Invoices[1].ID != "i1" {
		t.Fatalf("unsent set wrong: %+v", rep.Invoices)
	}
	if rep.Invoices[0].HasBillEmail || !rep.Invoices[1].HasBillEmail {
		t.Fatalf("has_bill_email wrong: %+v", rep.Invoices)
	}
	if rep.TotalBalance != 1100 || rep.StatusUnreported != 1 {
		t.Fatalf("totals wrong: total=%v unreported=%d", rep.TotalBalance, rep.StatusUnreported)
	}
}
