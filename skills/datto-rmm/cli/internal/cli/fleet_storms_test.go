// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"testing"
	"time"
)

func TestComputeStorms(t *testing.T) {
	now := time.Date(2026, 5, 29, 0, 0, 0, 0, time.UTC)
	recent := rawJSON(`"2026-05-28T00:00:00Z"`)
	old := rawJSON(`"2026-01-01T00:00:00Z"`)

	alerts := []fleetAlert{
		{AlertSourceInfo: fleetAlertSource{DeviceUID: "u1", DeviceName: "dev1", SiteName: "A"}, Timestamp: recent},
		{AlertSourceInfo: fleetAlertSource{DeviceUID: "u1", DeviceName: "dev1", SiteName: "A"}, Timestamp: recent},
		{AlertSourceInfo: fleetAlertSource{DeviceUID: "u1", DeviceName: "dev1", SiteName: "A"}, Timestamp: recent},
		{AlertSourceInfo: fleetAlertSource{DeviceUID: "u2", DeviceName: "dev2", SiteName: "B"}, Timestamp: recent},
		{AlertSourceInfo: fleetAlertSource{DeviceUID: "u2", DeviceName: "dev2", SiteName: "B"}, Timestamp: old},             // outside window
		{AlertSourceInfo: fleetAlertSource{DeviceUID: "u3", DeviceName: "dev3", SiteName: "C"}, Timestamp: rawJSON("null")}, // unparseable -> counted
	}

	t.Run("7d window, counts and order", func(t *testing.T) {
		got := computeStorms(alerts, 7, 0, now)
		if len(got) != 3 {
			t.Fatalf("got %d groups, want 3: %+v", len(got), got)
		}
		if got[0].DeviceName != "dev1" || got[0].AlertCount != 3 {
			t.Fatalf("top = %+v, want dev1 count 3", got[0])
		}
		// dev2 has 1 in-window (old dropped); dev3 has 1 (unparseable counted).
		if got[1].AlertCount != 1 || got[2].AlertCount != 1 {
			t.Fatalf("tail counts = %d,%d want 1,1", got[1].AlertCount, got[2].AlertCount)
		}
	})

	t.Run("top N truncates", func(t *testing.T) {
		got := computeStorms(alerts, 7, 1, now)
		if len(got) != 1 || got[0].DeviceName != "dev1" {
			t.Fatalf("top1 = %+v, want only dev1", got)
		}
	})

	t.Run("empty input -> empty", func(t *testing.T) {
		got := computeStorms(nil, 7, 0, now)
		if len(got) != 0 {
			t.Fatalf("got %d, want 0", len(got))
		}
	})
}
