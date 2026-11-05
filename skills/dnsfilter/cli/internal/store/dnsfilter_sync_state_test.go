// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-fix regression test (handfixes.json: sync-clear-never-marks-complete).

package store

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"
)

func TestClearSyncCursorNeverMarksComplete(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	read := func(rt string) (cursor string, synced sql.NullString, count, complete int) {
		t.Helper()
		if err := s.db.QueryRow(`SELECT COALESCE(last_cursor, ''), CAST(last_synced_at AS TEXT), total_count, last_attempt_complete FROM sync_state WHERE resource_type = ?`, rt).
			Scan(&cursor, &synced, &count, &complete); err != nil {
			t.Fatal(err)
		}
		return
	}

	// A completed walk, then the --full clear.
	if err := s.SaveSyncStateAt("networks", "c-old", 7, time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	if _, _, _, complete := read("networks"); complete != 1 {
		t.Fatalf("precondition: a completed walk reads complete=%d", complete)
	}
	if err := s.ClearSyncCursor("networks"); err != nil {
		t.Fatal(err)
	}
	cursor, synced, count, complete := read("networks")
	if complete != 0 || cursor != "" || synced.Valid || count != 0 {
		t.Fatalf("after clear: complete=%d cursor=%q last_synced_at=%v count=%d, want 0/''/NULL/0", complete, cursor, synced, count)
	}
	if c, at, n, err := s.GetSyncState("networks"); err != nil || c != "" || !at.IsZero() || n != 0 {
		t.Fatalf("GetSyncState after clear = %q %v %d %v", c, at, n, err)
	}

	// A clear of a collection with no prior row.
	if err := s.ClearSyncCursor("users"); err != nil {
		t.Fatal(err)
	}
	if _, synced, _, complete := read("users"); complete != 0 || synced.Valid {
		t.Fatalf("a fresh clear reads complete=%d last_synced_at=%v", complete, synced)
	}

	// Only the walk's final save marks it complete again.
	if err := s.SaveSyncStateAt("networks", "", 9, time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, _, _, complete := read("networks"); complete != 1 {
		t.Fatal("a completed walk after the clear does not read complete")
	}
}
