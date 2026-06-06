// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"testing"
	"time"
)

func TestComputeWarranty(t *testing.T) {
	now := time.Date(2026, 5, 29, 0, 0, 0, 0, time.UTC)

	devices := []fleetDevice{
		{Hostname: "expired", SiteName: "A", WarrantyDate: "2026-05-01", DeviceType: fleetDeviceType{Type: "Desktop"}}, // -28d
		{Hostname: "soon", SiteName: "A", WarrantyDate: "2026-06-15", DeviceType: fleetDeviceType{Type: "Laptop"}},     // +17d
		{Hostname: "far", SiteName: "B", WarrantyDate: "2027-01-01"},                                                   // +217d
		{Hostname: "nowarranty", SiteName: "B", WarrantyDate: ""},
	}

	t.Run("within 60 includes expired+soon, sorted by DaysLeft asc", func(t *testing.T) {
		got := computeWarranty(devices, 60, now)
		if len(got) != 2 {
			t.Fatalf("got %d, want 2: %+v", len(got), got)
		}
		if got[0].Hostname != "expired" {
			t.Fatalf("first = %q, want expired (most negative)", got[0].Hostname)
		}
		if got[0].DaysLeft >= 0 {
			t.Fatalf("expired DaysLeft = %d, want negative", got[0].DaysLeft)
		}
		if got[1].Hostname != "soon" || got[1].DaysLeft <= 0 {
			t.Fatalf("second = %+v, want soon with positive days", got[1])
		}
		if got[1].DeviceType != "Laptop" {
			t.Fatalf("DeviceType = %q, want Laptop", got[1].DeviceType)
		}
	})

	t.Run("within 0 excludes future warranties", func(t *testing.T) {
		got := computeWarranty(devices, 0, now)
		if len(got) != 1 || got[0].Hostname != "expired" {
			t.Fatalf("got %+v, want only expired", got)
		}
	})

	t.Run("no parseable warranties -> empty", func(t *testing.T) {
		got := computeWarranty([]fleetDevice{{Hostname: "x", WarrantyDate: ""}}, 60, now)
		if len(got) != 0 {
			t.Fatalf("got %d, want 0", len(got))
		}
	})
}
