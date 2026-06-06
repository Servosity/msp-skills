// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import "testing"

func TestComputeAvGaps(t *testing.T) {
	devices := []fleetDevice{
		{Hostname: "good", SiteName: "A", Antivirus: fleetAntivirus{AntivirusProduct: "Defender", AntivirusStatus: "Running"}},
		{Hostname: "stopped", SiteName: "A", Antivirus: fleetAntivirus{AntivirusProduct: "Defender", AntivirusStatus: "Not Running"}},
		{Hostname: "none", SiteName: "B", Antivirus: fleetAntivirus{}},
		{Hostname: "deleted", SiteName: "B", Deleted: true, Antivirus: fleetAntivirus{AntivirusStatus: "Not Running"}},
	}

	t.Run("default flags anything not running, sorted site/host", func(t *testing.T) {
		got := computeAvGaps(devices, "")
		if len(got) != 2 {
			t.Fatalf("got %d, want 2: %+v", len(got), got)
		}
		// Site A "stopped" before Site B "none".
		if got[0].Hostname != "stopped" || got[1].Hostname != "none" {
			t.Fatalf("order = %q,%q want stopped,none", got[0].Hostname, got[1].Hostname)
		}
		if got[0].Antivirus.AntivirusStatus != "Not Running" {
			t.Fatalf("nested AV not preserved: %+v", got[0].Antivirus)
		}
	})

	t.Run("status filter normalizes hyphen/space", func(t *testing.T) {
		got := computeAvGaps(devices, "not-running")
		if len(got) != 1 || got[0].Hostname != "stopped" {
			t.Fatalf("got %+v, want only stopped", got)
		}
	})

	t.Run("status filter no match -> empty", func(t *testing.T) {
		got := computeAvGaps(devices, "quarantined")
		if len(got) != 0 {
			t.Fatalf("got %d, want 0", len(got))
		}
	})

	t.Run("all running -> empty under default", func(t *testing.T) {
		all := []fleetDevice{{Hostname: "a", Antivirus: fleetAntivirus{AntivirusStatus: "running"}}}
		got := computeAvGaps(all, "")
		if len(got) != 0 {
			t.Fatalf("got %d, want 0", len(got))
		}
	})
}
