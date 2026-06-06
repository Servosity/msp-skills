// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written (Phase 3): behavioral tests for mrr against a seeded store.

package cli

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestNovelMrrCommand(t *testing.T) {
	db := seedTranscendenceStore(t)
	dbPath := db.Path()
	db.Close()

	cmd := newNovelMrrCmd(&rootFlags{asJSON: true})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"--db", dbPath})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("mrr execute: %v", err)
	}

	var res mrrResult
	if err := json.Unmarshal(buf.Bytes(), &res); err != nil {
		t.Fatalf("parsing mrr JSON: %v\n%s", err, buf.String())
	}
	// Active subs only: s-active (20*5=100, margin (20-15)*5=25) + s-noinv (100*1=100, margin 20).
	// s-cancelled is excluded. Expected MRR=200, margin=45, active=2.
	wantMRR := 200.0
	wantMargin := 45.0
	if res.Totals.MRR != wantMRR {
		t.Errorf("MRR = %v, want %v", res.Totals.MRR, wantMRR)
	}
	if res.Totals.Margin != wantMargin {
		t.Errorf("Margin = %v, want %v", res.Totals.Margin, wantMargin)
	}
	if res.Totals.ActiveSubscriptions != 2 {
		t.Errorf("active subs = %d, want 2 (cancelled excluded)", res.Totals.ActiveSubscriptions)
	}
	if len(res.ByProduct) == 0 {
		t.Error("expected per-product breakdown")
	}

	// --all should include the cancelled subscription (3 total).
	cmdAll := newNovelMrrCmd(&rootFlags{asJSON: true})
	var bufAll bytes.Buffer
	cmdAll.SetOut(&bufAll)
	cmdAll.SetArgs([]string{"--db", dbPath, "--all"})
	if err := cmdAll.Execute(); err != nil {
		t.Fatalf("mrr --all execute: %v", err)
	}
	var resAll mrrResult
	if err := json.Unmarshal(bufAll.Bytes(), &resAll); err != nil {
		t.Fatalf("parsing mrr --all JSON: %v", err)
	}
	if resAll.Totals.ActiveSubscriptions != 3 {
		t.Errorf("--all subs = %d, want 3", resAll.Totals.ActiveSubscriptions)
	}
}
