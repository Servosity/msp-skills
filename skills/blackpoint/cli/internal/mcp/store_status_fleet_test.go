// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written (Codex r1 P1 "incomplete fetch read as complete"): fleet-sync
// records its incomplete or failed fetches in fleet_sync_state, and MCP
// search/sql must report the store partial while any such record stands.

package mcp

import (
	"path/filepath"
	"testing"

	"blackpoint-pp-cli/internal/store"
)

func TestStoreStatusPartialWhileFleetSyncRecordsAGap(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.SaveSyncState("tenants", "", 2); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`CREATE TABLE fleet_sync_state (kind TEXT NOT NULL, scope TEXT NOT NULL, synced_at TEXT NOT NULL,
			row_count INTEGER NOT NULL, error TEXT NOT NULL DEFAULT '', PRIMARY KEY (kind, scope))`,
		`INSERT INTO fleet_sync_state VALUES ('detection', 't1', '2026-10-27T00:00:00Z', 1, 'incomplete: GET /alert-groups returned 1 of 2 rows')`,
	} {
		if _, err := db.DB().Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	if st, err := mcpStoreStatus(db); err != nil || st != mcpStoreStatusPartial {
		t.Fatalf("fleet gap recorded: want %q, got %q (err=%v)", mcpStoreStatusPartial, st, err)
	}
	if _, err := db.DB().Exec(`UPDATE fleet_sync_state SET error = ''`); err != nil {
		t.Fatal(err)
	}
	if st, err := mcpStoreStatus(db); err != nil || st != mcpStoreStatusReady {
		t.Fatalf("clean fleet state: want %q, got %q (err=%v)", mcpStoreStatusReady, st, err)
	}
}
