// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import "testing"

func TestLvlComputeAtRisk(t *testing.T) {
	devices := lvlFxDevices()
	alerts := lvlFxAlerts()
	updates := lvlFxUpdates()
	groups := lvlFxGroups()
	now := lvlFxNow()

	t.Run("ranking and top", func(t *testing.T) {
		res := lvlComputeAtRisk(devices, alerts, updates, groups, 20, "", now)
		if res.EvaluatedDevices != 4 {
			t.Fatalf("evaluated = %d, want 4", res.EvaluatedDevices)
		}
		if res.Count != 4 {
			t.Fatalf("count = %d, want 4 (all carry some risk)", res.Count)
		}
		// dev1: alerts(critical3+warning2)=5 + 2 pending + securityscore(40 -> +4) = 11 -> highest
		if res.Devices[0].DeviceID != "dev1" {
			t.Errorf("rank1 = %s, want dev1", res.Devices[0].DeviceID)
		}
		if res.Devices[0].Rank != 1 || res.Devices[0].RiskScore < res.Devices[1].RiskScore {
			t.Errorf("ranking not descending: %+v", res.Devices)
		}
		if res.Devices[0].OpenAlerts != 2 || res.Devices[0].PendingUpdates != 2 {
			t.Errorf("dev1 detail = alerts %d pending %d, want 2/2", res.Devices[0].OpenAlerts, res.Devices[0].PendingUpdates)
		}
	})

	t.Run("top cap", func(t *testing.T) {
		res := lvlComputeAtRisk(devices, alerts, updates, groups, 1, "", now)
		if res.Count != 1 || res.Devices[0].DeviceID != "dev1" {
			t.Errorf("top1 = %+v, want only dev1", res.Devices)
		}
	})

	t.Run("group scope excludes ungrouped", func(t *testing.T) {
		res := lvlComputeAtRisk(devices, alerts, updates, groups, 20, "g1", now)
		if res.EvaluatedDevices != 3 { // dev1(g1), dev2(g2 descendant), dev3(g1); dev4 ungrouped excluded
			t.Fatalf("evaluated in g1 subtree = %d, want 3", res.EvaluatedDevices)
		}
		for _, d := range res.Devices {
			if d.DeviceID == "dev4" {
				t.Errorf("dev4 should be excluded by group scope")
			}
		}
	})

	t.Run("resolved alerts do not score", func(t *testing.T) {
		res := lvlComputeAtRisk(devices, alerts, updates, groups, 20, "", now)
		for _, d := range res.Devices {
			if d.DeviceID == "dev2" && d.OpenAlerts != 0 {
				t.Errorf("dev2 open alerts = %d, want 0 (al3 is resolved)", d.OpenAlerts)
			}
		}
	})
}
