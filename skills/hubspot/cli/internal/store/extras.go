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
			"snapshot_at" TEXT,
			PRIMARY KEY ("object_type", "object_id", "property", "timestamp")
		)`,
		`CREATE INDEX IF NOT EXISTS "idx_hubspot_property_history_obj_prop_ts" ON "hubspot_property_history"("object_type", "object_id", "property", "timestamp" DESC)`,
		`CREATE INDEX IF NOT EXISTS "idx_hubspot_property_history_value_lookup" ON "hubspot_property_history"("object_type", "property", "value", "timestamp")`,
		// hubspot_pipeline_object_types records which object type each stored
		// pipeline belongs to, taken from the request path that fetched it
		// (GET /crm/v3/pipelines/{objectType}). Every object type's pipelines
		// share one resource, so the deal readers take only rows declared
		// deals here (handfixes.json: pipeline-object-type-declared).
		`CREATE TABLE IF NOT EXISTS "hubspot_pipeline_object_types" (
			"id" TEXT PRIMARY KEY,
			"object_type" TEXT NOT NULL
		)`,
	}
	for _, m := range migrations {
		if _, err := conn.ExecContext(ctx, m); err != nil {
			return fmt.Errorf("extra migration failed: %w", err)
		}
	}
	// snapshot_at is the time HubSpot reported for the record a history row
	// came with (handfixes.json hubspot-velocity-history-must-describe-current-stage);
	// a row an older build wrote has none, so it proves nothing about freshness.
	return s.ensureColumn(ctx, conn, "hubspot_property_history", "snapshot_at", "TEXT")
}
