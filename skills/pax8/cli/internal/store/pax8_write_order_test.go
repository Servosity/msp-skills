// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package store

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

// Whole-second updated_at cannot order two writes in one second. The rowid can:
// every write of a resource moves its row past every other row, one row per key,
// and the search index still follows the stored text.
func TestUpsertAdvancesTheRowOnEveryWrite(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "order.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rowid := func(resourceType, id string) int64 {
		t.Helper()
		var n int64
		if err := db.DB().QueryRow(`SELECT rowid FROM resources WHERE resource_type = ? AND id = ?`, resourceType, id).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	upsert := func(resourceType, id, body string) {
		t.Helper()
		if err := db.Upsert(resourceType, id, json.RawMessage(body)); err != nil {
			t.Fatal(err)
		}
	}
	upsert("a", "1", `{"id":"1","note":"firstword"}`)
	upsert("b", "1", `{"id":"1"}`)
	if rowid("a", "1") >= rowid("b", "1") {
		t.Fatalf("a insert after b's: rowids a=%d b=%d", rowid("a", "1"), rowid("b", "1"))
	}
	upsert("a", "1", `{"id":"1","note":"secondword"}`)
	if rowid("a", "1") <= rowid("b", "1") {
		t.Fatalf("an update did not advance: rowids a=%d b=%d", rowid("a", "1"), rowid("b", "1"))
	}
	upsert("b", "1", `{"id":"1"}`)
	if rowid("b", "1") <= rowid("a", "1") {
		t.Fatalf("a repeated write did not advance: rowids a=%d b=%d", rowid("a", "1"), rowid("b", "1"))
	}
	var rows int
	if err := db.DB().QueryRow(`SELECT COUNT(*) FROM resources WHERE resource_type = 'a'`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 1 {
		t.Fatalf("a has %d rows, want 1", rows)
	}
	for word, want := range map[string]int{"secondword": 1, "firstword": 0} {
		var hits int
		if err := db.DB().QueryRow(`SELECT COUNT(*) FROM resources_fts WHERE resources_fts MATCH ?`, word).Scan(&hits); err != nil {
			t.Fatal(err)
		}
		if hits != want {
			t.Fatalf("search for %q: %d hits, want %d", word, hits, want)
		}
	}
}
