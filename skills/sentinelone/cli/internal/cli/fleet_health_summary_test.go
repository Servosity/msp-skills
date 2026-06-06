// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import "testing"

func summaryTestAgents() []map[string]any {
	return []map[string]any{
		// online + in protect + healthy
		agentFix("a1", "ONLINE-1", "SiteA", "24.1.2.6", nil),
		// offline (disconnected, not decommissioned)
		agentFix("a2", "OFFLINE-1", "SiteA", "24.1.2.6", map[string]any{"networkStatus": "disconnected"}),
		// infected + out-of-date (online)
		agentFix("a3", "INFECTED-1", "SiteB", "24.1.2.6", map[string]any{"infected": true, "isUpToDate": false}),
		// decommissioned (excluded from quality counts; counts as decommissioned, not offline)
		agentFix("a4", "DECOMM-1", "SiteB", "24.1.2.6", map[string]any{"networkStatus": "disconnected", "isDecommissioned": true}),
	}
}

func TestFleetHealthSummaryTotals(t *testing.T) {
	db := newSeededStore(t, summaryTestAgents(), nil)
	out := runJSON(t, newNovelFleetHealthSummaryCmd(jsonFlags()), "--db", db)

	fleet := out["fleet"].(map[string]any)
	num := func(k string) float64 { v, _ := fleet[k].(float64); return v }

	if num("total") != 4 {
		t.Fatalf("total = %v, want 4", fleet["total"])
	}
	if num("online") != 2 {
		t.Fatalf("online = %v, want 2 (ONLINE-1 + INFECTED-1)", fleet["online"])
	}
	// OFFLINE-1 is offline; DECOMM-1 counts as decommissioned, not offline.
	if num("offline") != 1 {
		t.Fatalf("offline = %v, want 1 (decommissioned excluded)", fleet["offline"])
	}
	if num("decommissioned") != 1 {
		t.Fatalf("decommissioned = %v, want 1", fleet["decommissioned"])
	}
	if num("infected") != 1 {
		t.Fatalf("infected = %v, want 1", fleet["infected"])
	}
	if num("out_of_date") != 1 {
		t.Fatalf("out_of_date = %v, want 1", fleet["out_of_date"])
	}
	if at, _ := out["agents_total"].(float64); at != 4 {
		t.Fatalf("agents_total = %v, want 4", out["agents_total"])
	}
	// Without --by-site there should be no sites breakout.
	if _, ok := out["sites"]; ok {
		t.Fatalf("sites should be absent without --by-site")
	}
}

func TestFleetHealthSummaryBySiteSumsToFleet(t *testing.T) {
	db := newSeededStore(t, summaryTestAgents(), nil)
	out := runJSON(t, newNovelFleetHealthSummaryCmd(jsonFlags()), "--db", db, "--by-site")

	fleet := out["fleet"].(map[string]any)
	sites, ok := out["sites"].([]any)
	if !ok || len(sites) != 2 {
		t.Fatalf("expected 2 site rows with --by-site, got %v", out["sites"])
	}

	sum := map[string]float64{}
	for _, s := range sites {
		m := s.(map[string]any)
		for _, k := range []string{"total", "online", "offline", "decommissioned", "infected", "out_of_date", "under_protected"} {
			v, _ := m[k].(float64)
			sum[k] += v
		}
	}
	for _, k := range []string{"total", "online", "offline", "decommissioned", "infected", "out_of_date", "under_protected"} {
		fv, _ := fleet[k].(float64)
		if sum[k] != fv {
			t.Fatalf("per-site %s sum = %v, want fleet %v", k, sum[k], fv)
		}
	}
}
