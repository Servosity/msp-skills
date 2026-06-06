// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// EnsureMspbotsSchema lazily creates the registry and snapshot tables used by
// the hand-written novel commands (registry, snapshot, diff, trend). Safe to
// call before every access; every statement is IF NOT EXISTS.
func (s *Store) EnsureMspbotsSchema(ctx context.Context) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS mspbots_registry (
			alias TEXT PRIMARY KEY,
			resource_id TEXT NOT NULL,
			resource_type TEXT NOT NULL CHECK (resource_type IN ('dataset','widget')),
			notes TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS mspbots_snapshots (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			alias TEXT NOT NULL,
			resource_id TEXT NOT NULL,
			resource_type TEXT NOT NULL,
			taken_at TEXT NOT NULL,
			row_count INTEGER NOT NULL,
			pages INTEGER NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_mspbots_snapshots_alias_taken
			ON mspbots_snapshots(alias, taken_at)`,
		`CREATE TABLE IF NOT EXISTS mspbots_snapshot_rows (
			snapshot_id INTEGER NOT NULL,
			idx INTEGER NOT NULL,
			row_key TEXT NOT NULL DEFAULT '',
			data TEXT NOT NULL,
			PRIMARY KEY (snapshot_id, idx)
		)`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("ensuring mspbots schema: %w", err)
		}
	}
	return nil
}

// RegistryEntry is one alias→resource mapping curated by the user. The Public
// API has no enumeration endpoint, so this local registry is the CLI's only
// discovery surface.
type RegistryEntry struct {
	Alias        string `json:"alias"`
	ResourceID   string `json:"resource_id"`
	ResourceType string `json:"resource_type"`
	Notes        string `json:"notes,omitempty"`
	CreatedAt    string `json:"created_at"`
}

// RegistryAdd inserts or replaces an alias mapping.
func (s *Store) RegistryAdd(ctx context.Context, e RegistryEntry) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if e.CreatedAt == "" {
		e.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO mspbots_registry (alias, resource_id, resource_type, notes, created_at)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(alias) DO UPDATE SET
		   resource_id = excluded.resource_id,
		   resource_type = excluded.resource_type,
		   notes = excluded.notes`,
		e.Alias, e.ResourceID, e.ResourceType, e.Notes, e.CreatedAt)
	return err
}

// RegistryGet returns the entry for alias, or sql.ErrNoRows when absent.
func (s *Store) RegistryGet(ctx context.Context, alias string) (RegistryEntry, error) {
	var e RegistryEntry
	var notes sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT alias, resource_id, resource_type, COALESCE(notes,''), created_at
		 FROM mspbots_registry WHERE alias = ?`, alias).
		Scan(&e.Alias, &e.ResourceID, &e.ResourceType, &notes, &e.CreatedAt)
	e.Notes = notes.String
	return e, err
}

// RegistryList returns every alias mapping ordered by alias.
func (s *Store) RegistryList(ctx context.Context) ([]RegistryEntry, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT alias, resource_id, resource_type, COALESCE(notes,''), created_at
		 FROM mspbots_registry ORDER BY alias`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]RegistryEntry, 0)
	for rows.Next() {
		var e RegistryEntry
		var notes sql.NullString
		if err := rows.Scan(&e.Alias, &e.ResourceID, &e.ResourceType, &notes, &e.CreatedAt); err != nil {
			return nil, err
		}
		e.Notes = notes.String
		out = append(out, e)
	}
	return out, rows.Err()
}

// RegistryRemove deletes an alias. Returns the number of rows removed.
func (s *Store) RegistryRemove(ctx context.Context, alias string) (int64, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	res, err := s.db.ExecContext(ctx, `DELETE FROM mspbots_registry WHERE alias = ?`, alias)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// SnapshotMeta describes one stored point-in-time capture.
type SnapshotMeta struct {
	ID           int64  `json:"id"`
	Alias        string `json:"alias"`
	ResourceID   string `json:"resource_id"`
	ResourceType string `json:"resource_type"`
	TakenAt      string `json:"taken_at"`
	RowCount     int    `json:"row_count"`
	Pages        int    `json:"pages"`
}

// SnapshotInsert stores a capture and its rows in one transaction.
// rowKeys[i] carries the per-row identity (an "id"-like field when present,
// else a content hash) used later by diff.
func (s *Store) SnapshotInsert(ctx context.Context, meta SnapshotMeta, rowsJSON []json.RawMessage, rowKeys []string) (int64, error) {
	if len(rowsJSON) != len(rowKeys) {
		return 0, fmt.Errorf("rows/keys length mismatch: %d vs %d", len(rowsJSON), len(rowKeys))
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.ExecContext(ctx,
		`INSERT INTO mspbots_snapshots (alias, resource_id, resource_type, taken_at, row_count, pages)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		meta.Alias, meta.ResourceID, meta.ResourceType, meta.TakenAt, meta.RowCount, meta.Pages)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO mspbots_snapshot_rows (snapshot_id, idx, row_key, data) VALUES (?, ?, ?, ?)`)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()
	for i, data := range rowsJSON {
		if _, err := stmt.ExecContext(ctx, id, i, rowKeys[i], string(data)); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return id, nil
}

// SnapshotsForAlias returns snapshot metadata for alias, newest first.
func (s *Store) SnapshotsForAlias(ctx context.Context, alias string, limit int) ([]SnapshotMeta, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, alias, resource_id, resource_type, taken_at, row_count, pages
		 FROM mspbots_snapshots WHERE alias = ? ORDER BY taken_at DESC, id DESC LIMIT ?`,
		alias, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]SnapshotMeta, 0)
	for rows.Next() {
		var m SnapshotMeta
		if err := rows.Scan(&m.ID, &m.Alias, &m.ResourceID, &m.ResourceType, &m.TakenAt, &m.RowCount, &m.Pages); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// SnapshotRows returns the stored rows for one snapshot in insertion order.
func (s *Store) SnapshotRows(ctx context.Context, snapshotID int64) (keys []string, data []json.RawMessage, err error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT row_key, data FROM mspbots_snapshot_rows WHERE snapshot_id = ? ORDER BY idx`, snapshotID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var k string
		var d string
		if err := rows.Scan(&k, &d); err != nil {
			return nil, nil, err
		}
		keys = append(keys, k)
		data = append(data, json.RawMessage(d))
	}
	return keys, data, rows.Err()
}
