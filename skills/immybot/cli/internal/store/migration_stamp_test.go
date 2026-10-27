// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package store

import (
	"path/filepath"
	"testing"
)

// The migration statements run when a store is new or names another list, and
// not on an open that has nothing to do: under the credential kit each one costs
// about 15 ms, and a command opens the store several times.
func TestMigrationsRunOnlyWhenTheStampDiffers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.db")
	hasAccess := func(s *Store) bool {
		t.Helper()
		var n int
		if err := s.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name = 'access'`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n == 1
	}
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if !hasAccess(s) {
		t.Fatal("a new store did not run the migrations")
	}
	if _, err := s.db.Exec(`DROP TABLE access`); err != nil {
		t.Fatal(err)
	}
	s.Close()

	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if hasAccess(s) {
		t.Fatal("a store already at this list ran the migrations again")
	}
	if _, err := s.db.Exec(`PRAGMA application_id = 1`); err != nil {
		t.Fatal(err)
	}
	s.Close()

	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if !hasAccess(s) {
		t.Fatal("a store stamped with another list did not run the migrations")
	}
}
