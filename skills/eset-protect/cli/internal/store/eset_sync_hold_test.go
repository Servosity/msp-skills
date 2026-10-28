// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"
)

// TestHeldSyncStateIsNeverWritten (handfixes.json:
// filtered-sync-never-prunes-or-completes): a resource a narrowed sync holds
// keeps the row the last unfiltered sync wrote, through every writer of a
// single resource's sync_state, scoped or not.
func TestHeldSyncStateIsNeverWritten(t *testing.T) {
	s, err := OpenWithContext(context.Background(), filepath.Join(t.TempDir(), "hold.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	if _, _, _, err := s.BindSyncScope("devices", "scope-a"); err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	if err := s.SaveSyncStateAtInScope("devices", "scope-a", "cur", 7, at); err != nil {
		t.Fatal(err)
	}
	row := func() string {
		var cursor, scope sql.NullString
		var synced sql.NullString
		var count, complete int
		if err := s.db.QueryRow(`SELECT last_cursor, last_synced_at, total_count, last_attempt_complete, scope_key FROM sync_state WHERE resource_type = 'devices'`).
			Scan(&cursor, &synced, &count, &complete, &scope); err != nil {
			t.Fatal(err)
		}
		return cursor.String + "|" + synced.String + "|" + scope.String + "|" + string(rune('0'+count)) + string(rune('0'+complete))
	}
	want := row()
	s.HoldSyncState("devices")
	later := at.Add(time.Hour)
	writers := map[string]func() error{
		"SaveSyncState":                      func() error { return s.SaveSyncState("devices", "x", 1) },
		"SaveSyncStateAt":                    func() error { return s.SaveSyncStateAt("devices", "x", 1, later) },
		"SaveSyncProgress":                   func() error { return s.SaveSyncProgress("devices", "x", 1) },
		"SaveSyncStateInScope":               func() error { return s.SaveSyncStateInScope("devices", "scope-a", "x", 1) },
		"SaveSyncStateAtInScope":             func() error { return s.SaveSyncStateAtInScope("devices", "scope-a", "x", 1, later) },
		"SaveSyncProgressInScope":            func() error { return s.SaveSyncProgressInScope("devices", "scope-a", "x", 1) },
		"SaveSyncCursor":                     func() error { return s.SaveSyncCursor("devices", "x") },
		"ClearSyncScopeIfCurrent":            func() error { return s.ClearSyncScopeIfCurrent("devices", "scope-a") },
		"MarkSyncAttemptIncompleteIfCurrent": func() error { return s.MarkSyncAttemptIncompleteIfCurrent("devices", "scope-a") },
		"BindSyncScope (other scope)": func() error {
			_, _, _, err := s.BindSyncScope("devices", "scope-b")
			return err
		},
	}
	for name, write := range writers {
		if err := write(); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got := row(); got != want {
			t.Fatalf("%s rewrote a held sync_state row: %q -> %q", name, want, got)
		}
	}
}
