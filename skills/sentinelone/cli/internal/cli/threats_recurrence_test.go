// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import "testing"

func TestThreatsRecurrenceGroupsByHash(t *testing.T) {
	threats := []map[string]any{
		threatFix("t1", "Repeater", "shaX", "BOX-1", "SiteA", map[string]any{"mitigationStatus": "mitigated"}),
		threatFix("t2", "Repeater", "shaX", "BOX-2", "SiteA", map[string]any{"mitigationStatus": "active"}),
		threatFix("t3", "OneOff", "shaY", "BOX-3", "SiteB", nil),
	}
	db := newSeededStore(t, nil, threats)

	out := runJSON(t, newNovelThreatsRecurrenceCmd(jsonFlags()), "--db", db)
	if tr, _ := out["total_recurring"].(float64); tr != 1 {
		t.Fatalf("total_recurring = %v, want 1 (only shaX recurs)", out["total_recurring"])
	}
	g := out["threats"].([]any)[0].(map[string]any)
	if g["key"] != "shaX" {
		t.Fatalf("recurring key = %v, want shaX", g["key"])
	}
	if de, _ := g["distinct_endpoints"].(float64); de != 2 {
		t.Fatalf("distinct_endpoints = %v, want 2", g["distinct_endpoints"])
	}
	if back, _ := g["recurred_after_mitigation"].(bool); !back {
		t.Fatalf("expected recurred_after_mitigation true (one mitigated + one active)")
	}
}

func TestThreatsRecurrenceByAgent(t *testing.T) {
	threats := []map[string]any{
		threatFix("t1", "Repeater", "shaX", "BOX-1", "SiteA", nil),
		threatFix("t2", "Repeater", "shaX", "BOX-2", "SiteA", nil),
	}
	db := newSeededStore(t, nil, threats)
	out := runJSON(t, newNovelThreatsRecurrenceCmd(jsonFlags()), "--db", db, "--by-agent")
	eps := out["endpoints"].([]any)
	if len(eps) != 2 {
		t.Fatalf("by-agent endpoints = %d, want 2", len(eps))
	}
}
