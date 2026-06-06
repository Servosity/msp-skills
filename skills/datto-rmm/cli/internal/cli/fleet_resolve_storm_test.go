// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"testing"
	"time"
)

func mkAlert(uid, deviceUID, deviceName, site string, resolved bool, ts string) fleetAlert {
	return fleetAlert{
		AlertUID:  uid,
		Resolved:  resolved,
		Timestamp: rawJSON(`"` + ts + `"`),
		AlertSourceInfo: fleetAlertSource{
			DeviceUID:  deviceUID,
			DeviceName: deviceName,
			SiteName:   site,
		},
	}
}

func TestComputeResolvePlan(t *testing.T) {
	now := time.Date(2026, 6, 5, 0, 0, 0, 0, time.UTC)
	alerts := []fleetAlert{
		mkAlert("a1", "dev1", "DC01", "Acme", false, "2026-06-04T00:00:00Z"),
		mkAlert("a2", "dev1", "DC01", "Acme", false, "2026-06-03T00:00:00Z"),
		mkAlert("a3", "dev1", "DC01", "Acme", false, "2026-06-02T00:00:00Z"),
		mkAlert("a4", "dev2", "WS02", "Beta", false, "2026-06-04T00:00:00Z"),
		mkAlert("a5", "dev1", "DC01", "Acme", true, "2026-06-04T00:00:00Z"),  // resolved: never counted
		mkAlert("a6", "dev1", "DC01", "Acme", false, "2026-01-01T00:00:00Z"), // outside window
	}

	tests := []struct {
		name        string
		days        int
		top         int
		minCount    int
		wantDevices int
		wantTotal   int
		wantFirst   string
	}{
		{"min-count filters singletons", 7, 10, 2, 1, 3, "dev1"},
		{"min-count 1 includes both", 7, 10, 1, 2, 4, "dev1"},
		{"top 1 keeps noisiest", 7, 1, 1, 1, 3, "dev1"},
		{"window 0 counts everything open", 0, 10, 1, 2, 5, "dev1"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			plan := computeResolvePlan(alerts, tc.days, tc.top, tc.minCount, now)
			if len(plan.Devices) != tc.wantDevices {
				t.Fatalf("devices = %d, want %d", len(plan.Devices), tc.wantDevices)
			}
			if plan.TotalAlerts != tc.wantTotal {
				t.Fatalf("total = %d, want %d", plan.TotalAlerts, tc.wantTotal)
			}
			if plan.Devices[0].DeviceUID != tc.wantFirst {
				t.Fatalf("first = %s, want %s", plan.Devices[0].DeviceUID, tc.wantFirst)
			}
		})
	}
}

func TestComputeResolvePlanCollectsAlertUIDs(t *testing.T) {
	now := time.Date(2026, 6, 5, 0, 0, 0, 0, time.UTC)
	alerts := []fleetAlert{
		mkAlert("a2", "dev1", "DC01", "Acme", false, "2026-06-03T00:00:00Z"),
		mkAlert("a1", "dev1", "DC01", "Acme", false, "2026-06-04T00:00:00Z"),
	}
	plan := computeResolvePlan(alerts, 7, 10, 1, now)
	if len(plan.Devices) != 1 {
		t.Fatalf("devices = %d, want 1", len(plan.Devices))
	}
	got := plan.Devices[0].AlertUIDs
	if len(got) != 2 || got[0] != "a1" || got[1] != "a2" {
		t.Fatalf("alertUids = %v, want sorted [a1 a2]", got)
	}
}
