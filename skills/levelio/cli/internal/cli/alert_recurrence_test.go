// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import "testing"

func TestLvlComputeAlertRecurrence(t *testing.T) {
	alerts := append(lvlFxAlerts(),
		lvlAlert{ID: "al5", DeviceID: "dev2", Severity: "critical", IsResolved: false, StartedAt: lvlFxAgo(5), Name: "High CPU"},
		lvlAlert{ID: "al6", DeviceID: "dev3", Severity: "warning", IsResolved: true, StartedAt: lvlFxAgo(10), ResolvedAt: lvlFxAgo(9), Name: "High CPU"},
	)
	res := lvlComputeAlertRecurrence(alerts, 0, 15, lvlFxNow())
	if res.TotalAlerts != 6 {
		t.Fatalf("total alerts = %d, want 6", res.TotalAlerts)
	}
	top := res.Monitors[0]
	if top.Name != "High CPU" || top.Count != 3 || top.Devices != 3 {
		t.Fatalf("top monitor = %+v, want High CPU count=3 devices=3", top)
	}
	if top.Unresolved != 2 {
		t.Errorf("High CPU unresolved = %d, want 2 (al1, al5)", top.Unresolved)
	}
}

func TestLvlComputeAlertRecurrenceWindowAndTop(t *testing.T) {
	// 24h window keeps only al1 (2h) — al2 (30h), al3 (50h), al4 (200h) fall out.
	res := lvlComputeAlertRecurrence(lvlFxAlerts(), 1, 15, lvlFxNow())
	if res.TotalAlerts != 1 || len(res.Monitors) != 1 || res.Monitors[0].Name != "High CPU" {
		t.Fatalf("window result = %+v, want only High CPU", res)
	}
	// top=1 caps the unwindowed 4-monitor list.
	res = lvlComputeAlertRecurrence(lvlFxAlerts(), 0, 1, lvlFxNow())
	if len(res.Monitors) != 1 {
		t.Fatalf("top cap failed: %d monitors", len(res.Monitors))
	}
}
