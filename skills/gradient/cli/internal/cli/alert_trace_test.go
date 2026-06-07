// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"gradient-pp-cli/internal/ledger"
)

func TestNovelAlertTraceStuckFilter(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GRADIENT_LEDGER_DIR", dir)
	now := time.Now().UTC()
	for _, rec := range []ledger.AlertRecord{
		{At: now, AccountID: "a1", MessageID: "m1", Title: "one", TicketStatus: "created", TicketID: "T-1"},
		{At: now, AccountID: "a2", MessageID: "m2", Title: "two", TicketStatus: "pending"},
		{At: now, AccountID: "a3", MessageID: "m3", Title: "three", TicketStatus: "timeout"},
	} {
		if err := ledger.AppendAlert(dir, rec); err != nil {
			t.Fatal(err)
		}
	}
	flags := &rootFlags{asJSON: true, dataSource: "auto"}
	cmd := newNovelAlertTraceCmd(flags)
	if err := cmd.Flags().Set("stuck", "true"); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("trace: %v", err)
	}
	var view struct {
		Alerts     []ledger.AlertRecord `json:"alerts"`
		Total      int                  `json:"total"`
		StuckCount int                  `json:"stuck_count"`
	}
	if err := json.Unmarshal(out.Bytes(), &view); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out.String())
	}
	if view.Total != 3 || view.StuckCount != 2 || len(view.Alerts) != 2 {
		t.Errorf("stuck filter mismatch: total=%d stuck=%d listed=%d", view.Total, view.StuckCount, len(view.Alerts))
	}
	for _, a := range view.Alerts {
		if a.TicketStatus == "created" {
			t.Errorf("created alert leaked through --stuck: %+v", a)
		}
	}
}

func TestNovelAlertTraceEmptyLedgerNote(t *testing.T) {
	t.Setenv("GRADIENT_LEDGER_DIR", t.TempDir())
	flags := &rootFlags{asJSON: true, dataSource: "auto"}
	cmd := newNovelAlertTraceCmd(flags)
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("trace: %v", err)
	}
	var view struct {
		Note  string `json:"note"`
		Total int    `json:"total"`
	}
	if err := json.Unmarshal(out.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if view.Total != 0 || view.Note == "" {
		t.Errorf("empty ledger should produce an honest note, got %+v", view)
	}
}
