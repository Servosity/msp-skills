// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// walkCoverageCreateSQL is the record of what each sync walk provably
// enumerated (hand-written, huntress fix C). sync_state keeps one row per
// resource type, with the start of a flat walk but the END of a dependent
// fan-out walk, and nothing about whether the walk was filtered or resumed,
// so it cannot prove that a row a later walk did not return was removed
// upstream. One row here per (resource_type, scope): scope is "" for the
// resource-level walk (a flat walk, or the whole dependent fan-out) and the
// parent id for one partition of a dependent walk.
//
//   - attempt_* is noted when a walk starts: from the first page it starts a
//     new logical walk; resumed from a saved page cursor it continues the
//     logical walk an earlier, interrupted run started (whose pages that run
//     already wrote), and a filter on any of its runs marks it filtered.
//   - last_* is the most recent finished walk of the scope, dated from the
//     start of its logical walk.
//   - full_* is the most recent walk that started from the first page in
//     the same run, reached a proven natural end, and carried no
//     row-selecting filter (no --param, --global-param, --resource-param,
//     since or date window): the only walk whose silence about a row proves
//     the row is gone.
//
// Times are RFC3339Nano UTC.
const walkCoverageCreateSQL = `CREATE TABLE IF NOT EXISTS walk_coverage (
	resource_type TEXT NOT NULL,
	scope TEXT NOT NULL,
	last_started_at TEXT,
	last_ended_at TEXT,
	last_complete INTEGER NOT NULL DEFAULT 0,
	last_filtered INTEGER NOT NULL DEFAULT 0,
	last_resumed INTEGER NOT NULL DEFAULT 0,
	full_started_at TEXT,
	full_ended_at TEXT,
	attempt_started_at TEXT,
	attempt_filtered INTEGER NOT NULL DEFAULT 0,
	PRIMARY KEY (resource_type, scope)
)`

// migrateExtras runs after the generated store migrations and before the
// schema-version stamp. It is the canonical place for novel-feature auxiliary
// tables that need to live in the local store.
//
// Edit this file when adding tables for novel commands. Keep migrations
// idempotent with CREATE TABLE IF NOT EXISTS / CREATE INDEX IF NOT EXISTS so
// every store open can safely re-run them.
func (s *Store) migrateExtras(ctx context.Context, conn *sql.Conn) error {
	migrations := []string{
		walkCoverageCreateSQL,
	}
	for _, m := range migrations {
		if _, err := conn.ExecContext(ctx, m); err != nil {
			return fmt.Errorf("extra migration failed: %w", err)
		}
	}
	return nil
}

func walkBit(b bool) int {
	if b {
		return 1
	}
	return 0
}

// NoteWalkStart notes that a walk of resourceType's scope is starting: a new
// logical walk when it starts from the first page (resumed false), or the
// continuation of the interrupted one whose saved page cursor it resumes, in
// which case the logical walk keeps its start and is filtered if any of its
// runs was.
func (s *Store) NoteWalkStart(resourceType, scope string, started time.Time, filtered, resumed bool) error {
	s.lockForWrite()
	defer s.unlockAfterWrite()
	if _, err := s.db.Exec(walkCoverageCreateSQL); err != nil {
		return err
	}
	if !resumed {
		_, err := s.db.Exec(`INSERT INTO walk_coverage (resource_type, scope, attempt_started_at, attempt_filtered) VALUES (?, ?, ?, ?)
			ON CONFLICT(resource_type, scope) DO UPDATE SET attempt_started_at = excluded.attempt_started_at, attempt_filtered = excluded.attempt_filtered`,
			resourceType, scope, started.UTC().Format(time.RFC3339Nano), walkBit(filtered))
		return err
	}
	_, err := s.db.Exec(`INSERT INTO walk_coverage (resource_type, scope, attempt_started_at, attempt_filtered) VALUES (?, ?, NULL, ?)
		ON CONFLICT(resource_type, scope) DO UPDATE SET attempt_filtered = MAX(walk_coverage.attempt_filtered, excluded.attempt_filtered)`,
		resourceType, scope, walkBit(filtered))
	return err
}

// RecordWalkCoverage records one finished walk of resourceType's scope: when
// it started and ended, whether it reached a proven natural end (complete),
// whether a row-selecting filter narrowed it, and whether it resumed from a
// saved page cursor. A resumed walk is dated from the start of the logical
// walk it continues (NoteWalkStart; this run's start when that is unknown),
// is filtered if any of its runs was, and is never a full walk: the pages
// before its cursor were read by another run, and the list may have shifted
// between the runs. A complete, unfiltered walk from the first page becomes
// the scope's full walk; any other walk leaves the previous full walk in
// place, so a later filtered, resumed or unfinished walk never erases the
// proof an earlier full one gave.
func (s *Store) RecordWalkCoverage(resourceType, scope string, started, ended time.Time, complete, filtered, resumed bool) error {
	s.lockForWrite()
	defer s.unlockAfterWrite()
	if _, err := s.db.Exec(walkCoverageCreateSQL); err != nil {
		return err
	}
	startedText := started.UTC().Format(time.RFC3339Nano)
	if resumed {
		var attemptStarted sql.NullString
		var attemptFiltered sql.NullInt64
		err := s.db.QueryRow(`SELECT attempt_started_at, attempt_filtered FROM walk_coverage WHERE resource_type = ? AND scope = ?`,
			resourceType, scope).Scan(&attemptStarted, &attemptFiltered)
		if err != nil && err != sql.ErrNoRows {
			return err
		}
		if attemptStarted.Valid && attemptStarted.String != "" {
			startedText = attemptStarted.String
		}
		if attemptFiltered.Int64 != 0 {
			filtered = true
		}
	}
	var fullStarted, fullEnded sql.NullString
	if complete && !filtered && !resumed {
		fullStarted = sql.NullString{String: startedText, Valid: true}
		fullEnded = sql.NullString{String: ended.UTC().Format(time.RFC3339Nano), Valid: true}
	}
	_, err := s.db.Exec(`INSERT INTO walk_coverage (resource_type, scope, last_started_at, last_ended_at, last_complete, last_filtered, last_resumed, full_started_at, full_ended_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(resource_type, scope) DO UPDATE SET
			last_started_at = excluded.last_started_at, last_ended_at = excluded.last_ended_at,
			last_complete = excluded.last_complete, last_filtered = excluded.last_filtered, last_resumed = excluded.last_resumed,
			full_started_at = COALESCE(excluded.full_started_at, walk_coverage.full_started_at),
			full_ended_at = CASE WHEN excluded.full_started_at IS NULL THEN walk_coverage.full_ended_at ELSE excluded.full_ended_at END`,
		resourceType, scope, startedText, ended.UTC().Format(time.RFC3339Nano),
		walkBit(complete), walkBit(filtered), walkBit(resumed), fullStarted, fullEnded)
	return err
}
