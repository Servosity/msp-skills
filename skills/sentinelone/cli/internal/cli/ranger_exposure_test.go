// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import "testing"

func TestRangerExposureFindsRogueBesideManaged(t *testing.T) {
	agents := []map[string]any{
		agentFix("a1", "MANAGED-1", "SiteA", "24.1.2.6", map[string]any{"lastIpToMgmt": "10.0.0.5"}),
	}
	db := newSeededStore(t, agents, nil)
	// A rogue device on the same /24 as a managed agent.
	seedResource(t, db, "rogues", []map[string]any{
		{"id": "r1", "deviceName": "UNKNOWN-LAPTOP", "ip": "10.0.0.9", "macAddress": "aa:bb:cc:dd:ee:ff"},
	})

	out := runJSON(t, newNovelRangerExposureCmd(jsonFlags()), "--db", db)
	if tot, _ := out["total_unmanaged"].(float64); tot != 1 {
		t.Fatalf("total_unmanaged = %v, want 1", out["total_unmanaged"])
	}
	subnets := out["subnets"].([]any)
	s0 := subnets[0].(map[string]any)
	if s0["subnet"] != "10.0.0.0/24" {
		t.Fatalf("subnet = %v, want 10.0.0.0/24", s0["subnet"])
	}
	if mp, _ := s0["managed_peers"].(float64); mp != 1 {
		t.Fatalf("managed_peers = %v, want 1", s0["managed_peers"])
	}
}

func TestRangerExposureNoDataIsHonest(t *testing.T) {
	db := newSeededStore(t, []map[string]any{agentFix("a1", "M", "S", "24.1.2.6", nil)}, nil)
	out := runJSON(t, newNovelRangerExposureCmd(jsonFlags()), "--db", db)
	if c, _ := out["count"].(float64); c != 0 {
		t.Fatalf("expected honest-empty with no ranger/rogues data, got %v", out)
	}
}
