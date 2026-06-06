// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel-feature store extensions: property-history snapshots,
// the local mutation audit log, and the pending-digest gate used by bulk-update.
// Lives beside the generated store.go so regen-merge preserves it whole.

package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// PropertyHistoryEntry mirrors one row of HubSpot's propertiesWithHistory blob.
type PropertyHistoryEntry struct {
	Property  string
	Value     string
	Timestamp time.Time
	Source    string
	SourceID  string
}

// UpsertPropertyHistoryBatch inserts a slice of property-history entries in one
// transaction. Idempotent on re-sync because of the composite PK.
func (s *Store) UpsertPropertyHistoryBatch(ctx context.Context, objectType, objectID string, entries []PropertyHistoryEntry) error {
	if len(entries) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	stmt, err := tx.PrepareContext(ctx, `INSERT OR REPLACE INTO hubspot_property_history (object_type, object_id, property, value, timestamp, source, source_id, synced_at) VALUES (?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	defer stmt.Close()
	for _, e := range entries {
		if _, err := stmt.ExecContext(ctx, objectType, objectID, e.Property, e.Value, e.Timestamp.UTC(), e.Source, e.SourceID); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

// LogMutation appends one mutation event to the local audit log.
// Best-effort: failures are silently swallowed so audit-logging never blocks
// the user. The audit log is a QoL artifact — "what did I just do?" — not a
// safety contract; a failed write must never take down the foreground command.
func (s *Store) LogMutation(ctx context.Context, command string, args []string, exitCode, affectedCount int, source string, duration time.Duration) {
	argsJSON, _ := json.Marshal(args)
	_, _ = s.db.ExecContext(ctx,
		`INSERT INTO hubspot_local_mutations (command, args_json, exit_code, affected_count, source, duration_ms) VALUES (?, ?, ?, ?, ?, ?)`,
		command, string(argsJSON), exitCode, affectedCount, source, duration.Milliseconds())
}

// PendingDigest is the row shape returned by GetPendingDigest. Mirrors
// cliutil.PendingDigest field-for-field so the gate can consume it through
// the PendingDigestStore interface without store importing cliutil (or vice
// versa). The duplication is intentional — a shared package between the
// two would force every cliutil helper to drag the store dependency along.
type PendingDigest struct {
	Digest    string
	Command   string
	PlanJSON  []byte
	RowCount  int
	CreatedAt time.Time
	ExpiresAt time.Time
}

// PutPendingDigest stores a planned-mutation digest under its digest key
// for later confirm-and-execute. Uses INSERT OR REPLACE so a re-run of
// the same dry-run (which produces the same digest) refreshes the TTL
// instead of erroring. Serialized on writeMu like every other write path.
func (s *Store) PutPendingDigest(ctx context.Context, digest, command string, plan []byte, rowCount int, expires time.Duration) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx,
		`INSERT OR REPLACE INTO hubspot_pending_digests
		(digest, command, plan_json, row_count, created_at, expires_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		digest, command, string(plan), rowCount,
		now.Format(time.RFC3339Nano), now.Add(expires).Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf("inserting pending digest: %w", err)
	}
	return nil
}

// GetPendingDigest returns the row if present and not expired. Returns
// (nil, nil) when missing or expired — caller treats both as "rerun the
// dry-run." Read-only path; no writeMu needed.
func (s *Store) GetPendingDigest(ctx context.Context, digest string) (*PendingDigest, error) {
	var row PendingDigest
	var createdRaw, expiresRaw string
	var plan string
	err := s.db.QueryRowContext(ctx,
		`SELECT digest, command, plan_json, row_count, created_at, expires_at
		FROM hubspot_pending_digests WHERE digest = ?`,
		digest,
	).Scan(&row.Digest, &row.Command, &plan, &row.RowCount, &createdRaw, &expiresRaw)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("selecting pending digest: %w", err)
	}
	row.PlanJSON = []byte(plan)
	row.CreatedAt = parsePendingDigestTime(createdRaw)
	row.ExpiresAt = parsePendingDigestTime(expiresRaw)
	if time.Now().UTC().After(row.ExpiresAt) {
		return nil, nil
	}
	return &row, nil
}

// PurgeExpiredDigests deletes expired rows. Called lazily by the gate
// before any Get; safe to call from anywhere without coordination.
func (s *Store) PurgeExpiredDigests(ctx context.Context) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM hubspot_pending_digests WHERE expires_at < ?`,
		time.Now().UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf("purging expired digests: %w", err)
	}
	return nil
}

// parsePendingDigestTime is forgiving — SQLite's DATETIME column may round-
// trip the value we inserted in RFC3339Nano form, but older inserts (or a
// future migration that uses CURRENT_TIMESTAMP defaults) could land in
// "2006-01-02 15:04:05" shape. Try both before giving up.
func parsePendingDigestTime(raw string) time.Time {
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05"} {
		if t, err := time.Parse(layout, raw); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}
