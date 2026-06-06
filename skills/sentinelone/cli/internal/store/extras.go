// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package store

import (
	"context"
	"database/sql"
	"fmt"
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
	}
	for _, m := range migrations {
		if _, err := conn.ExecContext(ctx, m); err != nil {
			return fmt.Errorf("extra migration failed: %w", err)
		}
	}
	return nil
}
