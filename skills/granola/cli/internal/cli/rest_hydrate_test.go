// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"granola-pp-cli/internal/granola"
	"granola-pp-cli/internal/store"

	_ "modernc.org/sqlite"
)

func newHydrateTestStore(t *testing.T) *store.Store {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "data.db")
	s, err := store.OpenWithContext(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if err := granola.EnsureSchema(context.Background(), s.DB()); err != nil {
		t.Fatalf("EnsureSchema: %v", err)
	}
	return s
}

func calRaw(t *testing.T, ce restCalendarEvent) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(ce)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func countRows(t *testing.T, s *store.Store, q string, args ...any) int {
	t.Helper()
	var n int
	if err := s.DB().QueryRowContext(context.Background(), q, args...).Scan(&n); err != nil {
		t.Fatalf("count %q: %v", q, err)
	}
	return n
}

func meetingCol(t *testing.T, s *store.Store, id, col string) string {
	t.Helper()
	var v string
	if err := s.DB().QueryRowContext(context.Background(), "SELECT "+col+" FROM meetings WHERE id=?", id).Scan(&v); err != nil {
		t.Fatalf("select %s: %v", col, err)
	}
	return v
}

func ptrAtt(a ...restAttendee) *[]restAttendee { return &a }
func ptrSeg(s ...restSegment) *[]restSegment   { return &s }

// TestUpsertRichMeeting_NonDestructiveOnPartial is the core data-loss guard:
// a full detail lands attendees/transcript/calendar; a second PARTIAL detail
// (attendees/transcript/calendar absent, as a truncated 200 would be) must NOT
// delete the previously stored rows or blank the calendar/bounds.
func TestUpsertRichMeeting_NonDestructiveOnPartial(t *testing.T) {
	ctx := context.Background()
	s := newHydrateTestStore(t)
	full := &restNoteDetail{
		ID: "not_1", Title: "Kickoff",
		CreatedAt: "2026-07-01T09:05:00Z", UpdatedAt: "2026-07-01T09:40:00Z",
		Attendees: ptrAtt(restAttendee{Name: "Alice", Email: "alice@example.com"}, restAttendee{Email: "bob@example.com"}),
		Transcript: ptrSeg(
			restSegment{Text: "hi", StartTime: "2026-07-01T09:00:00Z", EndTime: "2026-07-01T09:00:05Z"},
			restSegment{Text: "bye", StartTime: "2026-07-01T09:29:00Z", EndTime: "2026-07-01T09:30:00Z"},
		),
		CalendarEvent: calRaw(t, restCalendarEvent{CalendarEventID: "cal_1", ScheduledStartTime: "2026-07-01T09:00:00Z", ScheduledEndTime: "2026-07-01T09:30:00Z"}),
	}
	if err := upsertRichMeeting(ctx, s.DB(), full); err != nil {
		t.Fatalf("full upsert: %v", err)
	}
	if got := countRows(t, s, "SELECT COUNT(*) FROM attendees WHERE meeting_id=?", "not_1"); got != 2 {
		t.Fatalf("attendees after full: got %d want 2", got)
	}
	if got := countRows(t, s, "SELECT COUNT(*) FROM transcript_segments WHERE meeting_id=?", "not_1"); got != 2 {
		t.Fatalf("segments after full: got %d want 2", got)
	}
	if got := meetingCol(t, s, "not_1", "calendar_event_id"); got != "cal_1" {
		t.Fatalf("calendar_event_id: got %q want cal_1", got)
	}
	if got := meetingCol(t, s, "not_1", "started_at"); got != "2026-07-01T09:00:00Z" {
		t.Fatalf("started_at should be scheduled_start (not created_at): got %q", got)
	}

	// Partial re-sync: attendees/transcript/calendar all absent; only title changes.
	partial := &restNoteDetail{ID: "not_1", Title: "Kickoff (edited)", CreatedAt: full.CreatedAt, UpdatedAt: "2026-07-02T10:00:00Z"}
	if err := upsertRichMeeting(ctx, s.DB(), partial); err != nil {
		t.Fatalf("partial upsert: %v", err)
	}
	if got := countRows(t, s, "SELECT COUNT(*) FROM attendees WHERE meeting_id=?", "not_1"); got != 2 {
		t.Fatalf("PARTIAL deleted attendees: got %d want 2", got)
	}
	if got := countRows(t, s, "SELECT COUNT(*) FROM transcript_segments WHERE meeting_id=?", "not_1"); got != 2 {
		t.Fatalf("PARTIAL deleted segments: got %d want 2", got)
	}
	if got := meetingCol(t, s, "not_1", "calendar_event_id"); got != "cal_1" {
		t.Fatalf("PARTIAL blanked calendar_event_id: got %q", got)
	}
	if got := meetingCol(t, s, "not_1", "started_at"); got != "2026-07-01T09:00:00Z" {
		t.Fatalf("PARTIAL blanked bounds: got %q", got)
	}
	if got := meetingCol(t, s, "not_1", "title"); got != "Kickoff (edited)" {
		t.Fatalf("title should update: got %q", got)
	}
}

