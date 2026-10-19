// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package store

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

// TestRestoreLedgerMigrationFromOldSchema opens a ledger written before the
// resume_key / auth_scope columns and the (batch, entity, requested) index
// existed, with a duplicate row for one (entity, requested) pair.
func TestRestoreLedgerMigrationFromOldSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	stmts := []string{
		`CREATE TABLE restore_batches (
			batch_id TEXT PRIMARY KEY, batch_key TEXT NOT NULL, created_at TEXT NOT NULL, scope TEXT NOT NULL,
			account_id TEXT NOT NULL DEFAULT '', task_id TEXT NOT NULL, source TEXT NOT NULL DEFAULT '',
			label TEXT NOT NULL DEFAULT '', dates TEXT NOT NULL DEFAULT '[]', options TEXT NOT NULL DEFAULT '{}', last_refresh_at TEXT)`,
		`CREATE TABLE restore_jobs (
			id INTEGER PRIMARY KEY AUTOINCREMENT, batch_id TEXT NOT NULL REFERENCES restore_batches(batch_id),
			entity TEXT NOT NULL, ext_id TEXT NOT NULL DEFAULT '', requested TEXT NOT NULL, resolved_raw INTEGER NOT NULL,
			resolved TEXT NOT NULL, sub_source TEXT NOT NULL DEFAULT '', status TEXT NOT NULL, http_status INTEGER,
			response TEXT, error TEXT, task_id TEXT, progress TEXT, submitted_at TEXT, updated_at TEXT NOT NULL,
			UNIQUE(batch_id, entity, resolved_raw))`,
		`INSERT INTO restore_batches (batch_id, batch_key, created_at, scope, task_id) VALUES ('b1', 'k', '2026-10-01T00:00:00Z', 'account', 't')`,
		`INSERT INTO restore_jobs (batch_id, entity, requested, resolved_raw, resolved, status, updated_at)
			VALUES ('b1', 'a@contoso.com', '2026-10-01', 20260930120000, 'x', 'SUBMITTED', 'now')`,
		`INSERT INTO restore_jobs (batch_id, entity, requested, resolved_raw, resolved, status, updated_at)
			VALUES ('b1', 'a@contoso.com', '2026-10-01', 20261001120000, 'y', 'PLANNED', 'now')`,
		`INSERT INTO restore_jobs (batch_id, entity, requested, resolved_raw, resolved, status, updated_at)
			VALUES ('b1', 'b@contoso.com', '2026-10-01', 20260930120000, 'x', 'PLANNED', 'now')`,
	}
	for _, s := range stmts {
		if _, err := raw.Exec(s); err != nil {
			t.Fatalf("%s: %v", s, err)
		}
	}
	_ = raw.Close()

	for i := 0; i < 2; i++ { // the second open proves the migration is idempotent
		st, err := Open(path)
		if err != nil {
			t.Fatalf("open %d: %v", i, err)
		}
		db := st.DB()
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM restore_jobs WHERE batch_id='b1' AND entity='a@contoso.com'`).Scan(&n); err != nil || n != 1 {
			t.Fatalf("duplicate rows after migration = %d (%v)", n, err)
		}
		var status string
		if err := db.QueryRow(`SELECT status FROM restore_jobs WHERE entity='a@contoso.com'`).Scan(&status); err != nil || status != "SUBMITTED" {
			t.Fatalf("kept row status = %q (%v), want the oldest (SUBMITTED)", status, err)
		}
		if _, err := db.Exec(`UPDATE restore_batches SET resume_key='r', auth_scope='use-account' WHERE batch_id='b1'`); err != nil {
			t.Fatalf("new columns missing: %v", err)
		}
		_, err = db.Exec(`INSERT INTO restore_jobs (batch_id, entity, requested, resolved_raw, resolved, status, updated_at)
			VALUES ('b1', 'b@contoso.com', '2026-10-01', 20261002120000, 'z', 'PLANNED', 'now')`)
		if err == nil {
			t.Fatal("unique (batch_id, entity, requested) index should reject a second row")
		}
		_ = st.Close()
	}
}
