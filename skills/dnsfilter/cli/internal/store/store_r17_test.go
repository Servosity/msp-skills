// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-fix regression tests (handfixes.json: sync-non-object-item-is-incomplete,
// sync-seen-ids-are-stored-ids, read-only-open-never-hides-wal), Codex round 17.

package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

// Every list item that is not a JSON object is a row the API returned that
// was not stored, so it is an extract failure, never just skipped: sync reads
// extractFailures to refuse completion (and the prune that follows it).
func TestUpsertBatchCountsNonObjectItems(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	items := []json.RawMessage{
		json.RawMessage(`{"id":"1","type":"users","attributes":{"email":"a@example.com"}}`),
		json.RawMessage(`42`), json.RawMessage(`"x"`), json.RawMessage(`null`), json.RawMessage(`[1]`), json.RawMessage(`true`),
		json.RawMessage(`{"id":`), // undecodable
	}
	stored, extractFailures, typedFailures, err := s.UpsertBatchDetailed("users", items)
	if err != nil || stored != 1 || typedFailures != 0 {
		t.Fatalf("stored=%d typedFailures=%d err=%v, want 1 stored", stored, typedFailures, err)
	}
	if extractFailures != len(items)-1 {
		t.Fatalf("extractFailures=%d, want %d (every non-object item)", extractFailures, len(items)-1)
	}
}

// UpsertBatchStoredIDs returns exactly the bare ids the writer keyed its rows
// by: an ID-bearing envelope is unwrapped first, a parent-keyed child's
// storage key loses its parent suffix, and an item that was not stored
// (an extract failure) contributes nothing.
func TestUpsertBatchStoredIDsAreTheStoredIdentity(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ids, stored, extractFailures, _, err := s.UpsertBatchStoredIDs("users", []json.RawMessage{
		json.RawMessage(`{"result":{"id":"7","type":"users","attributes":{"email":"a@example.com"}}}`),
		json.RawMessage(`{"id":"8","type":"users"}`),
		json.RawMessage(`42`),
	})
	if err != nil || stored != 2 || extractFailures != 1 {
		t.Fatalf("stored=%d extractFailures=%d err=%v", stored, extractFailures, err)
	}
	if !reflect.DeepEqual(ids, []string{"7", "8"}) {
		t.Fatalf("stored ids = %q, want [7 8]", ids)
	}
	got, err := s.ListIDs("users")
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range ids {
		found := false
		for _, g := range got {
			if BareResourceID(g) == id {
				found = true
			}
		}
		if !found {
			t.Fatalf("returned id %q is not a stored row (%q)", id, got)
		}
	}
	// A parent-keyed child: the storage key carries the parent, the returned
	// id is the bare one ReconcilePartition compares against.
	var child string
	for rt, keys := range resourceParentKeyColumns {
		if len(keys) > 0 {
			child = rt
			break
		}
	}
	if child == "" {
		t.Skip("no parent-keyed resource in this print")
	}
	key := resourceParentKeyColumns[child][0]
	ids, _, _, _, err = s.UpsertBatchStoredIDs(child, []json.RawMessage{json.RawMessage(`{"id":"2001","` + key + `":"50"}`)})
	if err != nil || !reflect.DeepEqual(ids, []string{"2001"}) {
		t.Fatalf("%s: ids=%q err=%v, want the bare id 2001", child, ids, err)
	}
	if n, err := s.ReconcileAll(child, ids, "", nil); err != nil || n != 0 {
		t.Fatalf("%s: reconcile with the returned ids deleted %d (err %v), want 0", child, n, err)
	}
}

// A read-only open that cannot create the -shm WAL index must not silently
// fall back to immutable=1 while committed frames sit in the -wal: that read
// would miss them (stale rows, a stale complete flag). It either reads the
// WAL or refuses with the condition named. A read-only directory is the real
// trigger (a directory at the -shm path still opens with a heap index), so
// the test skips where directory modes are not enforced.
func TestOpenReadOnlyNeverHidesCommittedWAL(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory mode bits are not enforced on Windows")
	}
	src := t.TempDir()
	srcPath := filepath.Join(src, "data.db")
	w, err := Open(srcPath)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	// The main file holds the schema, one row and an incomplete sync state.
	if err := w.Upsert("users", "checkpointed", json.RawMessage(`{"id":"checkpointed"}`)); err != nil {
		t.Fatal(err)
	}
	if err := w.SaveSyncProgress("users", "", 1); err != nil {
		t.Fatal(err)
	}
	if _, err := w.DB().Exec(`PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		t.Fatal(err)
	}
	if _, err := w.DB().Exec(`PRAGMA wal_autocheckpoint=0`); err != nil {
		t.Fatal(err)
	}
	// Committed only in the WAL: a second row and a complete sync.
	if err := w.Upsert("users", "wal-only", json.RawMessage(`{"id":"wal-only"}`)); err != nil {
		t.Fatal(err)
	}
	if err := w.SaveSyncState("users", "", 2); err != nil {
		t.Fatal(err)
	}
	dst := t.TempDir()
	dstPath := filepath.Join(dst, "data.db")
	for _, suffix := range []string{"", "-wal"} {
		b, err := os.ReadFile(srcPath + suffix)
		if err != nil {
			t.Fatal(err)
		}
		if suffix == "-wal" && len(b) == 0 {
			t.Fatal("the WAL is empty; the fixture holds no uncheckpointed frames")
		}
		if err := os.WriteFile(dstPath+suffix, b, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Chmod(dst, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dst, 0o755) })
	if f, err := os.Create(filepath.Join(dst, "probe")); err == nil {
		f.Close()
		t.Skip("directory mode bits are not enforced here (running as root?)")
	}
	ro, err := OpenReadOnly(dstPath)
	if err != nil {
		if !strings.Contains(err.Error(), "-wal") || !strings.Contains(err.Error(), "checkpoint") {
			t.Fatalf("refusal does not name the WAL condition: %v", err)
		}
		return
	}
	defer ro.Close()
	n, err := ro.Count("users")
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("read-only open silently ignored the committed WAL frames: users=%d, want 2", n)
	}
}
