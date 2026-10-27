// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

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
		// usage_snapshots (legacy): keyed by offering name only, so distinct
		// editions/infrastructures of one offering overwrote each other. Kept
		// so older databases still open; nothing reads it any more.
		`CREATE TABLE IF NOT EXISTS "usage_snapshots" (
			"snapshot_date" TEXT NOT NULL,
			"tenant_id" TEXT NOT NULL,
			"offering" TEXT NOT NULL,
			"value" REAL,
			"edition" TEXT,
			"captured_at" DATETIME DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY ("snapshot_date", "tenant_id", "offering")
		)`,
		`CREATE INDEX IF NOT EXISTS "idx_usage_snapshots_date" ON "usage_snapshots"("snapshot_date")`,
		// usage_snapshot_rows: point-in-time copies of the usages table so
		// `usages drift` can diff two dates (the live usages table is
		// overwritten on every sync). Keyed by the full usage identity the
		// store keys usages by; value NULL means the API reported no value.
		`CREATE TABLE IF NOT EXISTS "usage_snapshot_rows" (
			"snapshot_date" TEXT NOT NULL,
			"tenant_id" TEXT NOT NULL,
			"application_id" TEXT NOT NULL DEFAULT '',
			"offering" TEXT NOT NULL DEFAULT '',
			"edition" TEXT NOT NULL DEFAULT '',
			"infra_id" TEXT NOT NULL DEFAULT '',
			"value" REAL,
			PRIMARY KEY ("snapshot_date", "tenant_id", "application_id", "offering", "edition", "infra_id")
		)`,
		`CREATE INDEX IF NOT EXISTS "idx_usage_snapshot_rows_date" ON "usage_snapshot_rows"("snapshot_date")`,
		// usage_snapshot_captures: one row per completed capture, so an
		// intentionally empty capture is distinguishable from a missing one.
		`CREATE TABLE IF NOT EXISTS "usage_snapshot_captures" (
			"snapshot_date" TEXT PRIMARY KEY,
			"rows" INTEGER NOT NULL,
			"captured_at" DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
	}
	for _, m := range migrations {
		if _, err := conn.ExecContext(ctx, m); err != nil {
			return fmt.Errorf("extra migration failed: %w", err)
		}
	}
	return nil
}
