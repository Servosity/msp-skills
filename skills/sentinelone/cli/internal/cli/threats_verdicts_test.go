// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"testing"
	"time"
)

func TestThreatsVerdictsDistribution(t *testing.T) {
	threats := []map[string]any{
		threatFix("t1", "A", "h1", "B1", "S", map[string]any{"analystVerdict": "true_positive"}),
		threatFix("t2", "B", "h2", "B2", "S", map[string]any{"analystVerdict": "true_positive"}),
		threatFix("t3", "C", "h3", "B3", "S", map[string]any{"analystVerdict": "false_positive"}),
	}
	db := newSeededStore(t, nil, threats)
	out := runJSON(t, newNovelThreatsVerdictsCmd(jsonFlags()), "--db", db)
	verdicts := out["analyst_verdict"].(map[string]any)
	if tp, _ := verdicts["true_positive"].(float64); tp != 2 {
		t.Fatalf("true_positive = %v, want 2", verdicts["true_positive"])
	}
	if fp, _ := verdicts["false_positive"].(float64); fp != 1 {
		t.Fatalf("false_positive = %v, want 1", verdicts["false_positive"])
	}
}

func TestThreatsVerdictsChangedDiffsSnapshots(t *testing.T) {
	db := newSeededStore(t, nil, nil)
	prev := time.Now().Add(-2 * time.Hour)
	latest := time.Now().Add(-1 * time.Hour)
	seedSnapshot(t, db, "threats", prev, []map[string]any{
		threatFix("t1", "Escalator", "h1", "B1", "S", map[string]any{"confidenceLevel": "suspicious"}),
	})
	seedSnapshot(t, db, "threats", latest, []map[string]any{
		threatFix("t1", "Escalator", "h1", "B1", "S", map[string]any{"confidenceLevel": "malicious"}),
	})
	out := runJSON(t, newNovelThreatsVerdictsCmd(jsonFlags()), "--db", db, "--changed")
	changes := out["changes"].([]any)
	if len(changes) != 1 {
		t.Fatalf("changes = %d, want 1 (confidence suspicious→malicious)", len(changes))
	}
	c := changes[0].(map[string]any)
	if c["field"] != "confidence_level" || c["from"] != "suspicious" || c["to"] != "malicious" {
		t.Fatalf("unexpected change: %v", c)
	}
}

func TestThreatsVerdictsChangedNeedsTwoSnapshots(t *testing.T) {
	db := newSeededStore(t, nil, nil)
	seedSnapshot(t, db, "threats", time.Now(), []map[string]any{threatFix("t1", "X", "h", "B", "S", nil)})
	out := runJSON(t, newNovelThreatsVerdictsCmd(jsonFlags()), "--db", db, "--changed")
	if c, _ := out["count"].(float64); c != 0 {
		t.Fatalf("expected honest-empty with one snapshot, got %v", out)
	}
}
