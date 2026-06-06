// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"testing"
	"time"
)

func TestBuildTimeline(t *testing.T) {
	now := time.Date(2026, 6, 5, 0, 0, 0, 0, time.UTC)
	alerts := []fleetAlert{
		mkAlert("a1", "dev1", "ACME-DC01", "Acme", false, "2026-06-04T10:00:00Z"),
		mkAlert("a2", "dev1", "ACME-DC01", "Acme", true, "2026-06-03T10:00:00Z"),
		mkAlert("a3", "dev2", "OTHER-BOX", "Beta", false, "2026-06-04T11:00:00Z"),
	}
	logs := []fleetActivityLog{
		{Action: "Quick job ran", Category: "job", Date: rawJSON(`"2026-06-04T12:00:00Z"`), Hostname: "ACME-DC01", Site: "Acme", User: "tech1"},
		{Action: "Audit", Category: "audit", Date: rawJSON(`"2026-01-01T00:00:00Z"`), Hostname: "ACME-DC01", Site: "Acme"}, // outside window
		{Action: "Login", Category: "user", Date: rawJSON(`"2026-06-04T09:00:00Z"`), Hostname: "OTHER-BOX", Site: "Beta"},
	}

	events := buildTimeline(alerts, logs, "ACME-DC01", 30, 100, now)
	if len(events) != 3 {
		t.Fatalf("events = %d, want 3 (2 alerts + 1 activity)", len(events))
	}
	// Newest first.
	if events[0].Source != "activity" || events[0].Time != "2026-06-04T12:00:00Z" {
		t.Fatalf("first event wrong: %+v", events[0])
	}
	if events[1].Source != "alert-open" {
		t.Fatalf("second event wrong: %+v", events[1])
	}
	if events[2].Source != "alert-resolved" {
		t.Fatalf("third event wrong: %+v", events[2])
	}
}

func TestBuildTimelineMatchesByUIDAndLimit(t *testing.T) {
	now := time.Date(2026, 6, 5, 0, 0, 0, 0, time.UTC)
	alerts := []fleetAlert{
		mkAlert("a1", "dev1", "ACME-DC01", "Acme", false, "2026-06-04T10:00:00Z"),
		mkAlert("a2", "dev1", "ACME-DC01", "Acme", false, "2026-06-03T10:00:00Z"),
	}
	events := buildTimeline(alerts, nil, "dev1", 30, 1, now)
	if len(events) != 1 {
		t.Fatalf("limit not applied: %d events", len(events))
	}
	if events[0].Time != "2026-06-04T10:00:00Z" {
		t.Fatalf("kept the wrong event under limit: %+v", events[0])
	}
}

func TestBuildTimelineNoMatchesIsEmpty(t *testing.T) {
	now := time.Date(2026, 6, 5, 0, 0, 0, 0, time.UTC)
	alerts := []fleetAlert{
		mkAlert("a1", "dev1", "ACME-DC01", "Acme", false, "2026-06-04T10:00:00Z"),
	}
	events := buildTimeline(alerts, nil, "NO-SUCH-DEVICE", 30, 100, now)
	if len(events) != 0 {
		t.Fatalf("expected empty timeline, got %+v", events)
	}
}

func TestMatchesDevice(t *testing.T) {
	tests := []struct {
		name string
		key  string
		uid  string
		host string
		id   string
		want bool
	}{
		{"by uid", "dev1", "dev1", "ALPHA", "", true},
		{"by hostname case-insensitive", "acme-dc01", "", "ACME-DC01", "", true},
		{"by numeric id", "42", "", "ALPHA", "42", true},
		{"no match", "other", "dev1", "ALPHA", "42", false},
		{"empty key", "", "dev1", "ALPHA", "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var raw []byte
			if tc.id != "" {
				raw = []byte(tc.id)
			}
			if got := matchesDevice(tc.key, tc.uid, tc.host, raw); got != tc.want {
				t.Fatalf("matchesDevice = %v, want %v", got, tc.want)
			}
		})
	}
}
