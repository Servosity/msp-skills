// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import "testing"

func TestCoverageGapsFlagsDetectOnly(t *testing.T) {
	agents := []map[string]any{
		agentFix("a1", "PROTECTED-1", "SiteA", "24.1.2.6", nil),
		agentFix("a2", "DETECT-1", "SiteA", "24.1.2.6", map[string]any{"mitigationMode": "detect"}),
		// Decommissioned detect-only agent must be excluded.
		agentFix("a3", "DECOMM-1", "SiteB", "24.1.2.6", map[string]any{"mitigationMode": "detect", "isDecommissioned": true}),
	}
	db := newSeededStore(t, agents, nil)
	out := runJSON(t, newNovelCoverageGapsCmd(jsonFlags()), "--db", db)
	if got, _ := out["total_gaps"].(float64); got != 1 {
		t.Fatalf("total_gaps = %v, want 1 (only the active detect-only agent)", out["total_gaps"])
	}
	eps := out["endpoints"].([]any)
	first := eps[0].(map[string]any)
	if name, _ := first["computer_name"].(string); name != "DETECT-1" {
		t.Fatalf("gap endpoint = %q, want DETECT-1", name)
	}
}

func TestCoverageGapsHealthyFleetIsHonestEmpty(t *testing.T) {
	agents := []map[string]any{agentFix("a1", "OK-1", "SiteA", "24.1.2.6", nil)}
	db := newSeededStore(t, agents, nil)
	out := runJSON(t, newNovelCoverageGapsCmd(jsonFlags()), "--db", db)
	if c, _ := out["count"].(float64); c != 0 {
		t.Fatalf("expected honest-empty count 0 for a fully-protected fleet, got %v", out)
	}
}

func TestCoverageSummaryPercent(t *testing.T) {
	agents := []map[string]any{
		agentFix("a1", "P1", "SiteA", "24.1.2.6", nil),
		agentFix("a2", "P2", "SiteA", "24.1.2.6", map[string]any{"mitigationMode": "detect"}),
	}
	db := newSeededStore(t, agents, nil)
	out := runJSON(t, newNovelCoverageCmd(jsonFlags()), "--db", db)
	if pct, _ := out["overall_coverage_pct"].(float64); pct != 50 {
		t.Fatalf("overall_coverage_pct = %v, want 50", out["overall_coverage_pct"])
	}
}
