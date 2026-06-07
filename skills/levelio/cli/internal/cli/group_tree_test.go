// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import "testing"

func TestLvlComputeGroupTree(t *testing.T) {
	devices := lvlFxDevices()
	groups := lvlFxGroups()
	alerts := lvlFxAlerts()
	now := lvlFxNow()

	t.Run("structure and rollup", func(t *testing.T) {
		res := lvlComputeGroupTree(devices, groups, nil, map[string]bool{}, 7, "", now)
		if res.UngroupedDevices != 1 { // dev4 has no group
			t.Fatalf("ungrouped = %d, want 1", res.UngroupedDevices)
		}
		if len(res.Roots) != 1 || res.Roots[0].Name != "Root" {
			t.Fatalf("roots = %+v, want single Root", res.Roots)
		}
		root := res.Roots[0]
		if root.DirectDevices != 2 || root.TotalDevices != 3 { // dev1+dev3 direct; +dev2 in child
			t.Errorf("Root direct/total = %d/%d, want 2/3", root.DirectDevices, root.TotalDevices)
		}
		if len(root.Children) != 1 || root.Children[0].TotalDevices != 1 {
			t.Errorf("child = %+v, want Child total1", root.Children)
		}
	})

	t.Run("with alerts stale score", func(t *testing.T) {
		with := map[string]bool{"alerts": true, "stale": true, "score": true}
		res := lvlComputeGroupTree(devices, groups, alerts, with, 7, "", now)
		root := res.Roots[0]
		if root.OpenAlerts == nil || *root.OpenAlerts != 2 { // dev1 has al1+al2 open
			t.Errorf("Root open alerts = %v, want 2", root.OpenAlerts)
		}
		if root.Stale == nil || *root.Stale != 1 { // bravo (dev2) in child subtree is 10d dark
			t.Errorf("Root rolled stale = %v, want 1", root.Stale)
		}
		if root.AvgScore == nil || *root.AvgScore != 65 { // (40 + 90) / 2
			t.Errorf("Root rolled avg score = %v, want 65", root.AvgScore)
		}
	})
}

func TestLvlComputeGroupTreeCycleOrphans(t *testing.T) {
	// Corrupt parent_id data forming a cycle must not silently drop the
	// groups (regression: cyclic components selected no root at all).
	groups := []lvlGroup{
		{ID: "c1", Name: "CycleA", ParentID: "c2"},
		{ID: "c2", Name: "CycleB", ParentID: "c1"},
		{ID: "g1", Name: "Root", ParentID: ""},
	}
	devices := []lvlDevice{
		{ID: "d1", Hostname: "alpha", GroupID: "c1"},
		{ID: "d2", Hostname: "bravo", GroupID: "g1"},
	}
	res := lvlComputeGroupTree(devices, groups, nil, map[string]bool{}, 14, "", lvlFxNow())
	seen := map[string]bool{}
	var walk func(nodes []*groupNode)
	walk = func(nodes []*groupNode) {
		for _, n := range nodes {
			seen[n.ID] = true
			walk(n.Children)
		}
	}
	walk(res.Roots)
	for _, id := range []string{"g1", "c1", "c2"} {
		if !seen[id] {
			t.Fatalf("group %s missing from tree output: roots=%+v", id, res.Roots)
		}
	}
}

func TestLvlComputeGroupTreeSelfParent(t *testing.T) {
	// A self-parented group (g.parent_id == g.id) must terminate and emit
	// the node exactly once.
	groups := []lvlGroup{{ID: "s1", Name: "SelfLoop", ParentID: "s1"}}
	devices := []lvlDevice{{ID: "d1", Hostname: "alpha", GroupID: "s1"}}
	res := lvlComputeGroupTree(devices, groups, nil, map[string]bool{}, 14, "", lvlFxNow())
	count := 0
	var walk func(nodes []*groupNode)
	walk = func(nodes []*groupNode) {
		for _, n := range nodes {
			if n.ID == "s1" {
				count++
			}
			walk(n.Children)
		}
	}
	walk(res.Roots)
	if count != 1 {
		t.Fatalf("self-parent group emitted %d times, want exactly 1: %+v", count, res.Roots)
	}
}
