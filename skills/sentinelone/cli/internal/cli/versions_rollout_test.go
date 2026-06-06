// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"testing"
	"time"
)

func TestVersionsRolloutProgressesOverSnapshots(t *testing.T) {
	db := newSeededStore(t, nil, nil)
	t1 := time.Now().Add(-48 * time.Hour)
	t2 := time.Now().Add(-1 * time.Hour)
	// t1: 1 of 2 on target. t2: 2 of 2 on target (rollout completed).
	seedSnapshot(t, db, "agents", t1, []map[string]any{
		agentFix("a1", "H1", "SiteA", "24.1.2.6", nil),
		agentFix("a2", "H2", "SiteA", "23.0.0.1", nil),
	})
	seedSnapshot(t, db, "agents", t2, []map[string]any{
		agentFix("a1", "H1", "SiteA", "24.1.2.6", nil),
		agentFix("a2", "H2", "SiteA", "24.1.2.6", nil),
	})
	out := runJSON(t, newNovelVersionsRolloutCmd(jsonFlags()), "--db", db)
	if tv, _ := out["target_version"].(string); tv != "24.1.2.6" {
		t.Fatalf("target_version = %q, want 24.1.2.6", tv)
	}
	tl := out["timeline"].([]any)
	if len(tl) != 2 {
		t.Fatalf("timeline length = %d, want 2", len(tl))
	}
	last := tl[1].(map[string]any)
	if pct, _ := last["on_target_pct"].(float64); pct != 100 {
		t.Fatalf("final on_target_pct = %v, want 100", last["on_target_pct"])
	}
	if stalled, _ := out["stalled"].(bool); stalled {
		t.Fatalf("rollout should not be flagged stalled (50%%→100%%)")
	}
}

func TestVersionsRolloutNoHistoryIsHonest(t *testing.T) {
	db := newSeededStore(t, nil, nil)
	out := runJSON(t, newNovelVersionsRolloutCmd(jsonFlags()), "--db", db)
	if c, _ := out["count"].(float64); c != 0 {
		t.Fatalf("expected honest-empty with no history, got %v", out)
	}
}
