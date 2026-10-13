// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"path/filepath"
	"testing"

	"granola-pp-cli/internal/granola"
	"granola-pp-cli/internal/store"

	_ "modernc.org/sqlite"
)

// newIsolatedStore creates a fresh SQLite store in a temp dir, applies the
// granola schema, and points granolaStoreOverride at it for the test's
// lifetime (restored on cleanup). This guarantees openGranolaCache reads ONLY
// this store and never the developer's real ~/.local/share store. Seed rows
// through the returned handle, then Close() it before invoking openGranolaCache
// so the command opens its own read connection.
func newIsolatedStore(t *testing.T) *store.Store {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "data.db")
	s, err := store.OpenWithContext(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	if err := granola.EnsureSchema(context.Background(), s.DB()); err != nil {
		s.Close()
		t.Fatalf("ensure schema: %v", err)
	}
	prev := granolaStoreOverride
	granolaStoreOverride = dbPath
	t.Cleanup(func() { granolaStoreOverride = prev })
	return s
}

// seedMeeting carries the meetings-table columns the tests care about; empty
// fields are inserted as empty strings (never NULL) and valid_meeting is 1.
type seedMeeting struct {
	ID, Title            string
	CreatedAt, UpdatedAt string
	StartedAt, EndedAt   string
	NotesMarkdown        string
	NotesPlain           string
	CalendarEventID      string
}

func insertMeeting(t *testing.T, s *store.Store, m seedMeeting) {
	t.Helper()
	_, err := s.DB().Exec(`INSERT INTO meetings
		(id, title, created_at, updated_at, started_at, ended_at,
		 notes_markdown, notes_plain, calendar_event_id, valid_meeting)
		VALUES (?,?,?,?,?,?,?,?,?,1)`,
		m.ID, m.Title, m.CreatedAt, m.UpdatedAt, m.StartedAt, m.EndedAt,
		m.NotesMarkdown, m.NotesPlain, m.CalendarEventID)
	if err != nil {
		t.Fatalf("insert meeting %s: %v", m.ID, err)
	}
}

func insertAttendee(t *testing.T, s *store.Store, meetingID, email, name, responseStatus string) {
	t.Helper()
	_, err := s.DB().Exec(
		`INSERT INTO attendees(meeting_id, email, name, response_status) VALUES (?,?,?,?)`,
		meetingID, email, name, responseStatus)
	if err != nil {
		t.Fatalf("insert attendee %s/%s: %v", meetingID, email, err)
	}
}

// insertSegment inserts one transcript segment with deterministic millisecond
// timestamps derived from idx (the actual values are irrelevant to the source
// counts these tests assert).
func insertSegment(t *testing.T, s *store.Store, meetingID string, idx int, source, text string) {
	t.Helper()
	base := int64(1_700_000_000_000)
	start := base + int64(idx)*5000
	_, err := s.DB().Exec(
		`INSERT INTO transcript_segments(meeting_id, idx, source, text, start_ts_ms, end_ts_ms, confidence)
		 VALUES (?,?,?,?,?,?,?)`,
		meetingID, idx, source, text, start, start+5000, 1.0)
	if err != nil {
		t.Fatalf("insert segment %s/%d: %v", meetingID, idx, err)
	}
}

