// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: local tables for health snapshots and the restore-test ledger.
// Neither table ever holds a credential.

package cli

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"time"

	"dropsuite-pp-cli/internal/store"
)

const fleetSchema = `
CREATE TABLE IF NOT EXISTS ds_health_snapshots_v2 (
	run_id      TEXT NOT NULL,
	taken_at    TEXT NOT NULL,
	org_id      TEXT NOT NULL,
	org_email   TEXT NOT NULL DEFAULT '',
	kind        TEXT NOT NULL,
	object      TEXT NOT NULL,
	object_id   TEXT NOT NULL,
	status      TEXT NOT NULL DEFAULT '',
	last_backup TEXT NOT NULL DEFAULT '',
	items       INTEGER,
	storage     INTEGER,
	grade       TEXT NOT NULL,
	reason      TEXT NOT NULL DEFAULT '',
	PRIMARY KEY (run_id, org_id, kind, object_id)
);
CREATE INDEX IF NOT EXISTS ds_health_snapshots_v2_taken ON ds_health_snapshots_v2(taken_at);
CREATE TABLE IF NOT EXISTS ds_restore_tests (
	id          INTEGER PRIMARY KEY AUTOINCREMENT,
	org_ref     TEXT NOT NULL,
	object      TEXT NOT NULL,
	kind        TEXT NOT NULL,
	result      TEXT NOT NULL,
	note        TEXT NOT NULL DEFAULT '',
	technician  TEXT NOT NULL DEFAULT '',
	tested_at   TEXT NOT NULL,
	recorded_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS ds_restore_tests_org ON ds_restore_tests(org_ref, tested_at);
`

func fleetDBPath(dbPath string) string {
	if dbPath != "" {
		return dbPath
	}
	return defaultDBPath("dropsuite-cli")
}

// openFleetDB opens (creating if needed) the local store with the fleet tables.
func openFleetDB(ctx context.Context, dbPath string) (*store.Store, error) {
	db, err := store.OpenWithContext(ctx, fleetDBPath(dbPath))
	if err != nil {
		return nil, fmt.Errorf("opening local store: %w", err)
	}
	if _, err := db.DB().ExecContext(ctx, fleetSchema); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("creating fleet tables: %w", err)
	}
	if err := migrateLegacySnapshots(ctx, db.DB()); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrating health snapshots: %w", err)
	}
	return db, nil
}

// hasLegacySnapshots reports whether the pre-object_id snapshot table exists.
func hasLegacySnapshots(ctx context.Context, db *sql.DB) bool {
	var n int
	err := db.QueryRowContext(ctx, `SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = 'ds_health_snapshots'`).Scan(&n)
	return err == nil && n > 0
}

