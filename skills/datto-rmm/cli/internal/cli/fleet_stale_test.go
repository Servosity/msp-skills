// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"testing"
	"time"
)

func TestComputeStale(t *testing.T) {
	now := time.Date(2026, 5, 29, 0, 0, 0, 0, time.UTC)

	devices := []fleetDevice{
		{Hostname: "old1", SiteName: "A", LastSeen: rawJSON(`"2026-04-01T00:00:00Z"`)},  // ~58d
		{Hostname: "fresh", SiteName: "A", LastSeen: rawJSON(`"2026-05-28T00:00:00Z"`)}, // 1d
		{Hostname: "old2", SiteName: "B", LastSeen: rawJSON(`"2026-03-01T00:00:00Z"`)},  // ~89d
		{Hostname: "del", SiteName: "B", LastSeen: rawJSON(`"2026-01-01T00:00:00Z"`), Deleted: true},
		{Hostname: "nodate", SiteName: "C", LastSeen: rawJSON("null")},
	}

	tests := []struct {
		name      string
		days      int
		wantHosts []string // in expected order
	}{
		{"30d window sorted desc", 30, []string{"old2", "old1"}},
		{"huge window excludes all", 9999, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := computeStale(devices, tc.days, now)
			if len(got) != len(tc.wantHosts) {
				t.Fatalf("got %d rows %+v, want %d", len(got), got, len(tc.wantHosts))
			}
			for i, h := range tc.wantHosts {
				if got[i].Hostname != h {
					t.Fatalf("row %d hostname = %q, want %q", i, got[i].Hostname, h)
				}
			}
		})
	}

	// Field-level assertions on happy path.
	got := computeStale(devices, 30, now)
	if got[0].DaysStale < got[1].DaysStale {
		t.Fatalf("not sorted by DaysStale desc: %+v", got)
	}
	if got[0].LastSeen != "2026-03-01T00:00:00Z" {
		t.Fatalf("LastSeen = %q, want 2026-03-01T00:00:00Z", got[0].LastSeen)
	}
}
