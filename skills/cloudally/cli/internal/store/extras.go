// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package store

import (
	"context"
	"database/sql"
	"errors"
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
		// Restore ledger: one batch per 'restore submit' invocation.
		// scope is distributor, reseller, or account; dates and options are
		// JSON documents so the schema stays stable as flags grow.
		`CREATE TABLE IF NOT EXISTS restore_batches (
			batch_id        TEXT PRIMARY KEY,
			batch_key       TEXT NOT NULL,
			created_at      TEXT NOT NULL,
			scope           TEXT NOT NULL,
			account_id      TEXT NOT NULL DEFAULT '',
			task_id         TEXT NOT NULL,
			source          TEXT NOT NULL DEFAULT '',
			label           TEXT NOT NULL DEFAULT '',
			dates           TEXT NOT NULL DEFAULT '[]',
			options         TEXT NOT NULL DEFAULT '{}',
			last_refresh_at TEXT
		)`,
		`CREATE INDEX IF NOT EXISTS idx_restore_batches_key ON restore_batches(batch_key)`,
		`CREATE INDEX IF NOT EXISTS idx_restore_batches_created ON restore_batches(created_at)`,
		// Restore ledger: one row per (entity, requested date) restore.
		// status moves PLANNED -> SUBMITTING before the request is sent,
		// then SUBMITTED | FAILED | FAILED_REDIRECT | UNKNOWN once the
		// outcome is known, then takes the raw RESTORE task status (or
		// UNMATCHED) as 'restore watch' refreshes it. Because a task can
		// itself report FAILED, status alone cannot say whether the API
		// ever accepted the restore; the submit_status column added below
		// holds the submission outcome on its own. The inline UNIQUE on
		// resolved_raw stays for existing ledgers; the identity --resume
		// relies on is the (batch_id, entity, requested) index added below.
		`CREATE TABLE IF NOT EXISTS restore_jobs (
			id            INTEGER PRIMARY KEY AUTOINCREMENT,
			batch_id      TEXT NOT NULL REFERENCES restore_batches(batch_id),
			entity        TEXT NOT NULL,
			ext_id        TEXT NOT NULL DEFAULT '',
			requested     TEXT NOT NULL,
			resolved_raw  INTEGER NOT NULL,
			resolved      TEXT NOT NULL,
			sub_source    TEXT NOT NULL DEFAULT '',
			status        TEXT NOT NULL,
			http_status   INTEGER,
			response      TEXT,
			error         TEXT,
			task_id       TEXT,
			progress      TEXT,
			submitted_at  TEXT,
			updated_at    TEXT NOT NULL,
			UNIQUE(batch_id, entity, resolved_raw)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_restore_jobs_batch ON restore_jobs(batch_id)`,
		`CREATE INDEX IF NOT EXISTS idx_restore_jobs_status ON restore_jobs(status)`,
	}
	for _, m := range migrations {
		if _, err := conn.ExecContext(ctx, m); err != nil {
			return fmt.Errorf("extra migration failed: %w", err)
		}
	}
	// Columns added after the first ledger release. ensureColumn reads the
	// table's column list first, since SQLite has no ADD COLUMN IF NOT EXISTS.
	for _, col := range restoreBatchAddedColumns {
		if err := s.ensureColumn(ctx, conn, "restore_batches", col.name, col.decl); err != nil {
			return fmt.Errorf("extra migration failed: %w", err)
		}
	}
	if _, err := conn.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_restore_batches_resume ON restore_batches(resume_key)`); err != nil {
		return fmt.Errorf("extra migration failed: %w", err)
	}
	for _, col := range restoreJobAddedColumns {
		if err := s.ensureColumn(ctx, conn, "restore_jobs", col.name, col.decl); err != nil {
			return fmt.Errorf("extra migration failed: %w", err)
		}
	}
	// Rows written before submit_status existed, or since by an older
	// binary, carry '' there. Every open fills them in from the older
	// columns; rows this binary writes always set it.
	if _, err := conn.ExecContext(ctx, `UPDATE restore_jobs SET submit_status = `+LegacySubmitStatusSQL("")+` WHERE submit_status = ''`); err != nil {
		return fmt.Errorf("extra migration failed: backfilling submit_status: %w", err)
	}
	return ensureRestoreJobIdentity(ctx, conn)
}

// restoreJobAddedColumns are restore_jobs columns newer than the original
// CREATE TABLE. submit_status is the submission outcome alone: PLANNED,
// SUBMITTING, SUBMITTED, FAILED (rejected before it ran), FAILED_REDIRECT,
// or UNKNOWN. 'restore watch' never writes a task status into it; the only
// change it makes is settling an UNKNOWN or SUBMITTING row it matched to a
// RESTORE task as SUBMITTED. Whether a row may be sent again is decided on
// this column, never on status.
var restoreJobAddedColumns = []struct{ name, decl string }{
	{"submit_status", "TEXT NOT NULL DEFAULT ''"},
}

// LegacySubmitStatusSQL derives a restore_jobs row's submission outcome
// from the columns written before submit_status existed, when 'restore
// watch' still wrote the RESTORE task status over the submission outcome.
// prefix qualifies the column names (for example "j."). A row the API
// accepted is recognised by a recorded task id or a 2xx answer, so a task
// that later reported FAILED is never read as a rejection that is safe to
// send again; every other status watch can write (task statuses, UNMATCHED)
// also means the restore was accepted.
func LegacySubmitStatusSQL(prefix string) string {
	p := prefix
	return `(CASE
		WHEN COALESCE(` + p + `task_id, '') <> '' THEN 'SUBMITTED'
		WHEN ` + p + `status IN ('PLANNED', 'SUBMITTING', 'SUBMITTED', 'UNKNOWN', 'FAILED_REDIRECT') THEN ` + p + `status
		WHEN ` + p + `status = 'FAILED' AND COALESCE(` + p + `http_status, 0) BETWEEN 200 AND 299 THEN 'SUBMITTED'
		WHEN ` + p + `status = 'FAILED' THEN 'FAILED'
		ELSE 'SUBMITTED' END)`
}

// restoreBatchAddedColumns are restore_batches columns newer than the
// original CREATE TABLE. resume_key is the account-free fingerprint used to
// find a batch on --resume; auth_scope records how the active token was
// obtained (partner, account-login, use-account) when the batch ran.
var restoreBatchAddedColumns = []struct{ name, decl string }{
	{"resume_key", "TEXT NOT NULL DEFAULT ''"},
	{"auth_scope", "TEXT NOT NULL DEFAULT ''"},
}

// RestoreJobIdentityIndex enforces one ledger row per mailbox per requested
// date within a batch, so --resume never adds a second row when the same
// date later resolves to a newer backup.
const RestoreJobIdentityIndex = "idx_restore_jobs_identity"

// ensureRestoreJobIdentity creates the (batch_id, entity, requested) unique
// index. Ledgers written before the index existed may hold duplicates for
// that key; the oldest row (lowest id) is kept and newer duplicates are
// removed first so the index can be built.
func ensureRestoreJobIdentity(ctx context.Context, conn *sql.Conn) error {
	var name string
	err := conn.QueryRowContext(ctx, `SELECT name FROM sqlite_master WHERE type='index' AND name=?`, RestoreJobIdentityIndex).Scan(&name)
	if err == nil {
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("checking index %s: %w", RestoreJobIdentityIndex, err)
	}
	if _, err := conn.ExecContext(ctx, `DELETE FROM restore_jobs WHERE id NOT IN (
		SELECT MIN(id) FROM restore_jobs GROUP BY batch_id, entity, requested)`); err != nil {
		return fmt.Errorf("removing duplicate restore rows: %w", err)
	}
	if _, err := conn.ExecContext(ctx, `CREATE UNIQUE INDEX IF NOT EXISTS `+RestoreJobIdentityIndex+` ON restore_jobs(batch_id, entity, requested)`); err != nil {
		return fmt.Errorf("creating %s: %w", RestoreJobIdentityIndex, err)
	}
	return nil
}
