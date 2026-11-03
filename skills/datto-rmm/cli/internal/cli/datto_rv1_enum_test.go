// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"testing"
	"time"
)

// Codex review r1 class "documented enum misread" (datto-rmm handfix
// fleet-posture-reads-the-documented-enums): Datto documents
// Antivirus.antivirusStatus as RunningAndUpToDate | RunningAndNotUpToDate |
// NotRunning | NotDetected and PatchManagement.patchStatus as NoPolicy |
// NoData | RebootRequired | InstallError | ApprovedPending | FullyPatched.
// `fleet av-gaps --status not-running` normalized to "not running" and never
// matched "NotRunning"; `fleet scorecard` counted a device whose patch status
// is NoData (zero counters: nothing known) as patched.
func TestFleetPostureReadsTheDocumentedEnums(t *testing.T) {
	devices := []fleetDevice{
		{UID: "d1", Hostname: "PC1", SiteName: "Acme", Antivirus: fleetAntivirus{AntivirusStatus: "NotRunning"}, PatchManagement: fleetPatch{PatchStatus: "NoData"}},
		{UID: "d2", Hostname: "PC2", SiteName: "Acme", Antivirus: fleetAntivirus{AntivirusStatus: "RunningAndUpToDate"}, PatchManagement: fleetPatch{PatchStatus: "FullyPatched"}},
		{UID: "d3", Hostname: "PC3", SiteName: "Acme", Antivirus: fleetAntivirus{AntivirusStatus: "NotDetected"}, PatchManagement: fleetPatch{PatchStatus: "InstallError"}},
		{UID: "d4", Hostname: "PC4", SiteName: "Acme", Antivirus: fleetAntivirus{AntivirusStatus: "RunningAndNotUpToDate"}, PatchManagement: fleetPatch{PatchStatus: "NoPolicy"}},
	}
	for filter, want := range map[string]string{
		"not-running": "PC1", "NotRunning": "PC1", "not running": "PC1", "not_running": "PC1",
		"not-detected": "PC3", "running-and-not-up-to-date": "PC4", "running-and-up-to-date": "PC2",
	} {
		got := computeAvGaps(devices, filter)
		if len(got) != 1 || got[0].Hostname != want {
			t.Errorf("av-gaps --status %q = %+v, want only %s", filter, got, want)
		}
	}
	if got := computeAvGaps(devices, ""); len(got) != 3 {
		t.Errorf("av-gaps with no filter = %d devices, want the 3 not RunningAndUpToDate", len(got))
	}
	if err := validateAvStatusFilter("stopped"); err == nil {
		t.Errorf("av-gaps --status accepted a value Datto never reports (it would match nothing and read as no gaps)")
	}
	card, ok := computeScorecard("Acme", devices, nil, nil, time.Now())
	if !ok {
		t.Fatal("scorecard: site not matched")
	}
	if card.PatchOkPct != 25 {
		t.Errorf("patchOkPct = %v, want 25 (only the FullyPatched device; NoData, InstallError and NoPolicy are not patched)", card.PatchOkPct)
	}
	if card.AvOkPct != 25 {
		t.Errorf("avOkPct = %v, want 25", card.AvOkPct)
	}
}
