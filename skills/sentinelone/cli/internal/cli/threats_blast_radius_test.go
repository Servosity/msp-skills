// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import "testing"

func TestBlastRadiusJoinsOneThreatAcrossEndpoints(t *testing.T) {
	sha := "3f5a9c2e1b7d8a4f6c0e2d1a9b8c7f6e5d4c3b2a"
	threats := []map[string]any{
		threatFix("t1", "Mimikatz", sha, "BOX-1", "SiteA", map[string]any{"mitigationStatus": "active", "incidentStatus": "unresolved"}),
		threatFix("t2", "Mimikatz", sha, "BOX-2", "SiteB", map[string]any{"mitigationStatus": "mitigated", "incidentStatus": "resolved"}),
		// Different threat must not match.
		threatFix("t3", "Other", "ffffffff", "BOX-3", "SiteA", nil),
	}
	db := newSeededStore(t, nil, threats)
	out := runJSON(t, newNovelThreatsBlastRadiusCmd(jsonFlags()), "--db", db, sha)
	if tot, _ := out["endpoints_total"].(float64); tot != 2 {
		t.Fatalf("endpoints_total = %v, want 2", out["endpoints_total"])
	}
	if a, _ := out["active"].(float64); a != 1 {
		t.Fatalf("active = %v, want 1", out["active"])
	}
	if m, _ := out["mitigated"].(float64); m != 1 {
		t.Fatalf("mitigated = %v, want 1", out["mitigated"])
	}
	if sites, ok := out["sites_affected"].([]any); !ok || len(sites) != 2 {
		t.Fatalf("sites_affected = %v, want 2 sites", out["sites_affected"])
	}
}

func TestBlastRadiusNoMatchIsHonest(t *testing.T) {
	db := newSeededStore(t, nil, []map[string]any{threatFix("t1", "X", "aaaa", "B", "S", nil)})
	out := runJSON(t, newNovelThreatsBlastRadiusCmd(jsonFlags()), "--db", db, "no-such-hash")
	if c, _ := out["count"].(float64); c != 0 {
		t.Fatalf("expected honest-empty for an unknown hash, got %v", out)
	}
}
