// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"granola-pp-cli/internal/client"
	"granola-pp-cli/internal/config"
	"granola-pp-cli/internal/store"
)

// insertNote seeds one thin `notes`-table row (as the framework notes/folders
// sync would) so the auto-refresh enrich has something to hydrate.
func insertNote(t *testing.T, s *store.Store, id, updatedAt string) {
	t.Helper()
	if _, err := s.DB().Exec(`INSERT INTO notes(id, data, updated_at) VALUES (?, '{}', ?)`, id, updatedAt); err != nil {
		t.Fatalf("insert note %s: %v", id, err)
	}
}

// TestUnhydratedRecentNoteIDs_IncrementalAndBounded proves the incremental
// signal: only notes lacking a meetings row are returned, newest-first, capped
// by the limit. This is what keeps steady-state auto-refresh at zero fetches.
func TestUnhydratedRecentNoteIDs_IncrementalAndBounded(t *testing.T) {
	ctx := context.Background()
	s := newHydrateTestStore(t)

	// not_1 (oldest) .. not_4 (newest); not_2 is already hydrated (meetings row).
	insertNote(t, s, "not_1", "2026-07-01T00:00:00Z")
	insertNote(t, s, "not_2", "2026-07-02T00:00:00Z")
	insertNote(t, s, "not_3", "2026-07-03T00:00:00Z")
	insertNote(t, s, "not_4", "2026-07-04T00:00:00Z")
	// A real hydrated meeting carries the note's updated_at (upsertRichMeeting
	// copies d.UpdatedAt), so it is NOT stale vs its note and stays excluded.
	if err := upsertRichMeeting(ctx, s.DB(), &restNoteDetail{ID: "not_2", Title: "already hydrated", UpdatedAt: "2026-07-02T00:00:00Z"}); err != nil {
		t.Fatalf("seed hydrated meeting: %v", err)
	}

	// Unbounded: every un-hydrated note, newest-first, excluding not_2.
	ids, err := unhydratedRecentNoteIDs(ctx, s.DB(), 0)
	if err != nil {
		t.Fatalf("unhydratedRecentNoteIDs: %v", err)
	}
	want := []string{"not_4", "not_3", "not_1"}
	if strings.Join(ids, ",") != strings.Join(want, ",") {
		t.Fatalf("un-hydrated ids = %v, want %v (not_2 is hydrated and must be excluded)", ids, want)
	}

	// Bounded: cap to 2 → only the two newest un-hydrated notes.
	ids, err = unhydratedRecentNoteIDs(ctx, s.DB(), 2)
	if err != nil {
		t.Fatalf("unhydratedRecentNoteIDs(limit=2): %v", err)
	}
	if strings.Join(ids, ",") != "not_4,not_3" {
		t.Fatalf("bounded ids = %v, want [not_4 not_3]", ids)
	}
}

