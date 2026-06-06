// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"testing"
	"time"
)

func TestComputeOrphans(t *testing.T) {
	now := time.Date(2026, 5, 29, 0, 0, 0, 0, time.UTC)
	recentAlert := rawJSON(`"2026-05-28T00:00:00Z"`)

	devices := []fleetDevice{
		{UID: "u1", Hostname: "suspended", SiteName: "A", Suspended: true, LastSeen: rawJSON(`"2026-05-28T00:00:00Z"`)},
		{UID: "u2", Hostname: "deleted", SiteName: "A", Deleted: true, LastSeen: rawJSON(`"2026-02-01T00:00:00Z"`)},
		{UID: "u3", Hostname: "stale", SiteName: "B", LastSeen: rawJSON(`"2026-01-01T00:00:00Z"`)},              // ~148d stale, no alert
		{UID: "u4", Hostname: "stale-but-alerting", SiteName: "B", LastSeen: rawJSON(`"2026-01-01T00:00:00Z"`)}, // stale but has recent alert
		{UID: "u5", Hostname: "healthy", SiteName: "C", LastSeen: rawJSON(`"2026-05-28T00:00:00Z"`)},
	}
	alerts := []fleetAlert{
		{AlertSourceInfo: fleetAlertSource{DeviceUID: "u4"}, Timestamp: recentAlert},
	}

	t.Run("stale-days 90, excludes recently-alerting and healthy", func(t *testing.T) {
		got := computeOrphans(devices, alerts, 90, now)
		gotHosts := map[string]orphanView{}
		for _, v := range got {
			gotHosts[v.Hostname] = v
		}
		if _, ok := gotHosts["healthy"]; ok {
			t.Fatalf("healthy should not be orphan: %+v", got)
		}
		if _, ok := gotHosts["stale-but-alerting"]; ok {
			t.Fatalf("stale-but-alerting has recent alert, should be excluded: %+v", got)
		}
		if len(got) != 3 {
			t.Fatalf("got %d orphans, want 3: %+v", len(got), got)
		}
		if got[0].Hostname != "stale" {
			t.Fatalf("most-stale first = %q, want stale", got[0].Hostname)
		}
		if gotHosts["deleted"].Reason != "deleted" {
			t.Fatalf("deleted reason = %q", gotHosts["deleted"].Reason)
		}
		if gotHosts["suspended"].Reason != "suspended" {
			t.Fatalf("suspended reason = %q", gotHosts["suspended"].Reason)
		}
	})

	t.Run("empty input -> empty", func(t *testing.T) {
		got := computeOrphans(nil, nil, 90, now)
		if len(got) != 0 {
			t.Fatalf("got %d, want 0", len(got))
		}
	})

	t.Run("all healthy -> empty", func(t *testing.T) {
		healthy := []fleetDevice{{UID: "x", Hostname: "h", LastSeen: rawJSON(`"2026-05-28T00:00:00Z"`)}}
		got := computeOrphans(healthy, nil, 90, now)
		if len(got) != 0 {
			t.Fatalf("got %d, want 0", len(got))
		}
	})
}
