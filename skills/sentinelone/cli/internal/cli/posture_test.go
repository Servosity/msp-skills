// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"testing"
	"time"
)

func TestPostureScorecard(t *testing.T) {
	agents := []map[string]any{
		// SiteA: 2 agents, 1 healthy, 1 unhealthy (offline).
		agentFix("a1", "A1", "SiteA", "24.1.2.6", nil),
		agentFix("a2", "A2", "SiteA", "24.1.2.6", map[string]any{"networkStatus": "disconnected"}),
	}
	threats := []map[string]any{
		threatFix("t1", "Evil", "deadbeef", "A1", "SiteA", map[string]any{
			"incidentStatus":   "unresolved",
			"mitigationStatus": "active",
			"createdAt":        time.Now().Add(-72 * time.Hour).UTC().Format(time.RFC3339),
		}),
	}
	db := newSeededStore(t, agents, threats)
	out := runJSON(t, newNovelPostureCmd(jsonFlags()), "--db", db)
	sites := out["sites"].([]any)
	var siteA map[string]any
	for _, s := range sites {
		m := s.(map[string]any)
		if m["site"] == "SiteA" {
			siteA = m
		}
	}
	if siteA == nil {
		t.Fatalf("SiteA not in posture output: %v", out)
	}
	if hp, _ := siteA["health_pct"].(float64); hp != 50 {
		t.Fatalf("SiteA health_pct = %v, want 50 (1 of 2 healthy)", siteA["health_pct"])
	}
	if ot, _ := siteA["open_threats"].(float64); ot != 1 {
		t.Fatalf("SiteA open_threats = %v, want 1", siteA["open_threats"])
	}
	if od, _ := siteA["oldest_open_threat_days"].(float64); od < 2 {
		t.Fatalf("SiteA oldest_open_threat_days = %v, want >= 2", siteA["oldest_open_threat_days"])
	}
}
