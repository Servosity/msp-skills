// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import "testing"

func TestLvlComputeFleet(t *testing.T) {
	devices := lvlFxDevices()
	groups := lvlFxGroups()

	t.Run("by platform", func(t *testing.T) {
		res := lvlComputeFleet(devices, groups, "platform", "", false, false)
		if res.Summary.Total != 4 || res.Summary.Online != 2 || res.Summary.Offline != 2 || res.Summary.Maintenance != 1 {
			t.Fatalf("summary = %+v, want total4 online2 offline2 maint1", res.Summary)
		}
		if res.Buckets[0].Key != "windows" || res.Buckets[0].Count != 2 {
			t.Errorf("top bucket = %+v, want windows=2", res.Buckets[0])
		}
	})

	t.Run("by group", func(t *testing.T) {
		res := lvlComputeFleet(devices, groups, "group", "", false, false)
		got := map[string]int{}
		for _, b := range res.Buckets {
			got[b.Key] = b.Count
		}
		if got["Root"] != 2 || got["Child"] != 1 || got["(no group)"] != 1 {
			t.Errorf("group buckets = %v, want Root2 Child1 (no group)1", got)
		}
	})

	t.Run("online filter", func(t *testing.T) {
		res := lvlComputeFleet(devices, groups, "platform", "", true, false)
		if res.Summary.Total != 2 || res.Summary.Offline != 0 {
			t.Errorf("online-only summary = %+v, want total2 offline0", res.Summary)
		}
	})

	t.Run("tag filter", func(t *testing.T) {
		res := lvlComputeFleet(devices, groups, "platform", "db", false, false)
		if res.Summary.Total != 1 { // only bravo carries the db tag
			t.Errorf("tag=db total = %d, want 1", res.Summary.Total)
		}
	})
}
