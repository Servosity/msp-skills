// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"testing"
	"time"
)

func TestBuildStaleIncidentsSweepsIdleOpen(t *testing.T) {
	now := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)
	incidents := []map[string]any{
		{"id": "I1", "title": "DB on fire", "status": "triggered", "urgency": "high",
			"created_at": rfc(now.Add(-72 * time.Hour)), "service": ref("SVCA", "db"),
			"assignments": []any{map[string]any{"assignee": ref("U1", "Priya")}}},
		{"id": "I2", "title": "Slow checkout", "status": "acknowledged", "urgency": "low",
			"created_at": rfc(now.Add(-48 * time.Hour)), "service": ref("SVCB", "web")},
		{"id": "I3", "title": "Fresh page", "status": "triggered", "urgency": "high",
			"created_at": rfc(now.Add(-2 * time.Hour)), "service": ref("SVCB", "web")},
		{"id": "I4", "title": "Already done", "status": "resolved",
			"created_at": rfc(now.Add(-100 * time.Hour)), "service": ref("SVCA", "db")},
	}
	logs := []map[string]any{
		// I1 touched 30h ago (newest of two entries) -> stale at 24h threshold.
		{"incident": ref("I1", "DB on fire"), "created_at": rfc(now.Add(-60 * time.Hour)), "type": "acknowledge_log_entry"},
		{"incident": ref("I1", "DB on fire"), "created_at": rfc(now.Add(-30 * time.Hour)), "type": "annotate_log_entry"},
		// I3 touched 1h ago -> fresh.
		{"incident": ref("I3", "Fresh page"), "created_at": rfc(now.Add(-time.Hour)), "type": "acknowledge_log_entry"},
		// Resolved I4 has logs but is not open.
		{"incident": ref("I4", "Already done"), "created_at": rfc(now.Add(-99 * time.Hour)), "type": "resolve_log_entry"},
	}

	res := buildStaleIncidents(incidents, logs, 24*time.Hour, now)

	if res.OpenIncidents != 3 {
		t.Fatalf("open_incidents = %d, want 3", res.OpenIncidents)
	}
	if len(res.Stale) != 2 {
		t.Fatalf("stale = %d rows, want 2 (I1 + I2): %+v", len(res.Stale), res.Stale)
	}
	// I2 has no logs at all -> falls back to created_at (48h idle) and sorts first.
	if res.Stale[0].IncidentID != "I2" || res.Stale[0].IdleSeconds != int64((48*time.Hour).Seconds()) {
		t.Errorf("longest-idle row = %+v, want I2 at 48h", res.Stale[0])
	}
	if res.Stale[1].IncidentID != "I1" || res.Stale[1].IdleSeconds != int64((30*time.Hour).Seconds()) {
		t.Errorf("second row = %+v, want I1 at 30h (newest log entry wins)", res.Stale[1])
	}
	if res.Stale[1].AssignedTo != "Priya" {
		t.Errorf("assigned_to = %q, want Priya", res.Stale[1].AssignedTo)
	}
	for _, s := range res.Stale {
		if s.IncidentID == "I3" {
			t.Errorf("fresh incident I3 leaked into stale sweep")
		}
		if s.IncidentID == "I4" {
			t.Errorf("resolved incident I4 leaked into stale sweep")
		}
	}
}

func TestBuildStaleIncidentsAllFresh(t *testing.T) {
	now := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)
	incidents := []map[string]any{
		{"id": "I1", "title": "Fresh", "status": "triggered", "created_at": rfc(now.Add(-time.Hour)), "service": ref("SVCA", "db")},
	}
	res := buildStaleIncidents(incidents, nil, 24*time.Hour, now)
	if res.OpenIncidents != 1 || len(res.Stale) != 0 {
		t.Fatalf("want 1 open / 0 stale, got %d/%d", res.OpenIncidents, len(res.Stale))
	}
	if res.Note == "" {
		t.Errorf("expected a lower-the-threshold note when everything is fresh")
	}
}

func TestBuildStaleIncidentsEmptyStore(t *testing.T) {
	res := buildStaleIncidents(nil, nil, 24*time.Hour, time.Now())
	if res.OpenIncidents != 0 || len(res.Stale) != 0 || res.Note != "" {
		t.Fatalf("empty store should produce zero rows and no note, got %+v", res)
	}
}