// migrateLegacySnapshots copies rows from the first snapshot table (keyed by
// display name, no object_id) into ds_health_snapshots_v2, using the old
// object as the id, then drops it. Drift across the upgrade compares old
// label keys with new id keys, so those objects read as removed and re-added
// once.
func migrateLegacySnapshots(ctx context.Context, db *sql.DB) error {
	if !hasLegacySnapshots(ctx, db) {
		return nil
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO ds_health_snapshots_v2
		(run_id, taken_at, org_id, org_email, kind, object, object_id, status, last_backup, items, storage, grade, reason)
		SELECT run_id, taken_at, org_id, org_email, kind, object, object, status, last_backup, items, storage, grade, reason
		FROM ds_health_snapshots`); err != nil {
		_ = tx.Rollback()
		return err
	}
	if _, err := tx.ExecContext(ctx, `DROP TABLE ds_health_snapshots`); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// openFleetDBReadOnly opens an existing local store read-only for the
// commands that only read history; it returns (nil, nil) when no store exists
// yet. Missing fleet tables read as empty (see isMissingTable).
func openFleetDBReadOnly(ctx context.Context, dbPath string) (*store.Store, error) {
	path := fleetDBPath(dbPath)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, nil
	}
	db, err := store.OpenReadOnlyContext(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("opening local store: %w", err)
	}
	if hasLegacySnapshots(ctx, db.DB()) {
		// One-time upgrade: migrate read-write, then reopen read-only.
		_ = db.Close()
		rw, err := openFleetDB(ctx, path)
		if err != nil {
			return nil, err
		}
		_ = rw.Close()
		if db, err = store.OpenReadOnlyContext(ctx, path); err != nil {
			return nil, fmt.Errorf("opening local store: %w", err)
		}
	}
	return db, nil
}

func isMissingTable(err error) bool {
	return err != nil && strings.Contains(err.Error(), "no such table")
}

type snapshotRow struct {
	RunID      string
	TakenAt    time.Time
	OrgID      string
	OrgEmail   string
	Kind       string
	Object     string
	ObjectID   string
	Status     string
	LastBackup string
	Items      *int64 // nil: the API returned no count
	Storage    *int64
	Grade      string
	Reason     string
}

func saveHealthSnapshot(ctx context.Context, db *sql.DB, runID string, takenAt time.Time, rows []healthRow) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	stmt, err := tx.PrepareContext(ctx, `INSERT OR REPLACE INTO ds_health_snapshots_v2
		(run_id, taken_at, org_id, org_email, kind, object, object_id, status, last_backup, items, storage, grade, reason)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	// Fixed-width milliseconds keep same-second runs in order when sorted as text.
	ts := takenAt.UTC().Format("2006-01-02T15:04:05.000Z07:00")
	for _, r := range rows {
		id := r.ObjectID
		if id == "" {
			id = r.Object
		}
		if _, err := stmt.ExecContext(ctx, runID, ts, r.OrgID, r.OrgEmail, r.Kind, r.Object, id, r.Status, r.LastBackup, r.Items, r.Storage, r.Grade, r.Reason); err != nil {
			_ = stmt.Close()
			_ = tx.Rollback()
			return err
		}
	}
	if err := stmt.Close(); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

type snapshotRun struct {
	RunID   string
	TakenAt time.Time
}

// listSnapshotRuns returns runs oldest first.
func listSnapshotRuns(ctx context.Context, db *sql.DB) ([]snapshotRun, error) {
	rows, err := db.QueryContext(ctx, `SELECT run_id, MIN(taken_at) FROM ds_health_snapshots_v2 GROUP BY run_id ORDER BY MIN(taken_at), run_id`)
	if isMissingTable(err) {
		return []snapshotRun{}, nil
	}
	if err != nil {
		return nil, err
	}
	runs := make([]snapshotRun, 0)
	for rows.Next() {
		var id, ts string
		if err := rows.Scan(&id, &ts); err != nil {
			_ = rows.Close()
			return nil, err
		}
		t, _ := time.Parse(time.RFC3339, ts)
		runs = append(runs, snapshotRun{RunID: id, TakenAt: t})
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	return runs, rows.Close()
}

func loadSnapshotRows(ctx context.Context, db *sql.DB, runID, orgID string) ([]snapshotRow, error) {
	q := `SELECT run_id, taken_at, org_id, org_email, kind, object, object_id, status, last_backup, items, storage, grade, reason
		FROM ds_health_snapshots_v2 WHERE run_id = ?`
	args := []any{runID}
	if orgID != "" {
		q += ` AND org_id = ?`
		args = append(args, orgID)
	}
	q += ` ORDER BY org_id, kind, object`
	rows, err := db.QueryContext(ctx, q, args...)
	if isMissingTable(err) {
		return []snapshotRow{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := make([]snapshotRow, 0)
	for rows.Next() {
		var r snapshotRow
		var ts string
		var items, storage sql.NullInt64
		if err := rows.Scan(&r.RunID, &ts, &r.OrgID, &r.OrgEmail, &r.Kind, &r.Object, &r.ObjectID, &r.Status, &r.LastBackup, &items, &storage, &r.Grade, &r.Reason); err != nil {
			_ = rows.Close()
			return nil, err
		}
		r.TakenAt, _ = time.Parse(time.RFC3339, ts)
		if items.Valid {
			r.Items = &items.Int64
		}
		if storage.Valid {
			r.Storage = &storage.Int64
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	return out, rows.Close()
}

type restoreTest struct {
	ID         int64  `json:"id"`
	OrgRef     string `json:"org"`
	Object     string `json:"object"`
	Kind       string `json:"kind"`
	Result     string `json:"result"`
	Note       string `json:"note,omitempty"`
	Technician string `json:"technician,omitempty"`
	TestedAt   string `json:"tested_at"`
	RecordedAt string `json:"recorded_at"`
}

func insertRestoreTest(ctx context.Context, db *sql.DB, t restoreTest) (int64, error) {
	res, err := db.ExecContext(ctx, `INSERT INTO ds_restore_tests (org_ref, object, kind, result, note, technician, tested_at, recorded_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, t.OrgRef, t.Object, t.Kind, t.Result, t.Note, t.Technician, t.TestedAt, t.RecordedAt)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// listRestoreTests returns tests newest first, on/after since. Callers pick
// one organization's rows with testsFor, which resolves each reference
// against the known organizations (SQL cannot tell a case-sensitive
// external_id from a case-insensitive email).
func listRestoreTests(ctx context.Context, db *sql.DB, since time.Time) ([]restoreTest, error) {
	q := `SELECT id, org_ref, object, kind, result, note, technician, tested_at, recorded_at FROM ds_restore_tests WHERE 1=1`
	args := []any{}
	if !since.IsZero() {
		q += ` AND tested_at >= ?`
		args = append(args, since.UTC().Format(time.RFC3339))
	}
	q += ` ORDER BY tested_at DESC, id DESC`
	rows, err := db.QueryContext(ctx, q, args...)
	if isMissingTable(err) {
		return []restoreTest{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := make([]restoreTest, 0)
	for rows.Next() {
		var t restoreTest
		if err := rows.Scan(&t.ID, &t.OrgRef, &t.Object, &t.Kind, &t.Result, &t.Note, &t.Technician, &t.TestedAt, &t.RecordedAt); err != nil {
			_ = rows.Close()
			return nil, err
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	return out, rows.Close()
}
