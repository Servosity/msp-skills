// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import "testing"

func TestLvlComputeAlertTriage(t *testing.T) {
	alerts := lvlFxAlerts()
	devices := lvlFxDevices()
	groups := lvlFxGroups()

	t.Run("by group, unresolved only", func(t *testing.T) {
		res := lvlComputeAlertTriage(alerts, devices, groups, "", "group", false)
		if res.TotalAlerts != 3 { // al1, al2 (dev1->Root), al4 (dev4->no group); al3 resolved
			t.Fatalf("total = %d, want 3", res.TotalAlerts)
		}
		if res.Clusters[0].Key != "Root" || res.Clusters[0].Count != 2 {
			t.Errorf("top cluster = %+v, want Root=2", res.Clusters[0])
		}
		if res.Clusters[0].BySeverity["critical"] != 1 || res.Clusters[0].BySeverity["warning"] != 1 {
			t.Errorf("Root severity breakdown = %v, want critical1 warning1", res.Clusters[0].BySeverity)
		}
	})

	t.Run("severity filter", func(t *testing.T) {
		res := lvlComputeAlertTriage(alerts, devices, groups, "critical", "group", false)
		if res.TotalAlerts != 1 || res.Clusters[0].Key != "Root" {
			t.Errorf("critical-only = %+v, want 1 in Root", res)
		}
	})

	t.Run("include resolved", func(t *testing.T) {
		res := lvlComputeAlertTriage(alerts, devices, groups, "", "severity", true)
		if res.TotalAlerts != 4 {
			t.Errorf("with resolved total = %d, want 4", res.TotalAlerts)
		}
	})

	t.Run("by severity", func(t *testing.T) {
		res := lvlComputeAlertTriage(alerts, devices, groups, "", "severity", false)
		got := map[string]int{}
		for _, c := range res.Clusters {
			got[c.Key] = c.Count
		}
		if got["critical"] != 1 || got["warning"] != 1 || got["information"] != 1 {
			t.Errorf("by severity = %v", got)
		}
	})
}

func TestLvlComputeAlertTriageInfoAlias(t *testing.T) {
	// "--severity info" must behave identically to "--severity information"
	// (regression: the alias validated but the filter compared exact strings
	// against the stored "information" and silently matched nothing).
	a := lvlComputeAlertTriage(lvlFxAlerts(), lvlFxDevices(), lvlFxGroups(), "info", "group", false)
	b := lvlComputeAlertTriage(lvlFxAlerts(), lvlFxDevices(), lvlFxGroups(), "information", "group", false)
	if a.TotalAlerts == 0 {
		t.Fatalf("--severity info matched zero alerts; alias not normalized")
	}
	if a.TotalAlerts != b.TotalAlerts {
		t.Fatalf("alias mismatch: info=%d information=%d", a.TotalAlerts, b.TotalAlerts)
	}
}
