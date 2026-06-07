// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Table-driven tests for the drift snapshot differ.

package cli

import "testing"

func TestDiffDriftStates(t *testing.T) {
	prior := map[string]driftDeviceState{
		"1": {DisplayName: "Switch", Status: "ONLINE"},
		"2": {DisplayName: "Camera", Status: "ONLINE"},
		"3": {DisplayName: "Printer", Status: "ONLINE"},
	}
	current := map[string]driftDeviceState{
		"1": {DisplayName: "Switch", Status: "ONLINE"},  // unchanged
		"2": {DisplayName: "Camera", Status: "OFFLINE"}, // status changed
		// "3" removed
		"4": {DisplayName: "New AP", Status: "ONLINE"}, // added
	}

	var report driftReport
	report.Added = []driftChange{}
	report.Removed = []driftChange{}
	report.StatusChanged = []driftChange{}
	diffDriftStates(prior, current, &report)

	if len(report.Added) != 1 || report.Added[0].DeviceID != "4" {
		t.Fatalf("added = %+v, want device 4", report.Added)
	}
	if len(report.Removed) != 1 || report.Removed[0].DeviceID != "3" {
		t.Fatalf("removed = %+v, want device 3", report.Removed)
	}
	if len(report.StatusChanged) != 1 || report.StatusChanged[0].DeviceID != "2" ||
		report.StatusChanged[0].From != "ONLINE" || report.StatusChanged[0].To != "OFFLINE" {
		t.Fatalf("status_changed = %+v, want device 2 ONLINE->OFFLINE", report.StatusChanged)
	}
}

func TestDiffDriftStatesNoChange(t *testing.T) {
	state := map[string]driftDeviceState{"1": {Status: "ONLINE"}}
	var report driftReport
	report.Added = []driftChange{}
	report.Removed = []driftChange{}
	report.StatusChanged = []driftChange{}
	diffDriftStates(state, state, &report)
	if len(report.Added)+len(report.Removed)+len(report.StatusChanged) != 0 {
		t.Fatalf("identical states should produce no drift, got %+v", report)
	}
}
