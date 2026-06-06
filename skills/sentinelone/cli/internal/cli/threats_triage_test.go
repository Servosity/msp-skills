// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"testing"
	"time"
)

func TestThreatsTriageRanksOpenThreats(t *testing.T) {
	threats := []map[string]any{
		// Resolved threat — must be excluded.
		threatFix("t1", "ResolvedEvil", "aaa1", "HOST-A", "SiteA", map[string]any{
			"confidenceLevel":  "malicious",
			"incidentStatus":   "resolved",
			"mitigationStatus": "mitigated",
		}),
		// Malicious + unresolved + old — should rank highest.
		threatFix("t2", "MaliciousOld", "bbb2", "HOST-B", "SiteA", map[string]any{
			"confidenceLevel":  "malicious",
			"incidentStatus":   "unresolved",
			"mitigationStatus": "active",
			"createdAt":        time.Now().Add(-20 * 24 * time.Hour).UTC().Format(time.RFC3339),
		}),
		// Suspicious + new — lower rank.
		threatFix("t3", "SuspiciousNew", "ccc3", "HOST-C", "SiteB", map[string]any{
			"confidenceLevel":  "suspicious",
			"incidentStatus":   "unresolved",
			"mitigationStatus": "active",
			"createdAt":        time.Now().UTC().Format(time.RFC3339),
		}),
	}
	db := newSeededStore(t, nil, threats)
	out := runJSON(t, newNovelThreatsTriageCmd(jsonFlags()), "--db", db)

	if got, _ := out["open_threats"].(float64); got != 2 {
		t.Fatalf("open_threats = %v, want 2 (resolved excluded)", out["open_threats"])
	}
	items := out["items"].([]any)
	if len(items) != 2 {
		t.Fatalf("items len = %d, want 2", len(items))
	}
	first := items[0].(map[string]any)
	second := items[1].(map[string]any)
	if first["threat"] != "MaliciousOld" {
		t.Fatalf("top-ranked threat = %v, want MaliciousOld", first["threat"])
	}
	if second["threat"] != "SuspiciousNew" {
		t.Fatalf("second threat = %v, want SuspiciousNew", second["threat"])
	}
	fs, _ := first["score"].(float64)
	ss, _ := second["score"].(float64)
	if !(fs > ss) {
		t.Fatalf("malicious-old score %v should exceed suspicious-new score %v", fs, ss)
	}
	if fs <= 0 || ss <= 0 {
		t.Fatalf("scores must be positive, got %v and %v", fs, ss)
	}
	// Resolved threat name must not appear anywhere.
	for _, it := range items {
		if it.(map[string]any)["threat"] == "ResolvedEvil" {
			t.Fatalf("resolved threat leaked into triage output")
		}
	}
}

func TestThreatsTriageSiteFilterNoMatch(t *testing.T) {
	threats := []map[string]any{
		threatFix("t1", "OnlyThreat", "abc1", "HOST-A", "SiteA", map[string]any{
			"confidenceLevel": "malicious",
			"incidentStatus":  "unresolved",
		}),
	}
	db := newSeededStore(t, nil, threats)
	out := runJSON(t, newNovelThreatsTriageCmd(jsonFlags()), "--db", db, "--site", "NoSuchSite")

	// Honest-empty must still report the (zero) open count for the filter.
	if ot, ok := out["open_threats"].(float64); !ok || ot != 0 {
		t.Fatalf("open_threats = %v, want 0 for unmatched site", out["open_threats"])
	}
	if c, _ := out["count"].(float64); c != 0 {
		t.Fatalf("honest-empty count = %v, want 0", out["count"])
	}
}
