// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import "testing"

func TestSitesRiskRanksHigherForThreatsAndGaps(t *testing.T) {
	agents := []map[string]any{
		// Site A: 2 agents, one in detect-only (coverage gap).
		agentFix("a1", "A-1", "SiteA", "24.1.2.6", nil),
		agentFix("a2", "A-2", "SiteA", "24.1.2.6", map[string]any{"mitigationMode": "detect"}),
		// Site B: 2 agents, all protected.
		agentFix("b1", "B-1", "SiteB", "24.1.2.6", nil),
		agentFix("b2", "B-2", "SiteB", "24.1.2.6", nil),
	}
	threats := []map[string]any{
		// One open threat on Site A.
		threatFix("t1", "Evil", "abc1", "A-1", "SiteA", map[string]any{
			"incidentStatus":   "unresolved",
			"mitigationStatus": "active",
		}),
	}
	db := newSeededStore(t, agents, threats)
	out := runJSON(t, newNovelSitesRiskCmd(jsonFlags()), "--db", db)

	sites := out["sites"].([]any)
	var a, b map[string]any
	for _, s := range sites {
		m := s.(map[string]any)
		switch m["site"] {
		case "SiteA":
			a = m
		case "SiteB":
			b = m
		}
	}
	if a == nil || b == nil {
		t.Fatalf("both sites should be present: %v", out)
	}
	ar, _ := a["risk_score"].(float64)
	br, _ := b["risk_score"].(float64)
	if !(ar > br) {
		t.Fatalf("SiteA risk %v should exceed SiteB risk %v", ar, br)
	}
	// Fields populated for Site A.
	if ot, _ := a["open_threats"].(float64); ot != 1 {
		t.Fatalf("SiteA open_threats = %v, want 1", a["open_threats"])
	}
	if cg, _ := a["coverage_gap_pct"].(float64); cg != 50 {
		t.Fatalf("SiteA coverage_gap_pct = %v, want 50 (1 of 2 in detect-only)", a["coverage_gap_pct"])
	}
	if ag, _ := a["agents"].(float64); ag != 2 {
		t.Fatalf("SiteA agents = %v, want 2", a["agents"])
	}
	// Site B is clean.
	if br != 0 {
		t.Fatalf("SiteB risk = %v, want 0 (no threats, full coverage)", br)
	}
}

func TestSitesRiskNoAgentsIsHonestEmpty(t *testing.T) {
	db := newSeededStore(t, nil, nil)
	out := runJSON(t, newNovelSitesRiskCmd(jsonFlags()), "--db", db)
	if c, _ := out["count"].(float64); c != 0 {
		t.Fatalf("expected honest-empty count 0 with no agents, got %v", out)
	}
}
