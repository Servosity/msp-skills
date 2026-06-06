// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored: fleet snapshot tables for the `fleet snapshot` / `fleet diff`
// novel commands. Snapshots freeze the current `resources` rows under a label
// so any fleet number reported from the local store can be reproduced later
// (QBR receipts, audit evidence). Lazy-init per the durable-extension pattern:
// the tables are created on first use, not in the generated migration slice.
package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// SnapshotMeta describes one labeled fleet snapshot.
type SnapshotMeta struct {
	Label     string `json:"label"`
	CreatedAt string `json:"createdAt"`
	Note      string `json:"note,omitempty"`
	RowCount  int    `json:"rowCount"`
}

// ensureSnapshotTables creates the snapshot tables when missing.
func (s *Store) ensureSnapshotTables(ctx context.Context) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS fleet_snapshot_meta (
			label TEXT PRIMARY KEY,
			created_at DATETIME NOT NULL,
			note TEXT,
			row_count INTEGER NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS fleet_snapshots (
			label TEXT NOT NULL,
			resource_type TEXT NOT NULL,
			id TEXT NOT NULL,
			data JSON NOT NULL,
			PRIMARY KEY (label, resource_type, id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_fleet_snapshots_label ON fleet_snapshots(label, resource_type)`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("creating snapshot tables: %w", err)
		}
	}
	return nil
}

// CreateFleetSnapshot copies every current resources row into the snapshot
// tables under label. Fails when the label already exists so receipts are
// immutable once taken.
func (s *Store) CreateFleetSnapshot(ctx context.Context, label, note string) (SnapshotMeta, error) {
	if err := s.ensureSnapshotTables(ctx); err != nil {
		return SnapshotMeta{}, err
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	var exists int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM fleet_snapshot_meta WHERE label = ?`, label).Scan(&exists); err != nil {
		return SnapshotMeta{}, err
	}
	if exists > 0 {
		return SnapshotMeta{}, fmt.Errorf("snapshot label %q already exists (snapshots are immutable receipts; pick a new label)", label)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return SnapshotMeta{}, err
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx,
		`INSERT INTO fleet_snapshots (label, resource_type, id, data)
		 SELECT ?, resource_type, id, data FROM resources`, label)
	if err != nil {
		return SnapshotMeta{}, fmt.Errorf("copying resources into snapshot: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		rows = 0
	}
	createdAt := time.Now().UTC().Format(time.RFC3339)
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO fleet_snapshot_meta (label, created_at, note, row_count) VALUES (?, ?, ?, ?)`,
		label, createdAt, note, rows); err != nil {
		return SnapshotMeta{}, fmt.Errorf("recording snapshot manifest: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return SnapshotMeta{}, err
	}
	return SnapshotMeta{Label: label, CreatedAt: createdAt, Note: note, RowCount: int(rows)}, nil
}

// ListFleetSnapshots returns every snapshot manifest row, newest first.
func (s *Store) ListFleetSnapshots(ctx context.Context) ([]SnapshotMeta, error) {
	if err := s.ensureSnapshotTables(ctx); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT label, created_at, note, row_count FROM fleet_snapshot_meta ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SnapshotMeta{}
	for rows.Next() {
		var m SnapshotMeta
		var note sql.NullString
		if err := rows.Scan(&m.Label, &m.CreatedAt, &note, &m.RowCount); err != nil {
			return nil, err
		}
		m.Note = note.String
		out = append(out, m)
	}
	return out, rows.Err()
}

// SnapshotResourceData returns the raw data blobs stored under a snapshot
// label for one resource type. Used by `fleet diff` to load a frozen device
// set.
func (s *Store) SnapshotResourceData(ctx context.Context, label, resourceType string) ([]json.RawMessage, error) {
	if err := s.ensureSnapshotTables(ctx); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT data FROM fleet_snapshots WHERE label = ? AND resource_type = ?`, label, resourceType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []json.RawMessage{}
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		out = append(out, json.RawMessage(raw))
	}
	return out, rows.Err()
}

// SnapshotExists reports whether a snapshot label is present.
func (s *Store) SnapshotExists(ctx context.Context, label string) (bool, error) {
	if err := s.ensureSnapshotTables(ctx); err != nil {
		return false, err
	}
	var n int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM fleet_snapshot_meta WHERE label = ?`, label).Scan(&n); err != nil {
		return false, err
	}
	return n > 0, nil
}
