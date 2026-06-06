// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// Acceptance coverage for the eight prior (kept) novel commands, executed
// through the real root command tree against the shared seeded store.

func TestNovelArAgingAcceptance(t *testing.T) {
	dbPath := seedNovelStore(t)
	out := runNovel(t, dbPath, "ar-aging")
	var rep map[string]any
	if err := json.Unmarshal([]byte(out), &rep); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if fmt.Sprint(rep["total_outstanding"]) != "800" { // i1 500 + i2 300
		t.Fatalf("total_outstanding wrong: %+v", rep)
	}
	if rep["by_entity"] == nil || len(rep["by_entity"].([]any)) != 2 {
		t.Fatalf("by_entity must roll up 2 customers: %+v", rep["by_entity"])
	}
}

func TestNovelApAgingAcceptance(t *testing.T) {
	dbPath := seedNovelStore(t)
	out := runNovel(t, dbPath, "ap-aging")
	var rep map[string]any
	if err := json.Unmarshal([]byte(out), &rep); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if fmt.Sprint(rep["total_outstanding"]) != "120" { // b1
		t.Fatalf("total_outstanding wrong: %+v", rep)
	}
}

func TestNovelBalancesAcceptance(t *testing.T) {
	dbPath := seedNovelStore(t)
	out := runNovel(t, dbPath, "balances")
	if !strings.Contains(out, "{") {
		t.Fatalf("balances must emit JSON: %s", out)
	}
	var rep map[string]any
	if err := json.Unmarshal([]byte(out), &rep); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
}

func TestNovelInvoicesStaleAcceptance(t *testing.T) {
	dbPath := seedNovelStore(t)
	out := runNovel(t, dbPath, "invoices", "stale", "--days", "1")
	var rows []map[string]any
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if len(rows) != 1 || fmt.Sprint(rows[0]["doc_number"]) != "INV-1" {
		t.Fatalf("stale list must contain only overdue INV-1: %+v", rows)
	}
}

func TestNovelPaymentsUnappliedAcceptance(t *testing.T) {
	dbPath := seedNovelStore(t)
	out := runNovel(t, dbPath, "payments", "unapplied")
	var rows []map[string]any
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if len(rows) != 1 || fmt.Sprint(rows[0]["unapplied_amt"]) != "50" {
		t.Fatalf("unapplied list wrong: %+v", rows)
	}
}

func TestNovelCustomersTopAcceptance(t *testing.T) {
	dbPath := seedNovelStore(t)
	out := runNovel(t, dbPath, "customers", "top", "--by", "balance")
	var rows []map[string]any
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if len(rows) == 0 || fmt.Sprint(rows[0]["name"]) != "Acme Inc" {
		t.Fatalf("top customer wrong: %+v", rows)
	}
	// negative: bad --by must fail with usage error
	flagsBad := []string{"customers", "top", "--by", "bogus", "--db", dbPath, "--json"}
	flags := &rootFlags{}
	root := newRootCmd(flags)
	root.SetArgs(flagsBad)
	if err := root.Execute(); err == nil {
		t.Fatal("customers top --by bogus must fail")
	}
}

func TestNovelVendorsSpendAcceptance(t *testing.T) {
	dbPath := seedNovelStore(t)
	out := runNovel(t, dbPath, "vendors", "spend")
	var rows []map[string]any
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if len(rows) != 1 || fmt.Sprint(rows[0]["name"]) != "Vendor One" || fmt.Sprint(rows[0]["total"]) != "120" {
		t.Fatalf("vendor spend wrong: %+v", rows)
	}
}

func TestNovelDupesCustomersAcceptance(t *testing.T) {
	dbPath := seedNovelStore(t)
	out := runNovel(t, dbPath, "dupes", "customers")
	var groups []map[string]any
	if err := json.Unmarshal([]byte(out), &groups); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if len(groups) != 1 { // Acme Inc vs Acme, Inc.
		t.Fatalf("want 1 dupe group, got %+v", groups)
	}
	// negative: vendors have no dupes — empty result, not fabricated groups
	outV := runNovel(t, dbPath, "dupes", "vendors")
	trimmed := strings.TrimSpace(outV)
	if trimmed != "[]" && trimmed != "null" {
		var vg []map[string]any
		if err := json.Unmarshal([]byte(outV), &vg); err != nil || len(vg) != 0 {
			t.Fatalf("vendors must have no dupes: %s", outV)
		}
	}
}
