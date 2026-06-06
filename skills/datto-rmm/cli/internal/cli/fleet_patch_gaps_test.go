// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import "testing"

func TestComputePatchGaps(t *testing.T) {
	devices := []fleetDevice{
		{Hostname: "behind", SiteName: "A", PatchManagement: fleetPatch{PatchesApprovedPending: 5, PatchesNotApproved: 3, PatchesInstalled: 100, PatchStatus: "Reboot Required"}}, // 8
		{Hostname: "ok", SiteName: "A", PatchManagement: fleetPatch{PatchesApprovedPending: 0, PatchesNotApproved: 0, PatchesInstalled: 200, PatchStatus: "Fully Patched"}},       // 0
		{Hostname: "mid", SiteName: "B", PatchManagement: fleetPatch{PatchesApprovedPending: 2, PatchesNotApproved: 0, PatchStatus: "Pending"}},                                   // 2
		{Hostname: "deleted", SiteName: "B", Deleted: true, PatchManagement: fleetPatch{PatchesApprovedPending: 99}},
	}

	t.Run("min-missing 1, sorted by MissingCount desc", func(t *testing.T) {
		got := computePatchGaps(devices, 1)
		if len(got) != 2 {
			t.Fatalf("got %d, want 2: %+v", len(got), got)
		}
		if got[0].Hostname != "behind" || got[0].MissingCount != 8 {
			t.Fatalf("top = %+v, want behind/8", got[0])
		}
		if got[1].Hostname != "mid" || got[1].MissingCount != 2 {
			t.Fatalf("second = %+v, want mid/2", got[1])
		}
		if got[0].Installed != 100 || got[0].PatchStatus != "Reboot Required" {
			t.Fatalf("fields = %+v", got[0])
		}
	})

	t.Run("min-missing 5 excludes mid", func(t *testing.T) {
		got := computePatchGaps(devices, 5)
		if len(got) != 1 || got[0].Hostname != "behind" {
			t.Fatalf("got %+v, want only behind", got)
		}
	})

	t.Run("high threshold -> empty", func(t *testing.T) {
		got := computePatchGaps(devices, 1000)
		if len(got) != 0 {
			t.Fatalf("got %d, want 0", len(got))
		}
	})
}