// TestBackfillMeetingsMetadataFromStore proves the new backfill reconstructs
// MeetingsMetadata (attendees) from seeded meetings + attendees rows.
func TestBackfillMeetingsMetadataFromStore(t *testing.T) {
	s := newIsolatedStore(t)
	insertMeeting(t, s, seedMeeting{ID: "m1", Title: "Sync Meeting", CreatedAt: "2026-05-01T10:00:00Z", CalendarEventID: "cal_1"})
	insertMeeting(t, s, seedMeeting{ID: "m2", Title: "Solo Meeting", CreatedAt: "2026-05-02T10:00:00Z"})
	insertAttendee(t, s, "m1", "alice@example.com", "Alice", "accepted")
	insertAttendee(t, s, "m1", "bob@example.com", "Bob", "declined")
	s.Close()

	c := &granola.Cache{}
	if err := backfillMeetingsMetadataFromStore(c); err != nil {
		t.Fatalf("backfillMeetingsMetadataFromStore: %v", err)
	}

	md := c.MeetingMetadataByID("m1")
	if md == nil {
		t.Fatal("expected metadata for m1, got nil")
	}
	if len(md.Attendees) != 2 {
		t.Fatalf("expected 2 attendees for m1, got %d (%+v)", len(md.Attendees), md.Attendees)
	}
	byEmail := map[string]granola.CalendarInvitee{}
	for _, a := range md.Attendees {
		byEmail[a.Email] = a
	}
	if a := byEmail["alice@example.com"]; a.Name != "Alice" || a.ResponseStatus != "accepted" {
		t.Errorf("alice attendee not populated from store: %+v", a)
	}
	if a := byEmail["bob@example.com"]; a.Name != "Bob" || a.ResponseStatus != "declined" {
		t.Errorf("bob attendee not populated from store: %+v", a)
	}
	// REST exposes no meeting creator; it must be left nil, not fabricated.
	if md.Creator != nil {
		t.Errorf("expected nil Creator (no REST source), got %+v", md.Creator)
	}
	// A meeting with no attendees yields no metadata entry (readers treat the
	// nil return from MeetingMetadataByID as "no data").
	if md2 := c.MeetingMetadataByID("m2"); md2 != nil {
		t.Errorf("expected no metadata for attendee-less m2, got %+v", md2)
	}
}

// TestBackfillDocumentsFromStore_PeopleAndCalendar proves the documents
// backfill reconstructs doc.people and google_calendar_event (id + window)
// from the store.
func TestBackfillDocumentsFromStore_PeopleAndCalendar(t *testing.T) {
	s := newIsolatedStore(t)
	insertMeeting(t, s, seedMeeting{
		ID: "m1", Title: "Sync Meeting",
		CreatedAt: "2026-05-01T10:00:00Z", UpdatedAt: "2026-05-01T11:00:00Z",
		StartedAt: "2026-05-01T10:00:00Z", EndedAt: "2026-05-01T10:45:00Z",
		CalendarEventID: "cal_1",
	})
	insertAttendee(t, s, "m1", "alice@example.com", "Alice", "accepted")
	s.Close()

	c := &granola.Cache{}
	if err := backfillDocumentsFromStore(c); err != nil {
		t.Fatalf("backfillDocumentsFromStore: %v", err)
	}
	d := c.DocumentByID("m1")
	if d == nil {
		t.Fatal("expected document m1, got nil")
	}
	if d.People == nil || len(d.People.Attendees) != 1 || d.People.Attendees[0].Email != "alice@example.com" {
		t.Errorf("doc.people not backfilled from attendees: %+v", d.People)
	}
	if d.GoogleCalendarEvent == nil || d.GoogleCalendarEvent.ID != "cal_1" {
		t.Fatalf("google_calendar_event.id not backfilled: %+v", d.GoogleCalendarEvent)
	}
	// The stored meeting window is reconstructed into the calendar event's
	// {"dateTime": ...} shape so docWindow can read it back.
	if got := extractCalTimeRaw(d.GoogleCalendarEvent.Start); got != "2026-05-01T10:00:00Z" {
		t.Errorf("calendar start = %q, want the stored started_at", got)
	}
	if got := extractCalTimeRaw(d.GoogleCalendarEvent.End); got != "2026-05-01T10:45:00Z" {
		t.Errorf("calendar end = %q, want the stored ended_at", got)
	}
}

// TestOpenGranolaCache_MissingStoreIsEmptyNotError proves a fresh install (no
// SQLite DB yet) yields an empty cache with a nil error, so commands still run.
func TestOpenGranolaCache_MissingStoreIsEmptyNotError(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "no-such.db")
	prev := granolaStoreOverride
	granolaStoreOverride = missing
	t.Cleanup(func() { granolaStoreOverride = prev })

	c, err := openGranolaCache()
	if err != nil {
		t.Fatalf("missing store must not error, got: %v", err)
	}
	if c == nil {
		t.Fatal("expected non-nil cache")
	}
	if len(c.Documents) != 0 || len(c.MeetingsMetadata) != 0 || len(c.Transcripts) != 0 {
		t.Errorf("expected empty cache from missing store, got docs=%d md=%d tx=%d",
			len(c.Documents), len(c.MeetingsMetadata), len(c.Transcripts))
	}
}
