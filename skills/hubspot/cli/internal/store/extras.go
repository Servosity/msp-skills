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
		// hubspot_pending_digests backs the dry-run -> digest -> confirm
		// gating used by `contacts bulk-update` (and any future >100-row
		// mutation command). A row is written by the dry-run path and
		// looked up by the confirm path; rows expire after TTL and are
		// lazily GC'd by PurgeExpiredDigests. Keyed on the operator-facing
		// digest itself ("blast-<hex>") so two concurrent dry-runs with
		// identical plans collapse to a single row instead of duplicating.
		`CREATE TABLE IF NOT EXISTS "hubspot_pending_digests" (
			"digest" TEXT PRIMARY KEY,
			"command" TEXT NOT NULL,
			"plan_json" TEXT NOT NULL,
			"row_count" INTEGER NOT NULL,
			"created_at" DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			"expires_at" DATETIME NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS "idx_hubspot_pending_digests_expires_at" ON "hubspot_pending_digests"("expires_at")`,
		// hubspot_property_history is the shared per-property snapshot table
		// behind sync --with-history and the meetings history / ever-had /
		// status-report and deals velocity commands. Composite PK makes
		// re-sync idempotent.
		`CREATE TABLE IF NOT EXISTS "hubspot_property_history" (
			"object_type" TEXT NOT NULL,
			"object_id" TEXT NOT NULL,
			"property" TEXT NOT NULL,
			"value" TEXT,
			"timestamp" DATETIME NOT NULL,
			"source" TEXT,
			"source_id" TEXT,
			"synced_at" DATETIME DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY ("object_type", "object_id", "property", "timestamp")
		)`,
		`CREATE INDEX IF NOT EXISTS "idx_hubspot_property_history_obj_prop_ts" ON "hubspot_property_history"("object_type", "object_id", "property", "timestamp" DESC)`,
		`CREATE INDEX IF NOT EXISTS "idx_hubspot_property_history_value_lookup" ON "hubspot_property_history"("object_type", "property", "value", "timestamp")`,
		// hubspot_local_mutations is the best-effort local audit log written
		// by mutation commands via LogMutation ("what did I just do?").
		`CREATE TABLE IF NOT EXISTS "hubspot_local_mutations" (
			"id" INTEGER PRIMARY KEY AUTOINCREMENT,
			"timestamp" DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			"command" TEXT NOT NULL,
			"args_json" TEXT NOT NULL,
			"exit_code" INTEGER NOT NULL,
			"affected_count" INTEGER NOT NULL,
			"source" TEXT NOT NULL DEFAULT 'cli',
			"duration_ms" INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE INDEX IF NOT EXISTS "idx_hubspot_local_mutations_ts" ON "hubspot_local_mutations"("timestamp" DESC)`,
		`CREATE INDEX IF NOT EXISTS "idx_hubspot_local_mutations_cmd_ts" ON "hubspot_local_mutations"("command", "timestamp" DESC)`,
	}
	for _, m := range migrations {
		if _, err := conn.ExecContext(ctx, m); err != nil {
			return fmt.Errorf("extra migration failed: %w", err)
		}
	}
	return nil
}
