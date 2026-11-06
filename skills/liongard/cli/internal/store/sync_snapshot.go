// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package store

// liongard handfix (r2-narrowed-sync-is-not-a-snapshot): sync_state records
// whether a resource's last finished sync was a full, unnarrowed snapshot.
//
// A sync narrowed by a query parameter (Liongard's documented fields[]
// projection and conditions[] filter, --param, --global-param, a since
// window) or resumed from another walk's cursor finishes and sets
// last_attempt_complete like any other, yet holds only the rows and fields
// it asked for. Codex r2 class incomplete-snapshot-read-as-complete:
// `sync --resource-param 'agents:fields[]=ID'` stored [{"ID":1511}] and
// `agents offline` then reported zero offline agents.
//
// snapshot_full is 1 only when the hand writer below says the walk was
// full. A trigger resets it on every other write to the checkpoint columns,
// so any generated writer (SaveSyncState, SaveSyncStateAt,
// SaveSyncStateAtInScope, progress saves, scope rebinding, --full clears)
// leaves a checkpoint that does not claim a full snapshot. Fail closed by
// construction: the flag cannot outlive the walk that set it.

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

const syncSnapshotColumn = "snapshot_full"

// syncSnapshotResetTrigger clears snapshot_full whenever a writer touches
// the checkpoint columns. Only SaveSyncSnapshotAtInScope sets it again, in
// the same transaction, after its own checkpoint update fired this trigger.
const syncSnapshotResetTrigger = `CREATE TRIGGER IF NOT EXISTS "sync_state_snapshot_full_reset"
	AFTER UPDATE OF last_cursor, last_synced_at, total_count, last_attempt_complete, scope_key ON sync_state
	FOR EACH ROW WHEN NEW.snapshot_full <> 0
	BEGIN
		UPDATE sync_state SET snapshot_full = 0 WHERE resource_type = NEW.resource_type;
	END`

// migrateSyncSnapshot adds snapshot_full (default 0: a checkpoint written
// before this column existed does not prove a full snapshot) and the reset
// trigger. Idempotent; runs inside the migration lock from migrateExtras.
func (s *Store) migrateSyncSnapshot(ctx context.Context, conn *sql.Conn) error {
	var present int
	if err := conn.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM pragma_table_xinfo('sync_state') WHERE name = ?`, syncSnapshotColumn,
	).Scan(&present); err != nil {
		return fmt.Errorf("probing sync_state.%s: %w", syncSnapshotColumn, err)
	}
	if present == 0 {
		if err := addColumn(ctx, conn, "sync_state", syncSnapshotColumn, "INTEGER NOT NULL DEFAULT 0"); err != nil {
			return err
		}
	}
	if _, err := conn.ExecContext(ctx, syncSnapshotResetTrigger); err != nil {
		return fmt.Errorf("creating the sync snapshot trigger: %w", err)
	}
	return nil
}

// SaveSyncSnapshotAtInScope is SaveSyncStateAtInScope plus the snapshot
// verdict: full is true only for a walk that fetched the resource's whole
// list with nothing narrowing it. A row another walk has since rebound is
// left unchanged.
func (s *Store) SaveSyncSnapshotAtInScope(resourceType, scopeKey, cursor string, count int, at time.Time, full bool) error {
	s.lockForWrite()
	defer s.unlockAfterWrite()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.Exec(
		`UPDATE sync_state SET last_cursor = ?, last_synced_at = ?, total_count = ?, last_attempt_complete = 1
		 WHERE resource_type = ? AND scope_key = ?`,
		cursor, at.UTC().Format(time.RFC3339), count, resourceType, scopeKey,
	)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("sync scope for %s changed before the checkpoint was saved", resourceType)
	}
	verdict := 0
	if full {
		verdict = 1
	}
	if _, err := tx.Exec(
		`UPDATE sync_state SET snapshot_full = ? WHERE resource_type = ? AND scope_key = ?`,
		verdict, resourceType, scopeKey,
	); err != nil {
		return err
	}
	return tx.Commit()
}

// SyncSnapshot is one resource's sync_state as the store-backed commands
// read it.
type SyncSnapshot struct {
	// Recorded is false when no sync of the resource ever started.
	Recorded bool
	// Complete is true when the last attempt finished with a sync time.
	Complete bool
	// Full is true when that finished sync was a full, unnarrowed snapshot.
	Full bool
}

// SyncSnapshotState reads a resource's snapshot verdict.
func (s *Store) SyncSnapshotState(resourceType string) (SyncSnapshot, error) {
	var complete, full sql.NullInt64
	var syncedAt sql.NullString
	err := s.db.QueryRow(
		`SELECT last_attempt_complete, last_synced_at, snapshot_full FROM sync_state WHERE resource_type = ?`,
		resourceType,
	).Scan(&complete, &syncedAt, &full)
	if errors.Is(err, sql.ErrNoRows) {
		return SyncSnapshot{}, nil
	}
	if err != nil {
		return SyncSnapshot{}, err
	}
	snap := SyncSnapshot{Recorded: true}
	snap.Complete = complete.Valid && complete.Int64 == 1 && syncedAt.Valid && strings.TrimSpace(syncedAt.String) != ""
	snap.Full = snap.Complete && full.Valid && full.Int64 == 1
	return snap, nil
}

// SyncSnapshotFullInScope reports whether scopeKey's current checkpoint for
// the resource is a finished full snapshot. Any read error is "no".
func (s *Store) SyncSnapshotFullInScope(resourceType, scopeKey string) bool {
	var complete, full sql.NullInt64
	err := s.db.QueryRow(
		`SELECT last_attempt_complete, snapshot_full FROM sync_state WHERE resource_type = ? AND scope_key = ?`,
		resourceType, scopeKey,
	).Scan(&complete, &full)
	return err == nil && complete.Valid && complete.Int64 == 1 && full.Valid && full.Int64 == 1
}
