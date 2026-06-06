// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"testing"
	"time"
)

func mkDevice(uid, host, site string, online, deleted bool) fleetDevice {
	return fleetDevice{UID: uid, Hostname: host, SiteName: site, Online: online, Deleted: deleted}
}

func TestComputeFleetDiff(t *testing.T) {
	now := time.Date(2026, 6, 5, 0, 0, 0, 0, time.UTC)
	from := []fleetDevice{
		mkDevice("d1", "alpha", "Acme", true, false),
		mkDevice("d2", "bravo", "Acme", false, false),
		mkDevice("d9", "ghost", "Acme", false, true), // deleted: excluded
	}
	to := []fleetDevice{
		mkDevice("d1", "alpha", "Acme", true, false),
		mkDevice("d3", "charlie", "Beta", true, false),
	}

	added, removed, fromTotals, toTotals := computeFleetDiff(from, to, now)
	if len(added) != 1 || added[0].UID != "d3" {
		t.Fatalf("added = %+v, want [d3]", added)
	}
	if len(removed) != 1 || removed[0].UID != "d2" {
		t.Fatalf("removed = %+v, want [d2]", removed)
	}
	if fromTotals.Devices != 2 || toTotals.Devices != 2 {
		t.Fatalf("totals wrong: from=%+v to=%+v", fromTotals, toTotals)
	}
	if fromTotals.Online != 1 || toTotals.Online != 2 {
		t.Fatalf("online wrong: from=%d to=%d", fromTotals.Online, toTotals.Online)
	}
}

func TestComputeFleetDiffEmptySides(t *testing.T) {
	now := time.Date(2026, 6, 5, 0, 0, 0, 0, time.UTC)
	added, removed, fromTotals, toTotals := computeFleetDiff(nil, nil, now)
	// No fabricated drift from two empty states.
	if len(added) != 0 || len(removed) != 0 {
		t.Fatalf("empty diff fabricated changes: added=%v removed=%v", added, removed)
	}
	if fromTotals.Devices != 0 || toTotals.Devices != 0 {
		t.Fatalf("empty totals wrong")
	}
}

func TestComputePostureTotals(t *testing.T) {
	now := time.Date(2026, 6, 5, 0, 0, 0, 0, time.UTC)
	devices := []fleetDevice{
		{
			UID: "d1", Hostname: "alpha", Online: true,
			Antivirus:       fleetAntivirus{AntivirusStatus: "NotRunning"},
			PatchManagement: fleetPatch{PatchesApprovedPending: 4},
			WarrantyDate:    "2025-01-01",
		},
		{
			UID: "d2", Hostname: "bravo", Online: false,
			Antivirus:       fleetAntivirus{AntivirusStatus: "RunningAndUpToDate"},
			PatchManagement: fleetPatch{PatchesApprovedPending: 0},
			WarrantyDate:    "2030-01-01",
		},
	}
	totals := computePostureTotals(devices, now)
	if totals.Devices != 2 || totals.Online != 1 {
		t.Fatalf("device/online totals wrong: %+v", totals)
	}
	if totals.AVNotRunning != 1 {
		t.Fatalf("avNotRunning = %d, want 1", totals.AVNotRunning)
	}
	if totals.PatchesMissing != 4 {
		t.Fatalf("patchesMissing = %d, want 4", totals.PatchesMissing)
	}
	if totals.WarrantyExpired != 1 {
		t.Fatalf("warrantyExpired = %d, want 1", totals.WarrantyExpired)
	}
}
