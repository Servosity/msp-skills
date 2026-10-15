// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// MirrorWrite is one resource type's part of a QuoteWerks mirror commit.
type MirrorWrite struct {
	Resource string
	// Rows replace their stored copies outright. Unlike Upsert/UpsertBatch
	// there is no keep-richer merge: a value QuoteWerks cleared to null or ""
	// must not survive from the previous snapshot. Each row needs an "id".
	Rows []json.RawMessage
	// Delete removes these ids (and their search-index entries).
	Delete []string
	// Stamp records sync_state for Resource with Count rows; Partial marks a
	// walk that stopped before reading everything (last_attempt_complete=0).
	Stamp   bool
	Count   int
	Partial bool
}

// ApplyMirror commits a whole mirror run in one transaction, so a failure
// part-way leaves the previous snapshot (and its change baseline) intact.
func (s *Store) ApplyMirror(ctx context.Context, writes []MirrorWrite) error {
	s.lockForWrite()
	defer s.unlockAfterWrite()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now().UTC().Format(time.RFC3339)
	for _, w := range writes {
		for _, raw := range w.Rows {
			var obj map[string]any
			if err := json.Unmarshal(raw, &obj); err != nil {
				return fmt.Errorf("mirror %s: decoding row: %w", w.Resource, err)
			}
			id := extractObjectID(obj)
			if id == "" {
				return fmt.Errorf("mirror %s: row has no id", w.Resource)
			}
			if err := s.upsertGenericResourceTx(tx, w.Resource, id, raw); err != nil {
				return fmt.Errorf("mirror %s: storing %s: %w", w.Resource, id, err)
			}
		}
		for _, id := range w.Delete {
			if _, err := tx.ExecContext(ctx, `DELETE FROM resources WHERE resource_type = ? AND id = ?`, w.Resource, id); err != nil {
				return fmt.Errorf("mirror %s: deleting %s: %w", w.Resource, id, err)
			}
			if _, err := tx.ExecContext(ctx, `DELETE FROM resources_fts WHERE rowid = ?`, ftsRowID(w.Resource, id)); err != nil {
				return fmt.Errorf("mirror %s: deleting search entry %s: %w", w.Resource, id, err)
			}
		}
		if w.Stamp {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO sync_state (resource_type, last_cursor, last_synced_at, total_count, last_attempt_complete)
				 VALUES (?, '', ?, ?, ?)
				 ON CONFLICT(resource_type) DO UPDATE SET last_cursor = excluded.last_cursor,
				 last_synced_at = excluded.last_synced_at, total_count = excluded.total_count,
				 last_attempt_complete = excluded.last_attempt_complete`,
				w.Resource, now, w.Count, !w.Partial); err != nil {
				return fmt.Errorf("mirror %s: stamping sync state: %w", w.Resource, err)
			}
		}
	}
	return tx.Commit()
}
