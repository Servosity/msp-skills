// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"testing"
	"time"
)

func oncallIv(scheduleID, scheduleName, userID string, start, end time.Time) map[string]any {
	return map[string]any{
		"schedule": ref(scheduleID, scheduleName),
		"user":     ref(userID, "user "+userID),
		"start":    rfc(start),
		"end":      rfc(end),
	}
}

func TestBuildScheduleGapsFindsHoles(t *testing.T) {
	from := time.Date(2026, 6, 6, 0, 0, 0, 0, time.UTC)
	win := pdWindow{Since: from, Until: from.AddDate(0, 0, 7)}
	schedules := []map[string]any{
		{"id": "PSCHED1", "summary": "Primary NOC"},
		{"id": "PSCHED2", "summary": "Secondary"},
		{"id": "PSCHED3", "summary": "Never synced"},
	}
	oncalls := []map[string]any{
		// PSCHED1: covered days 0-3, gap day 3-4, covered day 4 to end.
		oncallIv("PSCHED1", "Primary NOC", "U1", from, from.AddDate(0, 0, 3)),
		oncallIv("PSCHED1", "Primary NOC", "U2", from.AddDate(0, 0, 4), win.Until),
		// PSCHED2: fully covered by two overlapping intervals.
		oncallIv("PSCHED2", "Secondary", "U3", from.Add(-24*time.Hour), from.AddDate(0, 0, 5)),
		oncallIv("PSCHED2", "Secondary", "U4", from.AddDate(0, 0, 4), win.Until.Add(24*time.Hour)),
		// EP-level on-call with no schedule ref: ignored.
		{"user": ref("U5", "user U5"), "start": rfc(from), "end": rfc(win.Until)},
	}

	res := buildScheduleGaps(schedules, oncalls, win, "")

	if len(res.Schedules) != 3 {
		t.Fatalf("expected 3 schedule rows, got %d", len(res.Schedules))
	}
	// Gappy schedule sorts first.
	first := res.Schedules[0]
	if first.ScheduleID != "PSCHED1" || len(first.Gaps) != 1 {
		t.Fatalf("first row = %+v, want PSCHED1 with 1 gap", first)
	}
	wantFrom := rfc(from.AddDate(0, 0, 3))
	wantUntil := rfc(from.AddDate(0, 0, 4))
	if first.Gaps[0].From != wantFrom || first.Gaps[0].Until != wantUntil {
		t.Errorf("gap = %+v, want %s -> %s", first.Gaps[0], wantFrom, wantUntil)
	}
	// Unsynced schedule sorts second, flagged.
	second := res.Schedules[1]
	if second.ScheduleID != "PSCHED3" || !second.NoSyncedData {
		t.Errorf("second row = %+v, want PSCHED3 flagged no_synced_data", second)
	}
	// Fully covered schedule sorts last.
	third := res.Schedules[2]
	if third.ScheduleID != "PSCHED2" || !third.Covered || len(third.Gaps) != 0 {
		t.Errorf("third row = %+v, want PSCHED2 fully covered", third)
	}
}

func TestBuildScheduleGapsScheduleFilter(t *testing.T) {
	from := time.Date(2026, 6, 6, 0, 0, 0, 0, time.UTC)
	win := pdWindow{Since: from, Until: from.AddDate(0, 0, 7)}
	schedules := []map[string]any{
		{"id": "PSCHED1", "summary": "Primary NOC"},
		{"id": "PSCHED2", "summary": "Secondary"},
	}
	res := buildScheduleGaps(schedules, nil, win, "Primary NOC")
	if len(res.Schedules) != 1 || res.Schedules[0].ScheduleID != "PSCHED1" {
		t.Fatalf("filter by name failed: %+v", res.Schedules)
	}
}

func TestBuildScheduleGapsEmpty(t *testing.T) {
	from := time.Date(2026, 6, 6, 0, 0, 0, 0, time.UTC)
	win := pdWindow{Since: from, Until: from.AddDate(0, 0, 7)}
	res := buildScheduleGaps(nil, nil, win, "")
	if len(res.Schedules) != 0 {
		t.Fatalf("expected no rows, got %d", len(res.Schedules))
	}
	if res.Note == "" {
		t.Errorf("expected a sync-first note on empty result")
	}
}

func TestBuildScheduleGapsOpenEndedIntervals(t *testing.T) {
	from := time.Date(2026, 6, 6, 0, 0, 0, 0, time.UTC)
	win := pdWindow{Since: from, Until: from.AddDate(0, 0, 7)}
	schedules := []map[string]any{
		{"id": "PSCHED1", "summary": "Open start"},
		{"id": "PSCHED2", "summary": "Open end"},
	}
	oncalls := []map[string]any{
		// Missing start: interval covers from the window start to day 2.
		{"schedule": ref("PSCHED1", "Open start"), "user": ref("U1", "user U1"), "end": rfc(from.AddDate(0, 0, 2))},
		// Missing end: interval covers day 5 to the window end.
		{"schedule": ref("PSCHED2", "Open end"), "user": ref("U2", "user U2"), "start": rfc(from.AddDate(0, 0, 5))},
	}
	res := buildScheduleGaps(schedules, oncalls, win, "")
	if len(res.Schedules) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(res.Schedules))
	}
	for _, row := range res.Schedules {
		if row.NoSyncedData {
			t.Fatalf("open-ended interval was dropped instead of clipped: %+v", row)
		}
		if len(row.Gaps) != 1 {
			t.Fatalf("%s: expected exactly 1 gap, got %+v", row.ScheduleID, row.Gaps)
		}
	}
	// Open start: gap is day2 -> end of window. Open end: gap is start -> day5.
	for _, row := range res.Schedules {
		switch row.ScheduleID {
		case "PSCHED1":
			if row.Gaps[0].From != rfc(from.AddDate(0, 0, 2)) || row.Gaps[0].Until != rfc(win.Until) {
				t.Errorf("PSCHED1 gap = %+v, want day2 -> window end", row.Gaps[0])
			}
		case "PSCHED2":
			if row.Gaps[0].From != rfc(from) || row.Gaps[0].Until != rfc(from.AddDate(0, 0, 5)) {
				t.Errorf("PSCHED2 gap = %+v, want window start -> day5", row.Gaps[0])
			}
		}
	}
}
