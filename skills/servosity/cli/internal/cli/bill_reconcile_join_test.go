// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// TestBillReconcileJoinsWithoutSyncedCompanies pins that bill --reconcile on a
// store with no companies still joins each bill line to its invoice row: the
// CSV's own names and a live /companies/ read resolve them. Before, every
// client came back twice, as "billed, not invoiced" and "invoiced but no
// Servosity backups".
func TestBillReconcileJoinsWithoutSyncedCompanies(t *testing.T) {
	isolateNovelStore(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/companies/":
			_, _ = w.Write([]byte(`{"next":null,"results":[{"id":4421,"name":"Acme Co","reseller":"https://api.example.test/api/v1/resellers/2/"},{"id":5150,"name":"Initech","reseller":"https://api.example.test/api/v1/resellers/2/"}]}`))
		case "/resellers/2/bill/":
			_, _ = w.Write([]byte(`{"line_items":[{"company":"Acme Co [Simplified]","product":"restic","count":40},{"company":"Globex [Silver]","product":"dr","count":1},{"company":"Initech [Gold]","product":"restic","count":3}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	t.Setenv("SERVOSITY_MSP_BASE_URL", srv.URL)
	t.Setenv("SERVOSITY_MSP_TOKEN", "fixture-token")

	csv := filepath.Join(t.TempDir(), "invoices.csv")
	if err := os.WriteFile(csv, []byte("company_id,company_name,invoiced_amount\n4421,Acme Co,$1.00\n4766,Globex,$500.00\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := RootCmd()
	cmd.SetArgs([]string{"bill", "--reconcile", csv, "--json"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("bill --reconcile: %v\n%s", err, out.String())
	}
	var report struct {
		Rows []reconcileRow `json:"rows"`
		Meta map[string]any `json:"meta"`
	}
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatalf("decode report: %v\n%s", err, out.String())
	}
	got := map[string]reconcileRow{}
	for _, r := range report.Rows {
		got[r.CompanyID] = r
	}
	if len(report.Rows) != 3 {
		t.Fatalf("want 3 rows (Acme, Globex, Initech), got %d: %+v", len(report.Rows), report.Rows)
	}
	if r := got["4421"]; r.ServosityBackups != 40 || r.InvoicedCent != 100 || r.Note != "" {
		t.Fatalf("Acme (CSV and live name) did not join: %+v", r)
	}
	if r := got["4766"]; r.ServosityBackups != 1 || r.InvoicedCent != 50000 || r.Note != "" {
		t.Fatalf("Globex (CSV name only) did not join: %+v", r)
	}
	if r := got["5150"]; r.ServosityBackups != 3 || r.Note != "Servosity-billed, not invoiced (lost revenue)" {
		t.Fatalf("Initech (live name, not invoiced) = %+v", r)
	}
	if _, ok := report.Meta["unresolved_bill_names"]; ok {
		t.Fatalf("no bill name should stay unresolved: %v", report.Meta)
	}
}

// TestJoinAndScoreLeavesADuplicateCSVNameToTheCompanyIndex pins that a name two
// CSV rows share does not pick a row by map order: the synced company index
// decides, the same way on every run.
func TestJoinAndScoreLeavesADuplicateCSVNameToTheCompanyIndex(t *testing.T) {
	invoices := map[string]reconcileRow{
		"4421": {CompanyID: "4421", CompanyName: "Acme Co", InvoicedCent: 100},
		"9001": {CompanyID: "9001", CompanyName: "Acme Co", InvoicedCent: 200},
	}
	for run := 0; run < 20; run++ {
		rows := joinAndScore([]billLineItem{{Company: "Acme Co [Simplified]", Product: "restic", Count: 40}}, invoices, map[string]int64{"acme co": 4421})
		for _, r := range rows {
			if r.CompanyID == "4421" && r.ServosityBackups != 40 {
				t.Fatalf("run %d: the bill line left the indexed company: %+v", run, rows)
			}
			if r.CompanyID == "9001" && r.ServosityBackups != 0 {
				t.Fatalf("run %d: the bill line went to the duplicate CSV row: %+v", run, rows)
			}
		}
	}
}
