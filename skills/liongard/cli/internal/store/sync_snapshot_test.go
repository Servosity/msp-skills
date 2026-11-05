// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package store

// r2-narrowed-sync-is-not-a-snapshot: only the hand writer may record a full
// snapshot, and every other write to a checkpoint clears the claim.

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"
)

func openSnapshotStore(t *testing.T) (*Store, string) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "data.db")
	s, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s, dbPath
}

// bindFullSnapshot runs the sync walk's state sequence: bind the scope,
// mark the attempt started, then record a finished full snapshot.
func bindFullSnapshot(t *testing.T, s *Store, resource, scope string) {
	t.Helper()
	if _, _, _, err := s.BindSyncScope(resource, scope); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveSyncProgressInScope(resource, scope, "", 0); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveSyncSnapshotAtInScope(resource, scope, "", 3, time.Now(), true); err != nil {
		t.Fatal(err)
	}
	snap, err := s.SyncSnapshotState(resource)
	if err != nil || !snap.Recorded || !snap.Complete || !snap.Full {
		t.Fatalf("a full snapshot was not recorded: %+v %v", snap, err)
	}
	if !s.SyncSnapshotFullInScope(resource, scope) {
		t.Fatalf("SyncSnapshotFullInScope(%q) is false after a full snapshot", scope)
	}
}

func TestSyncSnapshotOnlyTheHandWriterClaimsFull(t *testing.T) {
	s, _ := openSnapshotStore(t)
	const rt, scope = "agents", "template:instance=acme"

	if snap, err := s.SyncSnapshotState(rt); err != nil || snap.Recorded {
		t.Fatalf("a never-synced resource reads as recorded: %+v %v", snap, err)
	}

	// A narrowed walk finishes: complete, not full.
	if _, _, _, err := s.BindSyncScope(rt, scope); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveSyncSnapshotAtInScope(rt, scope, "", 1, time.Now(), false); err != nil {
		t.Fatal(err)
	}
	if snap, _ := s.SyncSnapshotState(rt); !snap.Complete || snap.Full {
		t.Fatalf("a narrowed finished walk: want complete and not full, got %+v", snap)
	}

	// Every generated writer that touches the checkpoint clears the claim.
	writers := map[string]func() error{
		"SaveSyncState":           func() error { return s.SaveSyncState(rt, "", 0) },
		"SaveSyncStateAt":         func() error { return s.SaveSyncStateAt(rt, "", 0, time.Now()) },
		"SaveSyncStateAtInScope":  func() error { return s.SaveSyncStateAtInScope(rt, scope, "", 0, time.Now()) },
		"SaveSyncStateInScope":    func() error { return s.SaveSyncStateInScope(rt, scope, "", 0) },
		"SaveSyncProgress":        func() error { return s.SaveSyncProgress(rt, "c", 0) },
		"SaveSyncProgressInScope": func() error { return s.SaveSyncProgressInScope(rt, scope, "c", 0) },
		"SaveSyncCursor":          func() error { return s.SaveSyncCursor(rt, "c") },
		"ClearSyncScopeIfCurrent": func() error { return s.ClearSyncScopeIfCurrent(rt, scope) },
		"MarkSyncAttemptIncompleteIfCurrent": func() error {
			return s.MarkSyncAttemptIncompleteIfCurrent(rt, scope)
		},
		"BindSyncScope (another tenant)": func() error {
			_, _, _, err := s.BindSyncScope(rt, "template:instance=other")
			return err
		},
	}
	for name, write := range writers {
		// Each case starts from a fresh full snapshot in scope.
		bindFullSnapshot(t, s, rt, scope)
		if err := write(); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		snap, err := s.SyncSnapshotState(rt)
		if err != nil {
			t.Fatalf("%s: reading state: %v", name, err)
		}
		if snap.Full {
			t.Errorf("%s left the checkpoint claiming a full snapshot: %+v", name, snap)
		}
		if s.SyncSnapshotFullInScope(rt, scope) {
			t.Errorf("%s left SyncSnapshotFullInScope true", name)
		}
	}

	// A row rebound to another scope cannot take a snapshot verdict.
	if _, _, _, err := s.BindSyncScope(rt, "template:instance=other"); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveSyncSnapshotAtInScope(rt, scope, "", 1, time.Now(), true); err == nil {
		t.Errorf("a snapshot was recorded against a scope the row no longer holds")
	}
}

// A store created before the column existed opens with the column at 0: a
// legacy finished checkpoint does not prove a full snapshot.
func TestSyncSnapshotLegacyCheckpointIsNotFull(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "data.db")
	raw, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`CREATE TABLE sync_state (
		resource_type TEXT PRIMARY KEY,
		last_cursor TEXT,
		last_synced_at DATETIME,
		total_count INTEGER DEFAULT 0,
		last_attempt_complete INTEGER NOT NULL DEFAULT 0,
		scope_key TEXT NOT NULL DEFAULT ''
	)`); err != nil {
		raw.Close()
		t.Fatal(err)
	}
	if _, err := raw.Exec(`INSERT INTO sync_state VALUES ('agents', '', '2026-11-01T00:00:00Z', 4, 1, '')`); err != nil {
		raw.Close()
		t.Fatal(err)
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ { // the second open proves the migration is idempotent
		s, err := Open(dbPath)
		if err != nil {
			t.Fatalf("open %d: %v", i, err)
		}
		snap, err := s.SyncSnapshotState("agents")
		s.Close()
		if err != nil || !snap.Complete || snap.Full {
			t.Fatalf("open %d: a legacy finished checkpoint: want complete and not full, got %+v %v", i, snap, err)
		}
	}
}

// The verdict survives a reopen (it is stored, not cached).
func TestSyncSnapshotPersists(t *testing.T) {
	s, dbPath := openSnapshotStore(t)
	bindFullSnapshot(t, s, "metrics", "")
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	re, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer re.Close()
	if snap, err := re.SyncSnapshotState("metrics"); err != nil || !snap.Full {
		t.Fatalf("after reopen: %+v %v", snap, err)
	}
}
