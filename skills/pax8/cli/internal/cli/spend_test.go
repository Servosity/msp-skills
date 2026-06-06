// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written (Phase 3): behavioral tests for spend against a seeded store.

package cli

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestNovelSpendCommand(t *testing.T) {
	db := seedTranscendenceStore(t)
	dbPath := db.Path()
	db.Close()

	// Invoices: c-active=100, c-billed=250. Ranked: c-billed first.
	cmd := newNovelSpendCmd(&rootFlags{asJSON: true})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"--db", dbPath})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("spend execute: %v", err)
	}
	var rows []spendRow
	if err := json.Unmarshal(buf.Bytes(), &rows); err != nil {
		t.Fatalf("parsing spend JSON: %v\n%s", err, buf.String())
	}
	if len(rows) < 2 {
		t.Fatalf("expected >=2 companies with invoices, got %d", len(rows))
	}
	if rows[0].CompanyID != "c-billed" {
		t.Errorf("top spender = %q, want c-billed (250 > 100)", rows[0].CompanyID)
	}
	if rows[0].Total != 250.0 {
		t.Errorf("c-billed total = %v, want 250", rows[0].Total)
	}
	if rows[0].CompanyName != "Billed MSP" {
		t.Errorf("company name not resolved: got %q", rows[0].CompanyName)
	}
}
