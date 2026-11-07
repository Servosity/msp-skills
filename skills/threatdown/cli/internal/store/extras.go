// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
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

// UpsertBatchReplacing is UpsertBatch for a caller that holds the console's
// whole record (fleet sync): each row replaces the stored one instead of
// keeping richer old fields, so a field the console cleared is cleared here.
// Every other writer (live reads, mutation responses, generic sync) keeps the
// merge, because those bodies can be partial. Hand-fix (handfixes.json
// fleet-sync-mirrors-the-console).
func (s *Store) UpsertBatchReplacing(resourceType string, items []json.RawMessage) (int, int, error) {
	replaceMu.Lock()
	if replaceFor[s] == nil {
		replaceFor[s] = map[string]int{}
	}
	replaceFor[s][resourceType]++
	replaceMu.Unlock()
	defer func() {
		replaceMu.Lock()
		if replaceFor[s][resourceType]--; replaceFor[s][resourceType] <= 0 {
			delete(replaceFor[s], resourceType)
		}
		if len(replaceFor[s]) == 0 {
			delete(replaceFor, s)
		}
		replaceMu.Unlock()
	}()
	return s.UpsertBatch(resourceType, items)
}

// ponytail: per-store flag map, not a per-call option, because UpsertBatch's
// generated signature has no room for one; a concurrent writer of the same
// resource on the same *Store during a replacing batch would replace too.
var (
	replaceMu  sync.Mutex
	replaceFor = map[*Store]map[string]int{}
)

func (s *Store) replacing(resourceType string) bool {
	replaceMu.Lock()
	defer replaceMu.Unlock()
	return replaceFor[s][resourceType] > 0
}
