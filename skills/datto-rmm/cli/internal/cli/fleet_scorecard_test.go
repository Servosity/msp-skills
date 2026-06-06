// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"testing"
	"time"
)

func TestComputeScorecard(t *testing.T) {
	now := time.Date(2026, 5, 29, 0, 0, 0, 0, time.UTC)

	devices := []fleetDevice{
		{Hostname: "a1", SiteName: "Acme", SiteUID: "site-acme", Online: true, CagVersion: "4.5.0",
			Antivirus:       fleetAntivirus{AntivirusStatus: "Running"},
			PatchManagement: fleetPatch{PatchesApprovedPending: 0, PatchesNotApproved: 0},
			WarrantyDate:    "2026-06-15"}, // within 90
		{Hostname: "a2", SiteName: "Acme", SiteUID: "site-acme", Online: false, CagVersion: "4.4.0",
			Antivirus:       fleetAntivirus{AntivirusStatus: "Not Running"},
			PatchManagement: fleetPatch{PatchesApprovedPending: 3},
			WarrantyDate:    "2030-01-01"}, // far out
		{Hostname: "b1", SiteName: "Beta", SiteUID: "site-beta", Online: true, CagVersion: "4.5.0",
			Antivirus: fleetAntivirus{AntivirusStatus: "Running"}},
	}
	alerts := []fleetAlert{
		{Resolved: false, AlertSourceInfo: fleetAlertSource{SiteUID: "site-acme", SiteName: "Acme"}},
		{Resolved: true, AlertSourceInfo: fleetAlertSource{SiteUID: "site-acme", SiteName: "Acme"}}, // resolved, not counted
		{Resolved: false, AlertSourceInfo: fleetAlertSource{SiteUID: "site-beta", SiteName: "Beta"}},
	}
	sites := []fleetSite{
		{UID: "site-acme", Name: "Acme"},
		{UID: "site-beta", Name: "Beta"},
		{UID: "site-empty", Name: "Empty Co"},
	}

	t.Run("match by name (case-insensitive)", func(t *testing.T) {
		card, ok := computeScorecard("acme", devices, alerts, sites, now)
		if !ok {
			t.Fatal("expected match")
		}
		if card.DeviceCount != 2 {
			t.Fatalf("DeviceCount = %d, want 2", card.DeviceCount)
		}
		if card.OnlineCount != 1 {
			t.Fatalf("OnlineCount = %d, want 1", card.OnlineCount)
		}
		if card.OpenAlerts != 1 {
			t.Fatalf("OpenAlerts = %d, want 1 (resolved excluded)", card.OpenAlerts)
		}
		if card.PatchOkPct != 50.0 {
			t.Fatalf("PatchOkPct = %v, want 50", card.PatchOkPct)
		}
		if card.AvOkPct != 50.0 {
			t.Fatalf("AvOkPct = %v, want 50", card.AvOkPct)
		}
		if card.WarrantyExpiring90 != 1 {
			t.Fatalf("WarrantyExpiring90 = %d, want 1", card.WarrantyExpiring90)
		}
		// Fleet max version is 4.5.0; a2 (4.4.0) is behind.
		if card.AgentsBehind != 1 {
			t.Fatalf("AgentsBehind = %d, want 1", card.AgentsBehind)
		}
		if card.SiteUID != "site-acme" {
			t.Fatalf("SiteUID = %q", card.SiteUID)
		}
	})

	t.Run("match by uid", func(t *testing.T) {
		card, ok := computeScorecard("site-beta", devices, alerts, sites, now)
		if !ok || card.DeviceCount != 1 || card.Site != "Beta" {
			t.Fatalf("beta = %+v ok=%v", card, ok)
		}
	})

	t.Run("site with no devices still matches via sites table", func(t *testing.T) {
		card, ok := computeScorecard("Empty Co", devices, alerts, sites, now)
		if !ok {
			t.Fatal("expected match for device-less site")
		}
		if card.DeviceCount != 0 {
			t.Fatalf("DeviceCount = %d, want 0", card.DeviceCount)
		}
	})

	t.Run("unknown site -> not found", func(t *testing.T) {
		_, ok := computeScorecard("Nope Inc", devices, alerts, sites, now)
		if ok {
			t.Fatal("expected not found")
		}
	})
}
