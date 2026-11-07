// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored store extension for the fleet-diff novel command. Kept in its
// own file with a lazy initialiser so `generate --force` preserves it and the
// generated migration slice in store.go stays untouched.

package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

const immyFleetSnapshotDDL = `
CREATE TABLE IF NOT EXISTS immy_fleet_snapshots (
	taken_at      TEXT NOT NULL,
	resource_type TEXT NOT NULL,
	id            TEXT NOT NULL,
	fingerprint   TEXT NOT NULL,
	label         TEXT,
	PRIMARY KEY (taken_at, resource_type, id)
)`

const immyFleetSnapshotIndexDDL = `
CREATE INDEX IF NOT EXISTS idx_immy_fleet_snapshots_taken
	ON immy_fleet_snapshots(taken_at)`

// immyFleetSnapshotKindsDDL records, per snapshot generation, every kind the
// writer tracked and the shape it tracked it in. A generation with no rows here
// was written before shapes were recorded (immybot v0.1.x) and is unversioned.
// Additive: a store from before it simply lacks the table.
const immyFleetSnapshotKindsDDL = `
CREATE TABLE IF NOT EXISTS immy_fleet_snapshot_kinds (
	taken_at TEXT NOT NULL,
	kind     TEXT NOT NULL,
	shape    TEXT NOT NULL,
	PRIMARY KEY (taken_at, kind)
)`

