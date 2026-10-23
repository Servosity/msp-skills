// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"servosity-msp-pp-cli/internal/client"
	"servosity-msp-pp-cli/internal/cliutil"
	"servosity-msp-pp-cli/internal/config"
	"servosity-msp-pp-cli/internal/snapshot"
	"servosity-msp-pp-cli/internal/store"
)

func TestReadInvoicesCSVSumsRepeatedRowsAndRejectsShortRows(t *testing.T) {
	dir := t.TempDir()
	split := filepath.Join(dir, "split.csv")
	if err := os.WriteFile(split, []byte("company_id,company_name,invoiced_amount\n1,Acme,100.00\n1,Acme,50.00\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	inv, err := readInvoicesCSV(split)
	if err != nil {
		t.Fatal(err)
	}
	if got := inv["1"].InvoicedCent; got != 15000 {
		t.Fatalf("two invoice lines for one company = %d cents, want 15000", got)
	}
	short := filepath.Join(dir, "short.csv")
	if err := os.WriteFile(short, []byte("company_id,company_name,invoiced_amount\n1,Acme\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readInvoicesCSV(short); err == nil || !strings.Contains(err.Error(), "columns") {
		t.Fatalf("short row err = %v, want a column-count error", err)
	}
}

func TestReconcileLeavesUnpricedCostsNull(t *testing.T) {
	rows := joinAndScore([]billLineItem{{Company: "Acme [Silver]", Product: "restic", Count: 2}},
		map[string]reconcileRow{"1": {CompanyID: "1", CompanyName: "Acme", InvoicedCent: 20000}},
		map[string]int64{"acme": 1})
	if len(rows) != 1 || rows[0].ServosityCent != nil || rows[0].DeltaCent != nil {
		t.Fatalf("rows = %+v; an unpriced bill must leave servosity_cents and delta_cents null", rows)
	}
	report := buildReport("2026-10", rows)
	if _, ok := report.Totals["delta"]; ok {
		t.Fatalf("totals = %v; no dollar delta without a priced bill", report.Totals)
	}
}

// TestHydrateKeepsEvidenceOnFailedLookups: a failed lookup must not erase a
// known success, a not-applicable answer must not read as "never", and an
// explicit {"date":null} still records "never succeeded".
func TestHydrateKeepsEvidenceOnFailedLookups(t *testing.T) {
	isolateNovelStore(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/restic-backups/20/latest-success/":
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"detail":"forbidden"}`))
		case "/restic-backups/21/latest-success/":
			_, _ = w.Write([]byte(`{"date":null}`))
		case "/dr-backups/30/latest-success/":
			_, _ = w.Write([]byte(`{"detail":"read the paired DR backup"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	db, err := store.Open(defaultDBPath("servosity-cli"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	for _, seed := range []error{
		db.UpsertResticBackups(json.RawMessage(`{"id":20,"company":"/companies/1/","device_name":"A"}`)),
		db.UpsertResticBackups(json.RawMessage(`{"id":21,"company":"/companies/1/","device_name":"B"}`)),
		db.Upsert("dr-backups", "30", json.RawMessage(`{"id":30,"company":"/companies/1/","device_name":"C"}`)),
		ensureLastSuccessTable(ctx, db),
	} {
		if seed != nil {
			t.Fatal(seed)
		}
	}
	for _, row := range [][]any{{"restic", "20", "2026-10-22T04:00:00Z"}, {"dr", "30", ""}} {
		if _, err := db.DB().ExecContext(ctx, `INSERT INTO pp_last_success (engine, backup_id, last_success, checked_at) VALUES (?, ?, ?, 'x')`, row...); err != nil {
			t.Fatal(err)
		}
	}

	c := client.New(&config.Config{BaseURL: srv.URL, ServosityMspToken: "fixture"}, 5*time.Second, 0)
	c.NoCache = true
	if _, _, err := hydrateLastSuccess(ctx, c, db); err != nil {
		t.Fatalf("hydrate: %v", err)
	}
	got := map[string]string{}
	rows, err := db.DB().QueryContext(ctx, `SELECT engine || '/' || backup_id, COALESCE(last_success, '') FROM pp_last_success`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			t.Fatal(err)
		}
		got[k] = v
	}
	if got["restic/20"] != "2026-10-22T04:00:00Z" {
		t.Errorf("failed lookup overwrote the known success: %v", got)
	}
	if v, ok := got["restic/21"]; !ok || v != "" {
		t.Errorf(`{"date":null} must record never-succeeded: %v`, got)
	}
	if _, ok := got["dr/30"]; ok {
		t.Errorf("a not-applicable answer must drop the row, not record never-succeeded: %v", got)
	}
}

func TestTriageRefusesMCPMutationWithoutConfirm(t *testing.T) {
	isolateNovelStore(t)
	writes := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writes++
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	t.Setenv("SERVOSITY_MSP_BASE_URL", srv.URL)
	t.Setenv("SERVOSITY_MSP_TOKEN", "fixture-token")
	t.Setenv(cliutil.MCPChildEnvVar, "1")

	const refusal = "re-run with confirm set to true"
	// No confirm, and a non-boolean MCP value (--confirm=1, which pflag reads
	// as true): both must refuse before any write reaches the API.
	for _, args := range [][]string{
		{"triage", "--ignore", "18"},
		{"triage", "--ignore", "18", "--confirm=1"},
	} {
		cmd := RootCmd()
		cmd.SetArgs(args)
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), refusal) {
			t.Fatalf("%v from MCP: err = %v, want the confirm refusal", args, err)
		}
	}
	if writes != 0 {
		t.Fatalf("%d write(s) reached the API without a boolean confirm", writes)
	}
	if !bareFlagPresent([]string{"triage", "--confirm"}, "confirm") || bareFlagPresent([]string{"--confirm=1"}, "confirm") || bareFlagPresent([]string{"--confirm=true"}, "confirm") {
		t.Fatal("bareFlagPresent must accept only the bare --confirm token")
	}
	preview := RootCmd()
	preview.SetArgs([]string{"triage", "--ignore", "18", "--dry-run"})
	var out bytes.Buffer
	preview.SetOut(&out)
	preview.SetErr(&out)
	if err := preview.Execute(); err != nil || writes != 0 {
		t.Fatalf("dry-run preview from MCP: err = %v, writes = %d", err, writes)
	}
}

// TestAttentionSnapshotKeepsEveryCompany: --top trims the output only. A
// truncated snapshot made the company below the cut look recovered to drift.
func TestAttentionSnapshotKeepsEveryCompany(t *testing.T) {
	isolateNovelStore(t)
	db, err := store.Open(defaultDBPath("servosity-cli"))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	old := time.Now().UTC().Add(-30 * 24 * time.Hour).Format(time.RFC3339)
	for _, seed := range []error{
		db.UpsertCompanies(json.RawMessage(`{"id":1,"name":"A"}`)),
		db.UpsertCompanies(json.RawMessage(`{"id":2,"name":"B"}`)),
		db.UpsertResticBackups(json.RawMessage(`{"id":11,"company":"/companies/1/","device_name":"A1"}`)),
		db.UpsertResticBackups(json.RawMessage(`{"id":21,"company":"/companies/2/","device_name":"B1"}`)),
		ensureLastSuccessTable(ctx, db),
	} {
		if seed != nil {
			t.Fatal(seed)
		}
	}
	for _, row := range [][]any{{"11", 1}, {"21", 2}} {
		if _, err := db.DB().ExecContext(ctx, `INSERT INTO pp_last_success (engine, backup_id, company_id, last_success, checked_at) VALUES ('restic', ?, ?, ?, 'x')`, row[0], row[1], old); err != nil {
			t.Fatal(err)
		}
	}
	_ = db.Close()

	var shown attentionResult
	if err := json.Unmarshal([]byte(runNovel(t, "attention", "--top", "1", "--json")), &shown); err != nil {
		t.Fatal(err)
	}
	if len(shown.Companies) != 1 {
		t.Fatalf("--top 1 showed %d companies", len(shown.Companies))
	}
	db, err = store.Open(defaultDBPath("servosity-cli"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	snap, err := snapshot.Latest(ctx, db.DB(), "attention")
	if err != nil || snap == nil {
		t.Fatalf("latest attention snapshot: %v", err)
	}
	var saved attentionResult
	if err := json.Unmarshal(snap.Data, &saved); err != nil {
		t.Fatal(err)
	}
	if len(saved.Companies) != 2 {
		t.Fatalf("snapshot kept %d companies, want both", len(saved.Companies))
	}
}

func TestFailureClustersCountHostsPerCompanyAndFollowPages(t *testing.T) {
	got := clusterFailures([]failureEvent{
		{engine: "restic", host: "SERVER", text: "disk full", company: 1},
		{engine: "restic", host: "SERVER", text: "disk full", company: 2},
	})
	if len(got) != 1 || got[0].Hosts != 2 || got[0].Clients != 2 {
		t.Fatalf("clusters = %+v; two clients' SERVER hosts are two hosts", got)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("page") == "2" {
			_, _ = w.Write([]byte(`{"next":null,"results":[{"errors":"b"}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"next":"https://api.servosity.com/api/v1/companies/1/restic-backup-failures/?month=2026-10-01&page=2","results":[{"errors":"a"}]}`))
	}))
	defer srv.Close()
	c := client.New(&config.Config{BaseURL: srv.URL, ServosityMspToken: "fixture"}, 5*time.Second, 0)
	c.NoCache = true
	items, err := fetchFailures(context.Background(), c, "/companies/1/restic-backup-failures/", map[string]string{"month": "2026-10-01"})
	if err != nil || len(items) != 2 {
		t.Fatalf("fetchFailures = %d items, err %v; want both pages", len(items), err)
	}
}

// TestFailureClustersKeepsPartialEvidence: a later page that fails must not
// turn the failures already read into a successful zero.
func TestFailureClustersKeepsPartialEvidence(t *testing.T) {
	isolateNovelStore(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/companies/1/restic-backup-failures/" && r.URL.Query().Get("page") == "2":
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"detail":"forbidden"}`))
		case r.URL.Path == "/companies/1/restic-backup-failures/":
			_, _ = w.Write([]byte(`{"next":"https://x/companies/1/restic-backup-failures/?page=2","results":[{"errors":"VSS timed out","resticbackupset":{"resticbackup":{"id":1,"device_name":"A"}}}]}`))
		default:
			_, _ = w.Write([]byte(`{"next":null,"results":[]}`))
		}
	}))
	defer srv.Close()
	t.Setenv("SERVOSITY_MSP_BASE_URL", srv.URL)
	t.Setenv("SERVOSITY_MSP_TOKEN", "fixture-token")

	var got failureClustersResult
	if err := json.Unmarshal([]byte(runNovel(t, "failure-clusters", "--companies", "1,2", "--engine", "restic", "--month", "2026-10", "--no-cache", "--json")), &got); err != nil {
		t.Fatal(err)
	}
	if got.Complete || len(got.Incomplete) != 1 || got.Failures != 1 || len(got.Clusters) != 1 {
		t.Fatalf("result = %+v; want the first page's failure kept and the gap reported", got)
	}
}
