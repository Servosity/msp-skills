// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"testing"
	"time"
)

func TestFleetHealthStaleRanksDecay(t *testing.T) {
	old := time.Now().Add(-30 * 24 * time.Hour).UTC().Format(time.RFC3339)
	agents := []map[string]any{
		// Healthy: online, up-to-date, protect, recent — should score ~0.
		agentFix("a-healthy", "HEALTHY-01", "SiteA", "24.1.2.6", nil),
		// Decayed: offline 30d, out-of-date, detect-only, infected.
		agentFix("a-bad", "ROTTEN-01", "SiteA", "22.0.0.1", map[string]any{
			"networkStatus":  "disconnected",
			"isUpToDate":     false,
			"mitigationMode": "detect",
			"infected":       true,
			"lastActiveDate": old,
			"scanFinishedAt": old,
		}),
	}
	db := newSeededStore(t, agents, nil)

	out := runJSON(t, newNovelFleetHealthStaleCmd(jsonFlags()), "--db", db)
	list, ok := out["agents"].([]any)
	if !ok || len(list) == 0 {
		t.Fatalf("expected ranked agents, got %v", out)
	}
	first := list[0].(map[string]any)
	if name, _ := first["computer_name"].(string); name != "ROTTEN-01" {
		t.Fatalf("expected most-decayed agent ROTTEN-01 ranked first, got %q", name)
	}
	if score, _ := first["decay_score"].(float64); score < 90 {
		t.Fatalf("expected ROTTEN-01 decay score >= 90 (offline+outdated+detect+infected), got %.1f", score)
	}
}

func TestFleetHealthStaleMinScoreFiltersHealthy(t *testing.T) {
	agents := []map[string]any{
		agentFix("a1", "HEALTHY-01", "SiteA", "24.1.2.6", nil),
		agentFix("a2", "HEALTHY-02", "SiteB", "24.1.2.6", nil),
	}
	db := newSeededStore(t, agents, nil)
	out := runJSON(t, newNovelFleetHealthStaleCmd(jsonFlags()), "--db", db, "--min-score", "50")
	// All healthy → nothing clears the threshold → honest empty (count 0).
	if c, _ := out["count"].(float64); c != 0 {
		t.Fatalf("expected count 0 for a healthy fleet above min-score, got %v (out=%v)", c, out)
	}
}

func TestFleetHealthSummaryCounts(t *testing.T) {
	agents := []map[string]any{
		agentFix("a1", "ON-1", "SiteA", "24.1.2.6", nil),
		agentFix("a2", "OFF-1", "SiteA", "24.1.2.6", map[string]any{"networkStatus": "disconnected"}),
		agentFix("a3", "INF-1", "SiteB", "24.1.2.6", map[string]any{"infected": true}),
	}
	db := newSeededStore(t, agents, nil)
	out := runJSON(t, newNovelFleetHealthCmd(jsonFlags()), "--db", db)
	if tot, _ := out["total_agents"].(float64); tot != 3 {
		t.Fatalf("total_agents = %v, want 3", out["total_agents"])
	}
	if on, _ := out["online"].(float64); on != 2 {
		t.Fatalf("online = %v, want 2", out["online"])
	}
	if inf, _ := out["infected"].(float64); inf != 1 {
		t.Fatalf("infected = %v, want 1", out["infected"])
	}
}
