// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
)

// PartitionMove names rows an older print stored under From that this build
// writes under To: only rows whose JSON:API `type` is in Types move, so a
// partition name the press reused for another family keeps its own rows.
type PartitionMove struct {
	From, To string
	Types    []string
}

// LegacyPartitionMoves is set by the cli package (auvik_local.go), which owns
// the family table. Every writable open moves the matching rows into the
// current partition, so the current syncer's reconcile governs them: an
// unfiltered full sync prunes what Auvik stopped returning, a filtered one
// prunes nothing. Idempotent: a moved row is no longer in From.
var LegacyPartitionMoves []PartitionMove

// migrateExtras runs after the generated store migrations and before the
// schema-version stamp. It is the canonical place for novel-feature auxiliary
// tables that need to live in the local store.
//
// Edit this file when adding tables for novel commands. Keep migrations
// idempotent with CREATE TABLE IF NOT EXISTS / CREATE INDEX IF NOT EXISTS so
// every store open can safely re-run them.
func (s *Store) migrateExtras(ctx context.Context, conn *sql.Conn) error {
	migrations := []string{
		// auvik_device_snapshots backs `auvik-cli inventory diff`.
		//
		// Auvik's API emits no deletion event: filter[modifiedAfter] surfaces
		// additions and changes only, so a decommissioned device simply stops
		// appearing in list responses. Detecting removals therefore requires
		// retaining our own prior view of the fleet. Each row is one device as
		// observed at one snapshot instant; the fingerprint lets the diff
		// classify "changed" without storing whole payloads.
		`CREATE TABLE IF NOT EXISTS auvik_device_snapshots (
			snapshot_at DATETIME NOT NULL,
			device_id   TEXT NOT NULL,
			tenant_id   TEXT,
			device_name TEXT,
			device_type TEXT,
			make_model  TEXT,
			serial_number    TEXT,
			firmware_version TEXT,
			software_version TEXT,
			fingerprint TEXT NOT NULL,
			PRIMARY KEY (snapshot_at, device_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_auvik_snapshots_device ON auvik_device_snapshots(device_id)`,
		// auvik_snapshot_runs records that a snapshot was taken, independently
		// of its device rows. An empty fleet (Auvik's documented data: [])
		// writes no device rows, so without this table the empty snapshot
		// would never become the newest baseline.
		`CREATE TABLE IF NOT EXISTS auvik_snapshot_runs (
			snapshot_at DATETIME NOT NULL PRIMARY KEY,
			devices     INTEGER NOT NULL
		)`,
	}
	for _, m := range migrations {
		if _, err := conn.ExecContext(ctx, m); err != nil {
			return fmt.Errorf("extra migration failed: %w", err)
		}
	}
	return moveLegacyPartitions(ctx, conn)
}

// moveLegacyPartitions applies LegacyPartitionMoves. A row whose id the current
// partition already holds is dropped, never copied over the current copy.
// The search index follows the move.
func moveLegacyPartitions(ctx context.Context, conn *sql.Conn) error {
	if len(LegacyPartitionMoves) == 0 {
		return nil
	}
	ftsExists, err := tableExists(ctx, conn, "resources_fts")
	if err != nil {
		return err
	}
	for _, m := range LegacyPartitionMoves {
		if m.From == "" || m.To == "" || m.From == m.To || len(m.Types) == 0 {
			continue
		}
		args := []any{m.From}
		for _, t := range m.Types {
			args = append(args, t)
		}
		// Drain first: one connection, no statement may run while rows is open.
		rows, err := conn.QueryContext(ctx, fmt.Sprintf(
			`SELECT id, data FROM resources WHERE resource_type = ? AND json_valid(data) AND json_extract(data, '$.type') IN (%s)`,
			strings.TrimSuffix(strings.Repeat("?,", len(m.Types)), ",")), args...)
		if err != nil {
			return fmt.Errorf("listing legacy %s rows: %w", m.From, err)
		}
		type row struct{ id, data string }
		var moving []row
		for rows.Next() {
			var r row
			if err := rows.Scan(&r.id, &r.data); err != nil {
				_ = rows.Close()
				return fmt.Errorf("scanning legacy %s row: %w", m.From, err)
			}
			moving = append(moving, r)
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return fmt.Errorf("reading legacy %s rows: %w", m.From, err)
		}
		if err := rows.Close(); err != nil {
			return fmt.Errorf("closing legacy %s rows: %w", m.From, err)
		}
		for _, r := range moving {
			res, err := conn.ExecContext(ctx, `INSERT OR IGNORE INTO resources (id, resource_type, data, synced_at, updated_at)
				SELECT id, ?, data, synced_at, updated_at FROM resources WHERE resource_type = ? AND id = ?`, m.To, m.From, r.id)
			if err != nil {
				return fmt.Errorf("moving %s/%s to %s: %w", m.From, r.id, m.To, err)
			}
			inserted, err := res.RowsAffected()
			if err != nil {
				return fmt.Errorf("moving %s/%s to %s: %w", m.From, r.id, m.To, err)
			}
			if _, err := conn.ExecContext(ctx, `DELETE FROM resources WHERE resource_type = ? AND id = ?`, m.From, r.id); err != nil {
				return fmt.Errorf("removing moved %s/%s: %w", m.From, r.id, err)
			}
			if !ftsExists {
				continue
			}
			if _, err := conn.ExecContext(ctx, `DELETE FROM resources_fts WHERE rowid = ?`, ftsRowID(m.From, r.id)); err != nil {
				return fmt.Errorf("unindexing moved %s/%s: %w", m.From, r.id, err)
			}
			if inserted == 1 {
				if _, err := conn.ExecContext(ctx, `DELETE FROM resources_fts WHERE rowid = ?`, ftsRowID(m.To, r.id)); err != nil {
					return fmt.Errorf("reindexing %s/%s: %w", m.To, r.id, err)
				}
				if _, err := conn.ExecContext(ctx,
					`INSERT INTO resources_fts (rowid, id, resource_type, content) VALUES (?, ?, ?, ?)`,
					ftsRowID(m.To, r.id), r.id, m.To, searchableResourceContent(json.RawMessage(r.data))); err != nil {
					return fmt.Errorf("indexing %s/%s: %w", m.To, r.id, err)
				}
			}
		}
	}
	return nil
}