// TestUpsertRichMeeting_ExplicitEmptyClears: a PRESENT empty array is
// authoritative and clears rows (distinct from an absent field).
func TestUpsertRichMeeting_ExplicitEmptyClears(t *testing.T) {
	ctx := context.Background()
	s := newHydrateTestStore(t)
	if err := upsertRichMeeting(ctx, s.DB(), &restNoteDetail{
		ID: "not_2", Title: "M",
		Attendees:  ptrAtt(restAttendee{Email: "x@example.com"}),
		Transcript: ptrSeg(restSegment{Text: "hi", StartTime: "2026-07-01T09:00:00Z", EndTime: "2026-07-01T09:00:01Z"}),
	}); err != nil {
		t.Fatal(err)
	}
	empty := []restAttendee{}
	emptySeg := []restSegment{}
	if err := upsertRichMeeting(ctx, s.DB(), &restNoteDetail{ID: "not_2", Title: "M", Attendees: &empty, Transcript: &emptySeg}); err != nil {
		t.Fatal(err)
	}
	if got := countRows(t, s, "SELECT COUNT(*) FROM attendees WHERE meeting_id=?", "not_2"); got != 0 {
		t.Fatalf("explicit empty should clear attendees: got %d", got)
	}
	if got := countRows(t, s, "SELECT COUNT(*) FROM transcript_segments WHERE meeting_id=?", "not_2"); got != 0 {
		t.Fatalf("explicit empty should clear segments: got %d", got)
	}
}

// TestUpsertRichMeeting_NoFabricatedBounds: no calendar + no transcript means
// started_at/ended_at stay empty, never note created/updated times.
func TestUpsertRichMeeting_NoFabricatedBounds(t *testing.T) {
	ctx := context.Background()
	s := newHydrateTestStore(t)
	if err := upsertRichMeeting(ctx, s.DB(), &restNoteDetail{
		ID: "not_3", Title: "M", CreatedAt: "2026-07-01T09:05:00Z", UpdatedAt: "2026-07-01T09:40:00Z",
	}); err != nil {
		t.Fatal(err)
	}
	if got := meetingCol(t, s, "not_3", "started_at"); got != "" {
		t.Fatalf("started_at should be empty (not fabricated from created_at): got %q", got)
	}
	if got := meetingCol(t, s, "not_3", "ended_at"); got != "" {
		t.Fatalf("ended_at should be empty (not fabricated from updated_at): got %q", got)
	}
}

