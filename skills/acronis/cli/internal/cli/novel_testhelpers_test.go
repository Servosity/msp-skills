// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"path/filepath"
	"testing"

	"acronis-pp-cli/internal/store"
)

// testStorePaths maps a *store.Store back to the on-disk path it was opened
// from, so command-level tests can pass --db.
var testStorePaths = map[*store.Store]string{}

// newNovelTestStore opens a fresh temp-dir store (with extras migrated) for a
// novel-command test. Closed automatically at test end.
func newNovelTestStore(t *testing.T) *store.Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "data.db")
	db, err := store.OpenWithContext(context.Background(), path)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	testStorePaths[db] = path
	t.Cleanup(func() { _ = db.Close(); delete(testStorePaths, db) })
	return db
}

// dbPathOf returns the on-disk path for a store opened via newNovelTestStore.
// The command under test opens its OWN connection to the same file, so the
// caller should flush all writes (they are committed synchronously by Exec)
// before invoking the command.
func dbPathOf(t *testing.T, db *store.Store) string {
	t.Helper()
	p, ok := testStorePaths[db]
	if !ok {
		t.Fatalf("store path not recorded; use newNovelTestStore")
	}
	return p
}

func mustExec(t *testing.T, db *store.Store, query string, args ...any) {
	t.Helper()
	if _, err := db.DB().Exec(query, args...); err != nil {
		t.Fatalf("exec %q: %v", query, err)
	}
}

// seedTenant inserts a tenant row.
func seedTenant(t *testing.T, db *store.Store, id, name, kind, parentID string) {
	t.Helper()
	mustExec(t, db,
		`INSERT INTO tenants(id, data, name, kind, parent_id) VALUES(?,?,?,?,?)`,
		id, `{"id":"`+id+`","name":"`+name+`"}`, name, kind, nullable(parentID))
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}
