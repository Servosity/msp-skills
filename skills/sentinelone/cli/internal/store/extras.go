// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// migrateExtras runs after the generated store migrations and before the
// schema-version stamp. It is the canonical place for novel-feature auxiliary
// tables that need to live in the local store.
//
// Edit this file when adding tables for novel commands. Keep migrations
// idempotent with CREATE TABLE IF NOT EXISTS / CREATE INDEX IF NOT EXISTS so
// every store open can safely re-run them.
func (s *Store) migrateExtras(ctx context.Context, conn *sql.Conn) error {
	migrations := []string{
		// fleet_snapshots stores a per-sync historical copy of high-gravity
		// entity state (agents, threats) so the time-travel novel features
		// (whatchanged, versions rollout, threats mttr, threats verdicts
		// --changed) can diff "now" against an earlier point. The live
		// `resources` table only keeps the latest upserted state per id, so
		// without this append-only table there is no history to diff.
		`CREATE TABLE IF NOT EXISTS fleet_snapshots (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			captured_at TEXT NOT NULL,
			entity      TEXT NOT NULL,
			entity_id   TEXT NOT NULL,
			data        TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_fleet_snapshots_entity_time ON fleet_snapshots(entity, captured_at)`,
		`CREATE INDEX IF NOT EXISTS idx_fleet_snapshots_entity_id ON fleet_snapshots(entity, entity_id)`,
		// fleet_capture_log records every capture, including one that held zero
		// rows (the last agent removed is a fleet state too), so history readers
		// never fall back to an older non-empty capture as "the latest".
		// walk_id names the sync walk whose own writes the capture holds; it is
		// NULL for captures recorded before walks were tracked (and for the
		// explicit-time CaptureFleetSnapshotFor API). No reader joins on it.
		`CREATE TABLE IF NOT EXISTS fleet_capture_log (
			entity      TEXT NOT NULL,
			captured_at TEXT NOT NULL,
			row_count   INTEGER NOT NULL DEFAULT 0,
			walk_id     INTEGER,
			PRIMARY KEY (entity, captured_at)
		)`,
		// fleet_walks registers one sync walk per invocation. walk_id is the
		// membership key: AUTOINCREMENT never reuses an id, even after a walk
		// row is deleted. schema_version is the store's PRAGMA schema_version
		// when the walk began; the per-connection recorder that fills
		// fleet_walk_rows hangs off the resources table, so a schema change
		// mid-walk (a table rebuild drops it) voids the walk's proof.
		`CREATE TABLE IF NOT EXISTS fleet_walks (
			walk_id        INTEGER PRIMARY KEY AUTOINCREMENT,
			opened_at      TEXT NOT NULL,
			schema_version INTEGER NOT NULL
		)`,
		// fleet_walk_rows is the exact set of agents/threats rows one walk
		// upserted, with the data it wrote. It is filled only by the TEMP
		// triggers the walk's own process installs on its own connections
		// (snapshots.go), never by a timestamp comparison.
		`CREATE TABLE IF NOT EXISTS fleet_walk_rows (
			walk_id   INTEGER NOT NULL,
			entity    TEXT NOT NULL,
			entity_id TEXT NOT NULL,
			data      TEXT NOT NULL,
			PRIMARY KEY (walk_id, entity, entity_id)
		)`,
		// fleet_walk_state is the sync_state history one walk wrote for an
		// entity: started = it marked the attempt incomplete (the walk began
		// requesting), complete = the last value it wrote. Only started AND
		// complete proves this walk finished a whole-fleet read.
		`CREATE TABLE IF NOT EXISTS fleet_walk_state (
			walk_id  INTEGER NOT NULL,
			entity   TEXT NOT NULL,
			started  INTEGER NOT NULL DEFAULT 0,
			complete INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (walk_id, entity)
		)`,
	}
	for _, m := range migrations {
		if _, err := conn.ExecContext(ctx, m); err != nil {
			return fmt.Errorf("extra migration failed: %w", err)
		}
	}
	// A store from a release before walks were tracked has fleet_capture_log
	// without walk_id. Add it (NULL for every existing capture) so the store
	// still opens and old captures keep reading exactly as before.
	var hasWalkID int
	if err := conn.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM pragma_table_xinfo('fleet_capture_log') WHERE name = 'walk_id'`,
	).Scan(&hasWalkID); err != nil {
		return fmt.Errorf("extra migration failed: inspecting fleet_capture_log: %w", err)
	}
	if hasWalkID == 0 {
		if _, err := conn.ExecContext(ctx, `ALTER TABLE fleet_capture_log ADD COLUMN walk_id INTEGER`); err != nil &&
			!strings.Contains(err.Error(), "duplicate column name") { // a concurrent open added it first
			return fmt.Errorf("extra migration failed: adding fleet_capture_log.walk_id: %w", err)
		}
	}
	// A store an older build wrote may hold credentials (agent passphrases,
	// registration tokens, uninstall passwords) in any table. Scrub every
	// table once per old store, gated on the pre-migration user_version
	// (below v14). This must stay before the generated version stamp, in the
	// same transaction, so a failed scrub leaves the store below v14
	// (s1_secret_store.go).
	if err := s.scrubStoredSecrets(ctx, conn); err != nil {
		return fmt.Errorf("extra migration failed: %w", err)
	}
	return nil
}
