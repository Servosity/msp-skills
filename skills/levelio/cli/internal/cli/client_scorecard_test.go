// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import "testing"

func TestLvlComputeClientScorecard(t *testing.T) {
	res := lvlComputeClientScorecard(lvlFxDevices(), lvlFxGroups(), lvlFxAlerts(), lvlFxUpdates(), 7, "criticals", lvlFxNow())
	// Fixture topology: g1 (Root) is the only top-level group and g2 is its
	// child, so dev1 (g1), dev2 (g2), dev3 (g1) roll up to Root; dev4 has no
	// group -> "(no group)".
	if len(res.Clients) != 2 {
		t.Fatalf("expected 2 clients (Root + no-group), got %d: %+v", len(res.Clients), res.Clients)
	}
	root := res.Clients[0]
	if root.Client != "Root" {
		t.Fatalf("expected Root first (most criticals), got %q", root.Client)
	}
	if root.Devices != 3 {
		t.Errorf("Root devices = %d, want 3", root.Devices)
	}
	// dev1 has the unresolved critical alert (al1); al3 is resolved.
	if root.OpenCritical != 1 {
		t.Errorf("Root open criticals = %d, want 1", root.OpenCritical)
	}
	// dev2 is 240h (10d) dark and not in maintenance; dev3 has no timestamp;
	// dev1 is 1h dark.
	if root.Stale != 1 {
		t.Errorf("Root stale = %d, want 1", root.Stale)
	}
	// Pending updates on Root devices: u1, u2 (dev1), u4 is errored not pending.
	if root.PendingUpdates != 2 {
		t.Errorf("Root pending updates = %d, want 2", root.PendingUpdates)
	}
	// Avg of dev1 (40) and dev2 (90).
	if root.AvgSecurityScore == nil || *root.AvgSecurityScore != 65.0 {
		t.Errorf("Root avg score = %v, want 65.0", root.AvgSecurityScore)
	}
	ng := res.Clients[1]
	if ng.Client != "(no group)" || ng.Devices != 1 {
		t.Errorf("no-group row = %+v, want 1 device", ng)
	}
}

func TestLvlComputeClientScorecardEmpty(t *testing.T) {
	res := lvlComputeClientScorecard(nil, nil, nil, nil, 14, "criticals", lvlFxNow())
	if len(res.Clients) != 0 {
		t.Fatalf("expected no clients for empty store, got %+v", res.Clients)
	}
}

func TestLvlComputeClientScorecardSameNameClients(t *testing.T) {
	// Two distinct top-level groups sharing a display name must not share a
	// score sum (regression: scoreSum was keyed by display name).
	groups := []lvlGroup{
		{ID: "ga", Name: "Acme", ParentID: ""},
		{ID: "gb", Name: "Acme", ParentID: ""},
	}
	devices := []lvlDevice{
		{ID: "d1", Hostname: "a1", GroupID: "ga", SecurityScore: lvlIntp(10)},
		{ID: "d2", Hostname: "b1", GroupID: "gb", SecurityScore: lvlIntp(90)},
	}
	res := lvlComputeClientScorecard(devices, groups, nil, nil, 14, "criticals", lvlFxNow())
	if len(res.Clients) != 2 {
		t.Fatalf("expected 2 clients, got %+v", res.Clients)
	}
	got := map[string]float64{}
	for _, c := range res.Clients {
		if c.AvgSecurityScore == nil {
			t.Fatalf("missing avg for %s/%s", c.Client, c.GroupID)
		}
		got[c.GroupID] = *c.AvgSecurityScore
	}
	if got["ga"] != 10.0 || got["gb"] != 90.0 {
		t.Fatalf("avg collision: got %v, want ga=10 gb=90", got)
	}
}

func TestLvlComputeClientScorecardUnsyncedGroup(t *testing.T) {
	// A device whose group_id references an unsynced group gets its own
	// bucket (raw id label), not the "(no group)" bucket.
	devices := []lvlDevice{
		{ID: "d1", Hostname: "a1", GroupID: "g-missing"},
		{ID: "d2", Hostname: "a2", GroupID: ""},
	}
	res := lvlComputeClientScorecard(devices, nil, nil, nil, 14, "criticals", lvlFxNow())
	if len(res.Clients) != 2 {
		t.Fatalf("expected 2 buckets, got %+v", res.Clients)
	}
	labels := map[string]int{}
	for _, c := range res.Clients {
		labels[c.Client] = c.Devices
	}
	if labels["g-missing"] != 1 || labels["(no group)"] != 1 {
		t.Fatalf("bucketing wrong: %v", labels)
	}
}
