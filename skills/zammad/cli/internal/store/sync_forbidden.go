// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package store

import (
	"fmt"
	"strings"
	"time"
)

// A resource the vendor refuses this token (HTTP 403) never completes, so its
// sync_state row stays at last_attempt_complete = 0 and held the whole store
// at partial (#692). The refusal is recorded here rather than in that column,
// because commands read the column as a plain 0 or 1. The table appears the
// first time a refusal is recorded, so a read treats a missing one as empty.
// Hand-fix (handfixes.json sync-forbidden-not-partial);
// tools/maintainer/replay_handfixes.py writes this file from
// handfix_replay/sync_forbidden_store.go.tmpl after a reprint.
const createSyncForbidden = `CREATE TABLE IF NOT EXISTS sync_forbidden (
	resource_type TEXT PRIMARY KEY,
	recorded_at TEXT NOT NULL
)`

func isMissingSyncForbidden(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "no such table")
}

// MarkSyncForbidden records that the latest attempt at this sync_state key was
// refused outright.
func (s *Store) MarkSyncForbidden(stateKey string) error {
	s.lockForWrite()
	defer s.unlockAfterWrite()
	if _, err := s.db.Exec(createSyncForbidden); err != nil {
		return fmt.Errorf("creating sync_forbidden: %w", err)
	}
	_, err := s.db.Exec(`INSERT INTO sync_forbidden (resource_type, recorded_at) VALUES (?, ?)
		ON CONFLICT(resource_type) DO UPDATE SET recorded_at = excluded.recorded_at`,
		stateKey, time.Now().UTC().Format(time.RFC3339))
	return err
}

// ClearSyncForbidden forgets a refusal. Each attempt calls it as it starts, so
// a walk that later dies mid-way can never read as refused.
func (s *Store) ClearSyncForbidden(stateKey string) error {
	s.lockForWrite()
	defer s.unlockAfterWrite()
	_, err := s.db.Exec(`DELETE FROM sync_forbidden WHERE resource_type = ?`, stateKey)
	if isMissingSyncForbidden(err) {
		return nil
	}
	return err
}

// SyncCoverage names the sync_state keys whose latest attempt did not finish,
// split into the ones the vendor refused and everything else.
type SyncCoverage struct {
	SkippedForbidden []string
	Incomplete       []string
}

// SyncCoverage reads the store's coverage. It writes nothing, so it works on a
// read-only store.
func (s *Store) SyncCoverage() (SyncCoverage, error) {
	var out SyncCoverage
	forbidden := map[string]bool{}
	marks, err := s.db.Query(`SELECT resource_type FROM sync_forbidden`)
	if err != nil && !isMissingSyncForbidden(err) {
		return out, err
	}
	if err == nil {
		defer marks.Close()
		for marks.Next() {
			var key string
			if err := marks.Scan(&key); err != nil {
				return out, err
			}
			forbidden[key] = true
		}
		if err := marks.Err(); err != nil {
			return out, err
		}
	}
	rows, err := s.db.Query(`SELECT resource_type FROM sync_state WHERE last_attempt_complete != 1 ORDER BY resource_type`)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return out, err
		}
		if forbidden[key] {
			out.SkippedForbidden = append(out.SkippedForbidden, key)
		} else {
			out.Incomplete = append(out.Incomplete, key)
		}
	}
	return out, rows.Err()
}
