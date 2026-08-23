// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored store extension for the fleet-diff novel command. Kept in its
// own file with a lazy initialiser so `generate --force` preserves it and the
// generated migration slice in store.go stays untouched.

package store

import (
	"context"
	"database/sql"
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

// EnsureImmyFleetSnapshots creates the fleet-diff snapshot table on first use.
// Idempotent, so novel commands can call it unconditionally before reading or
// writing snapshots.
func (s *Store) EnsureImmyFleetSnapshots(ctx context.Context) error {
	for _, stmt := range []string{immyFleetSnapshotDDL, immyFleetSnapshotIndexDDL} {
		if _, err := s.DB().ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}

// WriteImmyFleetSnapshot records one snapshot generation in a single
// transaction. It deliberately does not call Upsert/UpsertBatch: those open
// their own write transaction, and SQLite WAL permits only one writer, so
// nesting them inside this transaction could busy-wait or fail mid-write.
func (s *Store) WriteImmyFleetSnapshot(ctx context.Context, takenAt string, rows []ImmyFleetRow) error {
	if err := s.EnsureImmyFleetSnapshots(ctx); err != nil {
		return err
	}
	tx, err := s.DB().BeginTx(ctx, nil)
	if err != nil {
		return err
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

// ImmyFleetSnapshotTimes returns snapshot generations, newest first.
func (s *Store) ImmyFleetSnapshotTimes(ctx context.Context) ([]string, error) {
	if err := s.EnsureImmyFleetSnapshots(ctx); err != nil {
		return nil, err
	}
	rows, err := s.DB().QueryContext(ctx,
		`SELECT DISTINCT taken_at FROM immy_fleet_snapshots ORDER BY taken_at DESC`)
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
	if err := s.EnsureImmyFleetSnapshots(ctx); err != nil {
		return nil, err
	}
	rows, err := s.DB().QueryContext(ctx,
		`SELECT resource_type, id, fingerprint, COALESCE(label,'') FROM immy_fleet_snapshots WHERE taken_at = ?`, takenAt)
	if err != nil {
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
