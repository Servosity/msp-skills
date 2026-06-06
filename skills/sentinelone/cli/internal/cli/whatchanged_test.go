// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"testing"
	"time"
)

func TestWhatchangedDiffsSnapshots(t *testing.T) {
	db := newSeededStore(t, nil, nil)
	base := time.Now().Add(-10 * 24 * time.Hour)
	latest := time.Now().Add(-1 * time.Hour)

	// Baseline: agent online on v23, no threats.
	seedSnapshot(t, db, "agents", base, []map[string]any{
		agentFix("a1", "BOX-1", "SiteA", "23.0.0.1", nil),
	})
	seedSnapshot(t, db, "threats", base, nil)

	// Latest: same agent offline on v24, plus a brand-new threat.
	seedSnapshot(t, db, "agents", latest, []map[string]any{
		agentFix("a1", "BOX-1", "SiteA", "24.1.2.6", map[string]any{"networkStatus": "disconnected"}),
	})
	seedSnapshot(t, db, "threats", latest, []map[string]any{
		threatFix("t1", "NewEvil", "abc123", "BOX-1", "SiteA", nil),
	})

	out := runJSON(t, newNovelWhatchangedCmd(jsonFlags()), "--db", db, "--since", "30d")
	counts := out["counts"].(map[string]any)
	if v, _ := counts["agents_offline"].(float64); v != 1 {
		t.Fatalf("agents_offline = %v, want 1", counts["agents_offline"])
	}
	if v, _ := counts["version_changes"].(float64); v != 1 {
		t.Fatalf("version_changes = %v, want 1", counts["version_changes"])
	}
	if v, _ := counts["new_threats"].(float64); v != 1 {
		t.Fatalf("new_threats = %v, want 1", counts["new_threats"])
	}
}

func TestWhatchangedNeedsTwoSnapshots(t *testing.T) {
	db := newSeededStore(t, nil, nil)
	seedSnapshot(t, db, "agents", time.Now(), []map[string]any{agentFix("a1", "X", "S", "24.1.2.6", nil)})
	out := runJSON(t, newNovelWhatchangedCmd(jsonFlags()), "--db", db, "--since", "24h")
	if c, _ := out["count"].(float64); c != 0 {
		t.Fatalf("expected honest-empty with one snapshot, got %v", out)
	}
}
