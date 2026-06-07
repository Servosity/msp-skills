// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored tests for the Kaseya BMS novel commands.

package cli

import (
	"testing"
	"time"
)

var plNow = time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC)

func plOpp(subject, account, status string, amount, probability float64, inPipeline bool, closeDate string) map[string]any {
	return map[string]any{
		"Subject": subject, "AccountName": account, "Status": status,
		"Amount": amount, "Probability": probability, "InPipeline": inPipeline,
		"CloseDate": closeDate,
	}
}

func TestAggregatePipeline(t *testing.T) {
	rows := []map[string]any{
		plOpp("Deal A", "Acme", "Qualified", 10000, 50, true, "2026-07-01T00:00:00"),
		plOpp("Deal B", "Beta", "Qualified", 5000, 20, true, "2026-05-01T00:00:00"), // slipped
		plOpp("Deal C", "Gamma", "Proposal", 20000, 80, true, "2026-08-01T00:00:00"),
		plOpp("Deal D", "Delta", "Won", 9999, 100, false, "2026-05-01T00:00:00"), // not in pipeline
	}
	got := aggregatePipeline(rows, 20, "percent", plNow)
	if got.TotalOpen != 3 {
		t.Errorf("TotalOpen = %d, want 3", got.TotalOpen)
	}
	if got.TotalAmount != 35000 {
		t.Errorf("TotalAmount = %v, want 35000", got.TotalAmount)
	}
	if got.TotalWeighted != 22000 { // 5000 + 1000 + 16000
		t.Errorf("TotalWeighted = %v, want 22000", got.TotalWeighted)
	}
	if len(got.Stages) != 2 {
		t.Fatalf("len(Stages) = %d, want 2", len(got.Stages))
	}
	if got.Stages[0].Status != "Proposal" {
		t.Errorf("top stage = %s, want Proposal (largest amount)", got.Stages[0].Status)
	}
	if len(got.Slipped) != 1 || got.Slipped[0].Subject != "Deal B" {
		t.Fatalf("Slipped = %+v, want one entry Deal B", got.Slipped)
	}
	if got.Slipped[0].DaysLate < 36 || got.Slipped[0].DaysLate > 38 {
		t.Errorf("DaysLate = %d, want ~37", got.Slipped[0].DaysLate)
	}
}

func TestAggregatePipelineSlippedLimit(t *testing.T) {
	rows := []map[string]any{
		plOpp("Deal A", "Acme", "Qualified", 100, 50, true, "2026-05-01T00:00:00"),
		plOpp("Deal B", "Beta", "Qualified", 100, 50, true, "2026-04-01T00:00:00"),
	}
	got := aggregatePipeline(rows, 1, "percent", plNow)
	if len(got.Slipped) != 1 || got.Slipped[0].Subject != "Deal B" {
		t.Errorf("slipped-limit: got %+v, want only Deal B (most overdue)", got.Slipped)
	}
}

func TestAggregatePipelineFractionScale(t *testing.T) {
	rows := []map[string]any{
		plOpp("Deal A", "Acme", "Qualified", 10000, 0.5, true, "2026-07-01T00:00:00"),
	}
	got := aggregatePipeline(rows, 20, "fraction", plNow)
	if got.TotalWeighted != 5000 {
		t.Errorf("fraction scale: TotalWeighted = %v, want 5000", got.TotalWeighted)
	}
}

func TestAggregatePipelineEmpty(t *testing.T) {
	got := aggregatePipeline(nil, 20, "percent", plNow)
	if got.Note == "" {
		t.Errorf("expected note on empty mirror")
	}
}
