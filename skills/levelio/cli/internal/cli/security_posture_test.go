// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import "testing"

func TestLvlComputeSecurityPosture(t *testing.T) {
	devices := lvlFxDevices()
	groups := lvlFxGroups()

	t.Run("distribution and below", func(t *testing.T) {
		res := lvlComputeSecurityPosture(devices, groups, 70, false)
		if res.CountScored != 3 || res.CountUnscored != 1 {
			t.Fatalf("scored/unscored = %d/%d, want 3/1", res.CountScored, res.CountUnscored)
		}
		if res.Avg != 65 || res.Min != 40 || res.Max != 90 {
			t.Errorf("avg/min/max = %.1f/%d/%d, want 65/40/90", res.Avg, res.Min, res.Max)
		}
		if res.BelowCount != 2 || res.Below[0].Score != 40 {
			t.Errorf("below = %+v, want 2 with 40 first", res.Below)
		}
		dist := map[string]int{}
		for _, b := range res.Distribution {
			dist[b.Range] = b.Count
		}
		if dist["0-49"] != 1 || dist["50-69"] != 1 || dist["90-100"] != 1 {
			t.Errorf("distribution = %v", dist)
		}
	})

	t.Run("by group", func(t *testing.T) {
		res := lvlComputeSecurityPosture(devices, groups, 70, true)
		var root, child *secGroupRollup
		for i := range res.ByGroup {
			switch res.ByGroup[i].Group {
			case "Root":
				root = &res.ByGroup[i]
			case "Child":
				child = &res.ByGroup[i]
			}
		}
		if root == nil || root.CountScored != 1 || root.Avg != 40 { // dev1 scored 40; dev3 unscored
			t.Errorf("Root rollup = %+v, want scored1 avg40", root)
		}
		if child == nil || child.Avg != 90 {
			t.Errorf("Child rollup = %+v, want avg90", child)
		}
	})
}