// TestEnrichRecentForAutoRefresh_PopulatesRichTables is the FIX G happy-path
// receipt: after the thin sync populated `notes`, the auto-refresh enrich fills
// the rich meetings/transcript_segments tables for un-hydrated notes WITHOUT a
// manual `sync`/`enrich`, and it is incremental — an already-hydrated note is
// never re-fetched.
func TestEnrichRecentForAutoRefresh_PopulatesRichTables(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // isolate: no real MCP creds → human-notes step no-ops
	ctx := context.Background()
	s := newHydrateTestStore(t)

	// The detail endpoint echoes each note's real updated_at (the same value the
	// thin `notes` sync stored), so after enrichment m.updated_at == n.updated_at
	// and the note is not seen as stale on the next pass.
	noteUpdated := map[string]string{
		"not_new1": "2026-07-03T00:00:00Z",
		"not_new2": "2026-07-02T00:00:00Z",
		"not_done": "2026-07-01T00:00:00Z",
	}
	insertNote(t, s, "not_new1", noteUpdated["not_new1"])
	insertNote(t, s, "not_new2", noteUpdated["not_new2"])
	insertNote(t, s, "not_done", noteUpdated["not_done"])
	// not_done already has rich data at its note's updated_at — must NOT be re-fetched.
	if err := upsertRichMeeting(ctx, s.DB(), &restNoteDetail{ID: "not_done", Title: "done", UpdatedAt: noteUpdated["not_done"]}); err != nil {
		t.Fatalf("seed hydrated meeting: %v", err)
	}

	fetched := map[string]int{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/v1/notes/")
		fetched[id]++
		_ = json.NewEncoder(w).Encode(restNoteDetail{
			ID: id, Title: "Meeting " + id,
			CreatedAt: "2026-07-01T09:05:00Z", UpdatedAt: noteUpdated[id],
			Transcript: ptrSeg(restSegment{Text: "hi", StartTime: "2026-07-01T09:00:00Z", EndTime: "2026-07-01T09:30:00Z"}),
		})
	}))
	defer srv.Close()

	c := client.New(&config.Config{BaseURL: srv.URL, GranolaApiKey: "test-key"}, 0, 0)
	c.NoCache = true

	n := enrichRecentForAutoRefresh(ctx, s.DB(), c)
	if n != 2 {
		t.Fatalf("enriched %d meetings, want 2 (the two un-hydrated notes)", n)
	}
	// Incremental: only the un-hydrated notes were fetched.
	if fetched["not_done"] != 0 {
		t.Fatalf("already-hydrated note was re-fetched %d time(s) — not incremental", fetched["not_done"])
	}
	if fetched["not_new1"] != 1 || fetched["not_new2"] != 1 {
		t.Fatalf("expected exactly one fetch each for the un-hydrated notes, got %v", fetched)
	}
	// Rich tables now populated (not thin-only): meetings + transcript_segments.
	if got := countRows(t, s, "SELECT COUNT(*) FROM meetings WHERE id IN ('not_new1','not_new2')"); got != 2 {
		t.Fatalf("meetings rows for enriched notes = %d, want 2", got)
	}
	if got := countRows(t, s, "SELECT COUNT(*) FROM transcript_segments WHERE meeting_id='not_new1'"); got != 1 {
		t.Fatalf("transcript_segments for not_new1 = %d, want 1", got)
	}
	if got := meetingCol(t, s, "not_new1", "started_at"); got != "2026-07-01T09:00:00Z" {
		t.Fatalf("started_at from transcript span = %q, want 2026-07-01T09:00:00Z", got)
	}

	// A second pass finds nothing un-hydrated → zero additional fetches (fast).
	for k := range fetched {
		delete(fetched, k)
	}
	if n := enrichRecentForAutoRefresh(ctx, s.DB(), c); n != 0 {
		t.Fatalf("second pass enriched %d, want 0 (all notes already hydrated)", n)
	}
	if len(fetched) != 0 {
		t.Fatalf("second pass made HTTP fetches %v — steady state must be fetch-free", fetched)
	}
}

// TestEnrichRecentForAutoRefresh_NilSafe: nil db/client are no-ops (best-effort).
func TestEnrichRecentForAutoRefresh_NilSafe(t *testing.T) {
	if n := enrichRecentForAutoRefresh(context.Background(), nil, nil); n != 0 {
		t.Fatalf("nil inputs should no-op, got %d", n)
	}
}

// TestAutoRefreshEnrichCap_DogfoodShrinks: the cap shrinks under the dogfood
// runner so the pre-command enrich fits its per-command timeout.
func TestAutoRefreshEnrichCap_DogfoodShrinks(t *testing.T) {
	if got := autoRefreshEnrichCap(); got != 25 {
		t.Fatalf("default cap = %d, want 25", got)
	}
	t.Setenv("PRINTING_PRESS_DOGFOOD", "1")
	if got := autoRefreshEnrichCap(); got != 1 {
		t.Fatalf("dogfood cap = %d, want 1", got)
	}
}

