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
	}
	for _, m := range migrations {
		if _, err := conn.ExecContext(ctx, m); err != nil {
			return fmt.Errorf("extra migration failed: %w", err)
		}
	}
	return nil
}
