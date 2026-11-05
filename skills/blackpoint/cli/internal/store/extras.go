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
		// Add CREATE TABLE IF NOT EXISTS statements here.
	}
	for _, m := range migrations {
		if _, err := conn.ExecContext(ctx, m); err != nil {
			return fmt.Errorf("extra migration failed: %w", err)
		}
	}
	return nil
}

// FleetSyncGaps counts fleet-sync slices whose last fetch failed or was
// truncated. fleet-sync indexes its rows into the generated resource tables,
// but records completeness only in fleet_sync_state, so every reader that
// reports the store as complete must also consult this count. A store that
// never ran fleet-sync has no such table and no gaps.
func (s *Store) FleetSyncGaps() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM fleet_sync_state WHERE error <> ''`).Scan(&n)
	if err != nil && strings.Contains(strings.ToLower(err.Error()), "no such table") {
		return 0, nil
	}
	return n, err
}
