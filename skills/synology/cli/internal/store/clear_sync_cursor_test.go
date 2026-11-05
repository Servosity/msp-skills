// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/synology/handfixes.json
// under the "sync-clear-never-marks-complete" entry.

package store

import (
	"path/filepath"
	"testing"
)

// TestClearSyncCursorNeverMarksComplete: a cleared collection reads as an
// incomplete attempt until its walk finishes, never as synced.
func TestClearSyncCursorNeverMarksComplete(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.SaveSyncState("user", "cursor-9", 40); err != nil {
		t.Fatal(err)
	}
	if err := s.ClearSyncCursor("user"); err != nil {
		t.Fatal(err)
	}
	if err := s.ClearSyncCursor("group"); err != nil {
		t.Fatal(err)
	}
	for _, rt := range []string{"user", "group"} {
		var cursor string
		var count, complete int
		if err := s.DB().QueryRow(`SELECT last_cursor, total_count, last_attempt_complete FROM sync_state WHERE resource_type = ?`, rt).Scan(&cursor, &count, &complete); err != nil {
			t.Fatal(err)
		}
		if cursor != "" || count != 0 || complete != 0 {
			t.Fatalf("%s after clear: cursor=%q count=%d complete=%d, want empty, 0, 0", rt, cursor, count, complete)
		}
	}
}
