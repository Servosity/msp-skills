// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated; handfixes.json store-open-skips-current-schema).

package store

import (
	"database/sql"
	"path/filepath"
	"testing"
)

// An open with the stamp on the file applies nothing; an open with another
// stamp (another schema, or a file an older build migrated) applies everything.
func TestMigrationsRunOnlyWhenTheStampDiffers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data.db")
	reopen := func() *sql.DB {
		t.Helper()
		s, err := Open(path)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = s.Close() })
		return s.DB()
	}
	tableExists := func(db *sql.DB) bool {
		t.Helper()
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'issue_watch_state'`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n == 1
	}
	db := reopen()
	if !tableExists(db) {
		t.Fatal("a fresh file was not migrated")
	}
	if _, err := db.Exec(`DROP TABLE issue_watch_state`); err != nil {
		t.Fatal(err)
	}
	if db = reopen(); tableExists(db) {
		t.Fatal("an open with the stamp on the file ran the migrations")
	}
	if _, err := db.Exec(`PRAGMA application_id = 1`); err != nil {
		t.Fatal(err)
	}
	if db = reopen(); !tableExists(db) {
		t.Fatal("an open with another stamp did not run the migrations")
	}
	if a, b := migrationStamp([]string{"a"}), migrationStamp([]string{"b"}); a == b || a < 0 {
		t.Fatalf("the stamp must follow the migration text and fit the header field: %d, %d", a, b)
	}
}
