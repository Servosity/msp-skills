// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Seeded-store behavioral test for fleet health.

package cli

import (
	"encoding/json"
	"testing"
)

func TestFleetHealthCommand(t *testing.T) {
	dbPath := seedFleetStore(t)
	flags := &rootFlags{asJSON: true}
	out := runFleetRaw(t, newNovelFleetHealthCmd(flags), dbPath)

	var report fleetHealthReport
	if err := json.Unmarshal(out, &report); err != nil {
		t.Fatalf("decode health report: %v\noutput: %s", err, out)
	}

	if report.AgentsTotal != 2 {
		t.Fatalf("agents_total = %d, want 2", report.AgentsTotal)
	}
	if report.AgentsOnline != 1 || report.AgentsOffline != 1 {
		t.Fatalf("agents online/offline = %d/%d, want 1/1", report.AgentsOnline, report.AgentsOffline)
	}
	if report.DevicesTotal != 3 {
		t.Fatalf("devices_total = %d, want 3", report.DevicesTotal)
	}
	if report.DevicesOffline != 2 {
		t.Fatalf("devices_offline = %d, want 2 (OFFLINE + DOWN)", report.DevicesOffline)
	}
	if len(report.Sites) != 2 {
		t.Fatalf("sites = %d, want 2", len(report.Sites))
	}
	// Sites are sorted by offline-device count descending. Beta Site (Edge
	// Router DOWN) and Acme HQ (Lobby Cam OFFLINE) each have 1 — ordering is by
	// count then name, so Acme HQ leads on the tie.
	if report.SitesWithOfflineDevices != 2 {
		t.Fatalf("sites_with_offline_devices = %d, want 2", report.SitesWithOfflineDevices)
	}
}