// TestUpsertRichMeeting_TranscriptSpanBounds: no calendar but transcript present
// → bounds come from the transcript span.
func TestUpsertRichMeeting_TranscriptSpanBounds(t *testing.T) {
	ctx := context.Background()
	s := newHydrateTestStore(t)
	if err := upsertRichMeeting(ctx, s.DB(), &restNoteDetail{
		ID: "not_4", Title: "M",
		Transcript: ptrSeg(
			restSegment{Text: "a", StartTime: "2026-07-01T09:00:00Z", EndTime: "2026-07-01T09:00:05Z"},
			restSegment{Text: "b", StartTime: "2026-07-01T09:29:00Z", EndTime: "2026-07-01T09:30:00Z"},
		),
	}); err != nil {
		t.Fatal(err)
	}
	if got := meetingCol(t, s, "not_4", "started_at"); got != "2026-07-01T09:00:00Z" {
		t.Fatalf("started_at should be first segment start: got %q", got)
	}
	if got := meetingCol(t, s, "not_4", "ended_at"); got != "2026-07-01T09:30:00Z" {
		t.Fatalf("ended_at should be last segment end: got %q", got)
	}
}

// TestUpsertRichMeeting_CalendarStartOnlyKeepsEnd is the FIX H(1) guard: a
// second detail whose calendar_event carries scheduled_start_time but NO
// scheduled_end_time must update started_at WITHOUT blanking the previously
// stored ended_at (the two bounds are guarded independently now).
func TestUpsertRichMeeting_CalendarStartOnlyKeepsEnd(t *testing.T) {
	ctx := context.Background()
	s := newHydrateTestStore(t)
	// Full calendar with both bounds.
	if err := upsertRichMeeting(ctx, s.DB(), &restNoteDetail{
		ID: "not_c", Title: "M",
		CalendarEvent: calRaw(t, restCalendarEvent{CalendarEventID: "cal_1", ScheduledStartTime: "2026-07-01T09:00:00Z", ScheduledEndTime: "2026-07-01T09:30:00Z"}),
	}); err != nil {
		t.Fatalf("full upsert: %v", err)
	}
	// Re-sync: calendar present with a NEW start but end omitted.
	if err := upsertRichMeeting(ctx, s.DB(), &restNoteDetail{
		ID: "not_c", Title: "M",
		CalendarEvent: calRaw(t, restCalendarEvent{CalendarEventID: "cal_1", ScheduledStartTime: "2026-07-01T09:05:00Z"}),
	}); err != nil {
		t.Fatalf("start-only upsert: %v", err)
	}
	if got := meetingCol(t, s, "not_c", "started_at"); got != "2026-07-01T09:05:00Z" {
		t.Fatalf("started_at should update to the new scheduled_start: got %q", got)
	}
	if got := meetingCol(t, s, "not_c", "ended_at"); got != "2026-07-01T09:30:00Z" {
		t.Fatalf("ended_at must NOT be blanked by a start-only calendar: got %q", got)
	}
}

// TestUpsertRichMeeting_OmittedTitleKeepsStored is the FIX H(2) guard: a partial
// 200 that omits the title must not blank the previously stored title.
func TestUpsertRichMeeting_OmittedTitleKeepsStored(t *testing.T) {
	ctx := context.Background()
	s := newHydrateTestStore(t)
	if err := upsertRichMeeting(ctx, s.DB(), &restNoteDetail{
		ID: "not_t", Title: "Quarterly Review",
		CreatedAt: "2026-07-01T09:05:00Z", UpdatedAt: "2026-07-01T09:40:00Z",
	}); err != nil {
		t.Fatalf("initial upsert: %v", err)
	}
	// Partial re-sync with title (and timestamps) omitted.
	if err := upsertRichMeeting(ctx, s.DB(), &restNoteDetail{ID: "not_t"}); err != nil {
		t.Fatalf("partial upsert: %v", err)
	}
	if got := meetingCol(t, s, "not_t", "title"); got != "Quarterly Review" {
		t.Fatalf("omitted title must NOT blank the stored title: got %q", got)
	}
	if got := meetingCol(t, s, "not_t", "created_at"); got != "2026-07-01T09:05:00Z" {
		t.Fatalf("omitted created_at must NOT blank the stored value: got %q", got)
	}
	if got := meetingCol(t, s, "not_t", "updated_at"); got != "2026-07-01T09:40:00Z" {
		t.Fatalf("omitted updated_at must NOT blank the stored value: got %q", got)
	}
}
