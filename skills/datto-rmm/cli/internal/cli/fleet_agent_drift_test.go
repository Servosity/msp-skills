// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"testing"
)

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"1.0.0", "1.0.0", 0},
		{"1.2.0", "1.10.0", -1}, // numeric, not lexical
		{"2.0", "1.9.9", 1},
		{"1.0", "1.0.0", 0}, // missing segment == 0
		{"1.0.1", "1.0", 1},
		{"4.5.0-beta", "4.5.0", 0}, // non-numeric tail -> 0
	}
	for _, tc := range tests {
		if got := compareVersions(tc.a, tc.b); got != tc.want {
			t.Fatalf("compareVersions(%q,%q) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestComputeAgentDrift(t *testing.T) {
	devices := []fleetDevice{
		{Hostname: "newest", SiteName: "A", CagVersion: "4.5.0"},
		{Hostname: "one-behind", SiteName: "A", CagVersion: "4.4.0"},
		{Hostname: "two-behind", SiteName: "B", CagVersion: "4.3.0"},
		{Hostname: "alsonewest", SiteName: "B", CagVersion: "4.5.0"},
		{Hostname: "noversion", SiteName: "C", CagVersion: ""},
	}
	// Distinct versions desc: 4.5.0(rank0), 4.4.0(rank1), 4.3.0(rank2).

	t.Run("behind 1 flags below max, sorted desc", func(t *testing.T) {
		got := computeAgentDrift(devices, 1)
		if len(got) != 2 {
			t.Fatalf("got %d, want 2: %+v", len(got), got)
		}
		if got[0].Hostname != "two-behind" || got[0].VersionsBehind != 2 {
			t.Fatalf("top = %+v, want two-behind/2", got[0])
		}
		if got[1].Hostname != "one-behind" || got[1].VersionsBehind != 1 {
			t.Fatalf("second = %+v, want one-behind/1", got[1])
		}
		if got[0].LatestVersion != "4.5.0" {
			t.Fatalf("LatestVersion = %q, want 4.5.0", got[0].LatestVersion)
		}
	})

	t.Run("behind 0 flags any below max (same set here)", func(t *testing.T) {
		got := computeAgentDrift(devices, 0)
		if len(got) != 2 {
			t.Fatalf("got %d, want 2", len(got))
		}
	})

	t.Run("behind 2 only the most-behind", func(t *testing.T) {
		got := computeAgentDrift(devices, 2)
		if len(got) != 1 || got[0].Hostname != "two-behind" {
			t.Fatalf("got %+v, want only two-behind", got)
		}
	})

	t.Run("all same version -> empty", func(t *testing.T) {
		same := []fleetDevice{
			{Hostname: "a", CagVersion: "4.5.0"},
			{Hostname: "b", CagVersion: "4.5.0"},
		}
		got := computeAgentDrift(same, 1)
		if len(got) != 0 {
			t.Fatalf("got %d, want 0", len(got))
		}
	})

	t.Run("no versions -> empty", func(t *testing.T) {
		got := computeAgentDrift([]fleetDevice{{Hostname: "a", CagVersion: ""}}, 1)
		if len(got) != 0 {
			t.Fatalf("got %d, want 0", len(got))
		}
	})
}
