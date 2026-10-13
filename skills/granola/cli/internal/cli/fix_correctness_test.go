// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/json"
	"testing"

	"granola-pp-cli/internal/granola"
)

// TestDocWindow_NoRealSourceYieldsEmpty is the FIX I(b) guard: a doc with no
// calendar and no transcript must yield EMPTY bounds — never the note's
// created_at/updated_at edit times (which would fabricate a duration).
func TestDocWindow_NoRealSourceYieldsEmpty(t *testing.T) {
	d := &granola.Document{
		ID:        "not_x",
		CreatedAt: "2026-07-01T09:05:00Z",
		UpdatedAt: "2026-07-01T09:40:00Z",
	}
	start, end := docWindow(d, nil)
	if start != "" || end != "" {
		t.Fatalf("no calendar/transcript must yield empty bounds, got (%q,%q) — must not fall back to created/updated", start, end)
	}
}

// TestDocWindow_CalendarThenTranscript proves the real-source precedence:
// calendar window first, else transcript span; created/updated never used.
func TestDocWindow_CalendarThenTranscript(t *testing.T) {
	// Calendar present → its window wins.
	d := &granola.Document{
		ID:        "not_c",
		CreatedAt: "2026-07-01T08:00:00Z",
		UpdatedAt: "2026-07-01T12:00:00Z",
		GoogleCalendarEvent: &granola.GoogleCalendarEvent{
			Start: json.RawMessage(`{"dateTime":"2026-07-01T09:00:00Z"}`),
			End:   json.RawMessage(`{"dateTime":"2026-07-01T09:30:00Z"}`),
		},
	}
	if s, e := docWindow(d, nil); s != "2026-07-01T09:00:00Z" || e != "2026-07-01T09:30:00Z" {
		t.Fatalf("calendar window should win, got (%q,%q)", s, e)
	}

	// No calendar, transcript present → span fills the bounds.
	d2 := &granola.Document{ID: "not_t", CreatedAt: "2026-07-01T08:00:00Z", UpdatedAt: "2026-07-01T12:00:00Z"}
	segs := []granola.TranscriptSegment{
		{StartTimestamp: "2026-07-01T10:00:00Z", EndTimestamp: "2026-07-01T10:00:05Z"},
		{StartTimestamp: "2026-07-01T10:29:00Z", EndTimestamp: "2026-07-01T10:30:00Z"},
	}
	if s, e := docWindow(d2, segs); s != "2026-07-01T10:00:00Z" || e != "2026-07-01T10:30:00Z" {
		t.Fatalf("transcript span should fill bounds, got (%q,%q)", s, e)
	}
}

// TestStatsDuration_ExcludesEmptyBounds is the FIX I(a) guard: a meeting whose
// started_at/ended_at are EMPTY strings (no real bounds) must be excluded from
// duration stats rather than counted as a zero-duration meeting.
func TestStatsDuration_ExcludesEmptyBounds(t *testing.T) {
	s := newIsolatedStore(t)
	insertMeeting(t, s, seedMeeting{
		ID: "m_real", Title: "Real", CreatedAt: "2026-07-01T09:05:00Z",
		StartedAt: "2026-07-01T09:00:00Z", EndedAt: "2026-07-01T09:30:00Z",
		CalendarEventID: "cal_1",
	})
	insertMeeting(t, s, seedMeeting{
		ID: "m_empty", Title: "NoBounds", CreatedAt: "2026-07-02T09:05:00Z",
		StartedAt: "", EndedAt: "", CalendarEventID: "cal_2",
	})
	s.Close()

	flags := &rootFlags{asJSON: true}
	cmd := newStatsDurationCmd(flags)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"--by", "calendar"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("stats duration: %v", err)
	}

	var rows []map[string]any
	if err := json.Unmarshal(buf.Bytes(), &rows); err != nil {
		t.Fatalf("parse stats output %q: %v", buf.String(), err)
	}
	total := 0
	for _, r := range rows {
		if c, ok := r["meeting_count"].(float64); ok {
			total += int(c)
		}
		if r["key"] == "cal_2" {
			t.Fatalf("empty-bounds meeting (cal_2) must be excluded from duration stats: %v", rows)
		}
	}
	if total != 1 {
		t.Fatalf("duration stats meeting_count total = %d, want 1 (only the real-bounds meeting); output=%s", total, buf.String())
	}
}
