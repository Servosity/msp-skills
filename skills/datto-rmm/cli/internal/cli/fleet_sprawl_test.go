// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import "testing"

func TestComputeSprawl(t *testing.T) {
	software := map[string][]fleetSoftware{
		"d1": {{Name: "Chrome", Version: "120"}, {Name: "Slack", Version: "5"}},
		"d2": {{Name: "Chrome", Version: "120"}, {Name: "Chrome", Version: "119"}},
		"d3": {{Name: "Chrome", Version: "120"}},
	}

	t.Run("aggregate counts and order", func(t *testing.T) {
		got := computeSprawl(software, "")
		// Chrome/120 -> 3 installs / 3 devices, then Chrome/119 (1/1), Slack/5 (1/1) by name.
		if len(got) != 3 {
			t.Fatalf("got %d rows, want 3: %+v", len(got), got)
		}
		if got[0].Name != "Chrome" || got[0].Version != "120" {
			t.Fatalf("top = %+v, want Chrome/120", got[0])
		}
		if got[0].InstallCount != 3 || got[0].DeviceCount != 3 {
			t.Fatalf("Chrome/120 = installs %d devices %d, want 3/3", got[0].InstallCount, got[0].DeviceCount)
		}
		// Tie-break: Chrome/119 before Slack/5 (both count 1) by name asc.
		if got[1].Name != "Chrome" || got[1].Version != "119" {
			t.Fatalf("row1 = %+v, want Chrome/119", got[1])
		}
		if got[2].Name != "Slack" {
			t.Fatalf("row2 = %+v, want Slack", got[2])
		}
	})

	t.Run("name filter case-insensitive substring", func(t *testing.T) {
		got := computeSprawl(software, "chro")
		for _, v := range got {
			if v.Name != "Chrome" {
				t.Fatalf("filter leaked %+v", v)
			}
		}
		if len(got) != 2 {
			t.Fatalf("got %d, want 2 chrome rows", len(got))
		}
	})

	t.Run("filter excludes everything -> empty", func(t *testing.T) {
		got := computeSprawl(software, "nonexistent")
		if len(got) != 0 {
			t.Fatalf("got %d, want 0", len(got))
		}
	})

	t.Run("empty input -> empty", func(t *testing.T) {
		got := computeSprawl(map[string][]fleetSoftware{}, "")
		if len(got) != 0 {
			t.Fatalf("got %d, want 0", len(got))
		}
	})
}