// EnsureImmyFleetSnapshots creates the fleet-diff snapshot tables on first use.
// Idempotent, so novel commands can call it unconditionally before writing
// snapshots.
func (s *Store) EnsureImmyFleetSnapshots(ctx context.Context) error {
	for _, stmt := range []string{immyFleetSnapshotDDL, immyFleetSnapshotIndexDDL, immyFleetSnapshotKindsDDL} {
		if _, err := s.DB().ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}

// WriteImmyFleetSnapshot records one snapshot generation, with the shape of
// every kind it tracked (kinds maps kind to shape), in a single transaction.
// It refuses a generation without kinds, or a row of a kind it does not
// record, so nothing writes an unversioned snapshot. It deliberately does not
// call Upsert/UpsertBatch: those open their own write transaction, and SQLite
// WAL permits only one writer, so nesting them inside this transaction could
// busy-wait or fail mid-write.
func (s *Store) WriteImmyFleetSnapshot(ctx context.Context, takenAt string, kinds map[string]string, rows []ImmyFleetRow) error {
	if len(kinds) == 0 {
		return fmt.Errorf("fleet snapshot %s records no tracked kinds", takenAt)
	}
	for kind, shape := range kinds {
		if kind == "" || shape == "" {
			return fmt.Errorf("fleet snapshot %s: kind %q has no shape", takenAt, kind)
		}
	}
	for _, r := range rows {
		if _, ok := kinds[r.ResourceType]; !ok {
			return fmt.Errorf("fleet snapshot %s: row %s/%s is of a kind it does not record", takenAt, r.ResourceType, r.ID)
		}
	}
	if err := s.EnsureImmyFleetSnapshots(ctx); err != nil {
		return err
	}
	tx, err := s.DB().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	for kind, shape := range kinds {
		if _, err := tx.ExecContext(ctx,
			`INSERT OR REPLACE INTO immy_fleet_snapshot_kinds (taken_at, kind, shape) VALUES (?, ?, ?)`,
			takenAt, kind, shape); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	stmt, err := tx.PrepareContext(ctx,
		`INSERT OR REPLACE INTO immy_fleet_snapshots (taken_at, resource_type, id, fingerprint, label) VALUES (?, ?, ?, ?, ?)`)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	for _, r := range rows {
		if _, err := stmt.ExecContext(ctx, takenAt, r.ResourceType, r.ID, r.Fingerprint, r.Label); err != nil {
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

// ImmyFleetRow is one tracked object in a fleet snapshot.
type ImmyFleetRow struct {
	ResourceType string
	ID           string
	Fingerprint  string
	Label        string
}

// ImmyFleetSnapshotTimes returns snapshot generations, newest first. A
// generation of an empty fleet has no object rows, only its recorded kinds.
func (s *Store) ImmyFleetSnapshotTimes(ctx context.Context) ([]string, error) {
	out, err := s.immyFleetSnapshotTimes(ctx,
		`SELECT taken_at FROM immy_fleet_snapshots UNION SELECT taken_at FROM immy_fleet_snapshot_kinds ORDER BY taken_at DESC`)
	if immyNoSuchTable(err) {
		// A store from before kinds were recorded, or with no snapshot at all.
		out, err = s.immyFleetSnapshotTimes(ctx,
			`SELECT DISTINCT taken_at FROM immy_fleet_snapshots ORDER BY taken_at DESC`)
		if immyNoSuchTable(err) {
			return []string{}, nil // no snapshot was ever written
		}
	}
	return out, err
}

func (s *Store) immyFleetSnapshotTimes(ctx context.Context, query string) ([]string, error) {
	rows, err := s.DB().QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0)
	for rows.Next() {
		var t sql.NullString
		if err := rows.Scan(&t); err != nil {
			_ = rows.Close()
			return nil, err
		}
		if t.Valid {
			out = append(out, t.String)
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	return out, nil
}

// ReadImmyFleetSnapshot loads one snapshot generation keyed by
// resource_type + id.
func (s *Store) ReadImmyFleetSnapshot(ctx context.Context, takenAt string) (map[string]ImmyFleetRow, error) {
	rows, err := s.DB().QueryContext(ctx,
		`SELECT resource_type, id, fingerprint, COALESCE(label,'') FROM immy_fleet_snapshots WHERE taken_at = ?`, takenAt)
	if err != nil {
		if immyNoSuchTable(err) {
			return map[string]ImmyFleetRow{}, nil
		}
		return nil, err
	}
	out := map[string]ImmyFleetRow{}
	for rows.Next() {
		var rt, id, fp, label sql.NullString
		if err := rows.Scan(&rt, &id, &fp, &label); err != nil {
			_ = rows.Close()
			return nil, err
		}
		r := ImmyFleetRow{
			ResourceType: rt.String, ID: id.String,
			Fingerprint: fp.String, Label: label.String,
		}
		out[r.ResourceType+"\x00"+r.ID] = r
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	return out, nil
}

// ReadImmyFleetSnapshotKinds returns the kinds one snapshot generation
// recorded, mapped to their shapes. versioned is false for a generation
// written before kinds were recorded, including every generation in a store
// that predates the kinds table; the caller must not assume such a snapshot
// tracked anything the way this version does.
func (s *Store) ReadImmyFleetSnapshotKinds(ctx context.Context, takenAt string) (kinds map[string]string, versioned bool, err error) {
	rows, err := s.DB().QueryContext(ctx,
		`SELECT kind, shape FROM immy_fleet_snapshot_kinds WHERE taken_at = ?`, takenAt)
	if err != nil {
		if immyNoSuchTable(err) {
			return map[string]string{}, false, nil
		}
		return nil, false, err
	}
	kinds = map[string]string{}
	for rows.Next() {
		var kind, shape sql.NullString
		if err := rows.Scan(&kind, &shape); err != nil {
			_ = rows.Close()
			return nil, false, err
		}
		kinds[kind.String] = shape.String
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, false, err
	}
	if err := rows.Close(); err != nil {
		return nil, false, err
	}
	return kinds, len(kinds) > 0, nil
}

// immyNoSuchTable reports SQLite's missing-table error. Readers open the mirror
// read-only and cannot create the snapshot table, so a store that never took a
// snapshot has none.
func immyNoSuchTable(err error) bool {
	return err != nil && strings.Contains(err.Error(), "no such table")
}
