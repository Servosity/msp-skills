// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written (Phase 3): behavioral tests for company show against a seeded store.

package cli

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestNovelCompanyShowCommand(t *testing.T) {
	db := seedTranscendenceStore(t)
	dbPath := db.Path()
	db.Close()

	cmd := newNovelCompanyShowCmd(&rootFlags{asJSON: true})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"--db", dbPath, "c-active"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("company show execute: %v", err)
	}
	var res companyShowResult
	if err := json.Unmarshal(buf.Bytes(), &res); err != nil {
		t.Fatalf("parsing company show JSON: %v\n%s", err, buf.String())
	}
	if pax8FieldStr(res.Company, "id") != "c-active" {
		t.Errorf("company id = %q, want c-active", pax8FieldStr(res.Company, "id"))
	}
	if len(res.Subscriptions) != 1 {
		t.Errorf("c-active subscriptions = %d, want 1", len(res.Subscriptions))
	}
	if len(res.Invoices) != 1 {
		t.Errorf("c-active invoices = %d, want 1", len(res.Invoices))
	}
	if len(res.UsageSummaries) != 2 {
		t.Errorf("c-active usage summaries = %d, want 2", len(res.UsageSummaries))
	}

	// Unknown company -> error (not a silent empty 360).
	cmdMiss := newNovelCompanyShowCmd(&rootFlags{asJSON: true})
	var bufMiss bytes.Buffer
	cmdMiss.SetOut(&bufMiss)
	cmdMiss.SetErr(&bufMiss)
	cmdMiss.SetArgs([]string{"--db", dbPath, "does-not-exist"})
	if err := cmdMiss.Execute(); err == nil {
		t.Error("expected error for unknown company id")
	}
}
