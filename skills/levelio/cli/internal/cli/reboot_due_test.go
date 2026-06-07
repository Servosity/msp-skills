// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import "testing"

func TestLvlComputeRebootDue(t *testing.T) {
	devices := []lvlDevice{
		{ID: "dev1", Hostname: "alpha", Online: true, LastRebootTime: lvlFxAgo(100)},
		{ID: "dev2", Hostname: "bravo", Online: false, LastRebootTime: lvlFxAgo(1)},
		{ID: "dev3", Hostname: "charlie"}, // no reboot timestamp -> skipped
	}
	updates := []lvlUpdate{
		{ID: "u1", DeviceID: "dev1", InstalledOn: lvlFxAgo(50)},  // after reboot -> debt
		{ID: "u2", DeviceID: "dev1", InstalledOn: lvlFxAgo(72)},  // after reboot, oldest -> 3d wait
		{ID: "u3", DeviceID: "dev1", InstalledOn: lvlFxAgo(200)}, // before reboot -> not debt
		{ID: "u4", DeviceID: "dev2", InstalledOn: lvlFxAgo(5)},   // before dev2's reboot (1h ago)
		{ID: "u5", DeviceID: "dev3", InstalledOn: lvlFxAgo(2)},   // device skipped
	}
	res := lvlComputeRebootDue(devices, updates, 0, lvlFxNow())
	if res.DevicesChecked != 3 || res.Skipped != 1 {
		t.Fatalf("checked=%d skipped=%d, want 3/1", res.DevicesChecked, res.Skipped)
	}
	if len(res.Devices) != 1 {
		t.Fatalf("expected only alpha in debt, got %+v", res.Devices)
	}
	d := res.Devices[0]
	if d.Hostname != "alpha" || d.UpdatesSinceReboot != 2 {
		t.Fatalf("row = %+v, want alpha with 2 installs since reboot", d)
	}
	if d.WaitingDays != 3.0 {
		t.Errorf("waiting days = %v, want 3.0 (oldest install 72h ago)", d.WaitingDays)
	}
	// min-days filter excludes alpha when threshold above its wait.
	res = lvlComputeRebootDue(devices, updates, 4, lvlFxNow())
	if len(res.Devices) != 0 {
		t.Fatalf("min-days filter failed: %+v", res.Devices)
	}
}