// newCleanStore opens the store the way a FRESH INSTALL does: the generic
// schema (notes/folders from the spec) exists, but granola.EnsureSchema has NOT
// run, so the rich tables (meetings/attendees/transcript_segments) do not exist.
// This reproduces the clean-install state the auto-refresh enrich must create
// the schema for itself.
func newCleanStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.OpenWithContext(context.Background(), filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// TestEnrichRecentForAutoRefresh_CleanInstallCreatesRichTables is the FIX-7
// regression guard the Codex re-review demanded: on a clean install the rich
// `meetings` table does not exist yet, so the incremental LEFT JOIN would fail
// "no such table: meetings" and the swallowed error would leave the tables empty
// forever. The enrich must EnsureSchema itself first. This test uses a CLEAN
// store (no pre-created granola schema) precisely so the schema-order bug cannot
// hide behind a fixture that already ran EnsureSchema.
func TestEnrichRecentForAutoRefresh_CleanInstallCreatesRichTables(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	ctx := context.Background()
	s := newCleanStore(t)

	// Precondition: the rich table genuinely does not exist yet — else the test
	// would not exercise the schema-order path.
	if _, err := s.DB().ExecContext(ctx, `SELECT 1 FROM meetings LIMIT 1`); err == nil {
		t.Fatal("precondition failed: meetings table already exists; test cannot catch the schema-order bug")
	}

	insertNote(t, s, "not_clean", "2026-07-03T00:00:00Z")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/v1/notes/")
		_ = json.NewEncoder(w).Encode(restNoteDetail{
			ID: id, Title: "Meeting " + id,
			CreatedAt: "2026-07-01T09:05:00Z", UpdatedAt: "2026-07-01T09:40:00Z",
			Transcript: ptrSeg(restSegment{Text: "hi", StartTime: "2026-07-01T09:00:00Z", EndTime: "2026-07-01T09:30:00Z"}),
		})
	}))
	defer srv.Close()

	c := client.New(&config.Config{BaseURL: srv.URL, GranolaApiKey: "test-key"}, 0, 0)
	c.NoCache = true

	if n := enrichRecentForAutoRefresh(ctx, s.DB(), c); n != 1 {
		t.Fatalf("clean-install enrich populated %d meetings, want 1 (schema-order bug swallows the error and returns 0)", n)
	}
	if got := countRows(t, s, "SELECT COUNT(*) FROM meetings WHERE id='not_clean'"); got != 1 {
		t.Fatalf("meetings row after clean-install enrich = %d, want 1", got)
	}
	if got := countRows(t, s, "SELECT COUNT(*) FROM transcript_segments WHERE meeting_id='not_clean'"); got != 1 {
		t.Fatalf("transcript_segments after clean-install enrich = %d, want 1", got)
	}
}

// TestUnhydratedRecentNoteIDs_ReEnrichesUpdatedNote is the FIX-7 [P2] guard: a
// note whose updated_at advanced past its stored meeting (partial first pass, or
// the note gained transcription later) is selected again — not considered done
// forever just because a meetings row exists.
func TestUnhydratedRecentNoteIDs_ReEnrichesUpdatedNote(t *testing.T) {
	ctx := context.Background()
	s := newHydrateTestStore(t)
	insertNote(t, s, "not_u", "2026-07-05T00:00:00Z") // note last updated 07-05
	// Meeting was hydrated from an EARLIER note revision (partial first pass).
	if err := upsertRichMeeting(ctx, s.DB(), &restNoteDetail{ID: "not_u", Title: "m", UpdatedAt: "2026-07-01T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	ids, err := unhydratedRecentNoteIDs(ctx, s.DB(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(ids, ",") != "not_u" {
		t.Fatalf("a note updated after its meeting should re-enrich: got %v, want [not_u]", ids)
	}
	// Re-hydrate at the note's current revision → now up to date, excluded.
	if err := upsertRichMeeting(ctx, s.DB(), &restNoteDetail{ID: "not_u", Title: "m", UpdatedAt: "2026-07-05T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	if ids, _ := unhydratedRecentNoteIDs(ctx, s.DB(), 0); len(ids) != 0 {
		t.Fatalf("re-hydrated-at-current note should be done: got %v", ids)
	}
}
