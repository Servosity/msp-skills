package store

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

// Halo numbers ticket actions within their ticket, so action 1 exists on every
// ticket. Before actions was added to resourceParentKeyColumns the storage key
// was the bare action id, so each ticket's action 1 overwrote the previous
// one's and 8185 synced actions collapsed to 149 stored rows (#203).
func TestActionsStorageKeyIsQualifiedByTicket(t *testing.T) {
	action := func(id, ticketID float64) map[string]any {
		return map[string]any{"id": id, "ticket_id": ticketID}
	}

	first := resourceStorageID("actions", "1", action(1, 13))
	second := resourceStorageID("actions", "1", action(1, 14))

	if first == second {
		t.Fatalf("action 1 of ticket 13 and action 1 of ticket 14 share storage key %q; "+
			"one silently overwrites the other", first)
	}
	if first == "1" {
		t.Fatalf("actions storage key %q is the bare action id; it must be qualified by ticket_id", first)
	}
}

// A missing parent must degrade to the bare id rather than dropping the row.
func TestActionsStorageKeyFallsBackWithoutTicket(t *testing.T) {
	got := resourceStorageID("actions", "7", map[string]any{"id": float64(7)})
	if got != "7" {
		t.Fatalf("expected bare id %q when ticket_id is absent, got %q", "7", got)
	}
}

// Re-upserting the same action must not create a second row: the qualified key
// has to be stable across syncs.
func TestActionsUpsertIsIdempotentPerTicket(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer s.Close()

	batch := []json.RawMessage{
		json.RawMessage(`{"id":1,"ticket_id":13}`),
		json.RawMessage(`{"id":1,"ticket_id":14}`),
		json.RawMessage(`{"id":2,"ticket_id":13}`),
	}
	if _, _, err := s.UpsertBatch("actions", batch); err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	if _, _, err := s.UpsertBatch("actions", batch); err != nil {
		t.Fatalf("second upsert: %v", err)
	}

	got, err := s.Count("actions")
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if got != len(batch) {
		t.Fatalf("expected %d distinct stored actions after two identical syncs, got %d", len(batch), got)
	}
}

// Rows cached before the key change keep their bare-id keys. Without a purge a
// resync adds the qualified rows alongside them and every reader sees a stale
// unqualified duplicate on top of the correct set (#203).
func TestLegacyActionKeysArePurgedOnUpgrade(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "t.db")

	s, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	// Simulate a pre-fix cache: actions written under bare ids, on the old
	// schema version.
	if _, err := s.DB().Exec(
		`INSERT INTO resources (id, resource_type, data) VALUES ('1','actions','{"id":1,"ticket_id":13}')`,
	); err != nil {
		t.Fatalf("seed legacy row: %v", err)
	}
	if _, err := s.DB().Exec(`PRAGMA user_version = 4`); err != nil {
		t.Fatalf("stamp old version: %v", err)
	}
	s.Close()

	// Reopening runs migrate(), which must purge the legacy rows.
	s2, err := Open(dbPath)
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	defer s2.Close()

	var legacy int
	if err := s2.DB().QueryRow(
		`SELECT COUNT(*) FROM resources WHERE resource_type = 'actions' AND id = '1'`,
	).Scan(&legacy); err != nil {
		t.Fatalf("count legacy: %v", err)
	}
	if legacy != 0 {
		t.Fatalf("legacy bare-id action row survived the upgrade; a resync would leave it as a "+
			"stale duplicate alongside the ticket-qualified row (found %d)", legacy)
	}

	v, err := s2.SchemaVersion()
	if err != nil {
		t.Fatalf("schema version: %v", err)
	}
	if v != StoreSchemaVersion {
		t.Fatalf("schema version = %d, want %d", v, StoreSchemaVersion)
	}
}

// A cache written by a binary old enough never to have stamped user_version
// still holds bare-id action rows, so the purge must not skip version 0.
func TestLegacyActionKeysPurgedOnUnstampedDatabase(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "t.db")

	s, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	if _, err := s.DB().Exec(
		`INSERT INTO resources (id, resource_type, data) VALUES ('1','actions','{"id":1,"ticket_id":13}')`,
	); err != nil {
		t.Fatalf("seed legacy row: %v", err)
	}
	if _, err := s.DB().Exec(`PRAGMA user_version = 0`); err != nil {
		t.Fatalf("unstamp: %v", err)
	}
	s.Close()

	s2, err := Open(dbPath)
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	defer s2.Close()

	var legacy int
	if err := s2.DB().QueryRow(
		`SELECT COUNT(*) FROM resources WHERE resource_type = 'actions' AND id = '1'`,
	).Scan(&legacy); err != nil {
		t.Fatalf("count legacy: %v", err)
	}
	if legacy != 0 {
		t.Fatalf("legacy action row survived on an unstamped database (found %d)", legacy)
	}
}

// The purge must touch ticket actions only.
func TestPurgeLeavesOtherResourcesIntact(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "t.db")

	s, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	if _, err := s.DB().Exec(
		`INSERT INTO resources (id, resource_type, data) VALUES
			('1','actions','{"id":1}'),
			('5','tickets','{"id":5}'),
			('9','agent','{"id":9,"name":"Ada"}')`,
	); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := s.DB().Exec(`PRAGMA user_version = 4`); err != nil {
		t.Fatalf("stamp: %v", err)
	}
	s.Close()

	s2, err := Open(dbPath)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer s2.Close()

	var survivors int
	if err := s2.DB().QueryRow(
		`SELECT COUNT(*) FROM resources WHERE resource_type IN ('tickets','agent')`,
	).Scan(&survivors); err != nil {
		t.Fatalf("count: %v", err)
	}
	if survivors != 2 {
		t.Fatalf("purge removed non-action rows: expected 2 survivors, got %d", survivors)
	}
}
