// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package granola

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// granolaSchemaSQL is the set of CREATE TABLE statements added on top of
// the generator-owned store.go schema. All statements are idempotent
// (IF NOT EXISTS) so we can run them every open without harm.
var granolaSchemaSQL = []string{
	`CREATE TABLE IF NOT EXISTS meetings (
		id TEXT PRIMARY KEY,
		title TEXT,
		created_at TEXT,
		updated_at TEXT,
		started_at TEXT,
		ended_at TEXT,
		workspace_id TEXT,
		calendar_event_id TEXT,
		deleted_at TEXT,
		notes_markdown TEXT,
		notes_plain TEXT,
		transcript_available INTEGER NOT NULL DEFAULT 0,
		recipes_applied TEXT,
		creation_source TEXT,
		valid_meeting INTEGER NOT NULL DEFAULT 0
	)`,
	`CREATE INDEX IF NOT EXISTS idx_meetings_started ON meetings(started_at)`,
	`CREATE INDEX IF NOT EXISTS idx_meetings_created ON meetings(created_at)`,
	`CREATE INDEX IF NOT EXISTS idx_meetings_workspace ON meetings(workspace_id)`,

	`CREATE TABLE IF NOT EXISTS attendees (
		meeting_id TEXT NOT NULL,
		email TEXT NOT NULL,
		name TEXT,
		response_status TEXT,
		PRIMARY KEY (meeting_id, email)
	)`,
	`CREATE INDEX IF NOT EXISTS idx_attendees_email ON attendees(email)`,

	`CREATE TABLE IF NOT EXISTS transcript_segments (
		meeting_id TEXT NOT NULL,
		idx INTEGER NOT NULL,
		source TEXT,
		text TEXT,
		start_ts_ms INTEGER,
		end_ts_ms INTEGER,
		confidence REAL,
		PRIMARY KEY (meeting_id, idx)
	)`,
	`CREATE INDEX IF NOT EXISTS idx_segments_meeting ON transcript_segments(meeting_id)`,
	`CREATE INDEX IF NOT EXISTS idx_segments_source ON transcript_segments(source)`,

	`CREATE TABLE IF NOT EXISTS folders (
		id TEXT PRIMARY KEY,
		title TEXT,
		parent_id TEXT,
		workspace_id TEXT,
		owner_id TEXT,
		preset TEXT
	)`,

	`CREATE TABLE IF NOT EXISTS folder_memberships (
		folder_id TEXT NOT NULL,
		meeting_id TEXT NOT NULL,
		PRIMARY KEY (folder_id, meeting_id)
	)`,
	`CREATE INDEX IF NOT EXISTS idx_fm_meeting ON folder_memberships(meeting_id)`,

	`CREATE TABLE IF NOT EXISTS panel_templates (
		id TEXT PRIMARY KEY,
		slug TEXT,
		title TEXT,
		description TEXT,
		category TEXT
	)`,
	`CREATE INDEX IF NOT EXISTS idx_panel_templates_slug ON panel_templates(slug)`,

	`CREATE TABLE IF NOT EXISTS recipes (
		id TEXT PRIMARY KEY,
		slug TEXT,
		name TEXT,
		description TEXT,
		category TEXT,
		source TEXT
	)`,
	`CREATE INDEX IF NOT EXISTS idx_recipes_slug ON recipes(slug)`,

	`CREATE TABLE IF NOT EXISTS recipes_usage (
		recipe_id TEXT PRIMARY KEY,
		total_count INTEGER NOT NULL DEFAULT 0,
		last_used_at TEXT
	)`,

	`CREATE TABLE IF NOT EXISTS chat_threads (
		id TEXT PRIMARY KEY,
		meeting_id TEXT,
		workspace_id TEXT,
		title TEXT,
		created_at TEXT,
		updated_at TEXT
	)`,
	`CREATE INDEX IF NOT EXISTS idx_chat_threads_meeting ON chat_threads(meeting_id)`,

	`CREATE TABLE IF NOT EXISTS chat_messages (
		id TEXT PRIMARY KEY,
		thread_id TEXT NOT NULL,
		role TEXT,
		turn_index INTEGER,
		content TEXT,
		created_at TEXT
	)`,
	`CREATE INDEX IF NOT EXISTS idx_chat_messages_thread ON chat_messages(thread_id)`,

	`CREATE TABLE IF NOT EXISTS workspaces (
		id TEXT PRIMARY KEY,
		display_name TEXT,
		plan_type TEXT,
		role TEXT,
		raw TEXT
	)`,

	// Which meetings Granola's MCP has already been asked for private notes,
	// tracked apart from the REST timestamps so a failed or not-yet-connected
	// MCP pass is retried (handfixes.json: mcp-notes-backfill-tracked).
	// resolved = 1: MCP answered for this meeting (notes stored, or none).
	// resolved = 0: MCP listed no match; retried after a day, since MCP's
	// list can lag.
	`CREATE TABLE IF NOT EXISTS human_notes_state (
		meeting_id TEXT PRIMARY KEY,
		checked_at TEXT,
		resolved INTEGER NOT NULL DEFAULT 0
	)`,

	// AI summary from the REST detail endpoint, kept so local exports can
	// include it (handfixes.json: export-summary-stored-locally).
	`CREATE TABLE IF NOT EXISTS meeting_summaries (
		meeting_id TEXT PRIMARY KEY,
		summary_markdown TEXT,
		summary_text TEXT
	)`,

	`CREATE VIRTUAL TABLE IF NOT EXISTS meetings_fts USING fts5(
		title, notes_plain, content='meetings', content_rowid='rowid', tokenize='porter unicode61'
	)`,

	`CREATE VIRTUAL TABLE IF NOT EXISTS transcript_fts USING fts5(
		text, content='transcript_segments', tokenize='porter unicode61'
	)`,
}

// EnsureSchema runs the additive Granola-specific migrations. Idempotent.
// Call this from any command that touches the granola tables.
func EnsureSchema(ctx context.Context, db *sql.DB) error {
	for _, stmt := range granolaSchemaSQL {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("granola schema: %w: %s", err, firstLine(stmt))
		}
	}
	if err := ensureFTSTriggers(ctx, db); err != nil {
		return err
	}
	// Additive column migrations: CREATE TABLE IF NOT EXISTS never alters a
	// table an earlier build created, so a column added later is added here
	// (handfixes.json: mcp-notes-backfill-tracked). Existing rows keep their
	// data and take the column's default.
	if err := addColumnIfMissing(ctx, db, "human_notes_state", "resolved",
		`ALTER TABLE human_notes_state ADD COLUMN resolved INTEGER NOT NULL DEFAULT 0`); err != nil {
		return err
	}
	// Meeting timestamps are compared and ordered as TEXT, which is only
	// chronological in ONE representation: UTC, whole seconds, "Z"
	// (2026-10-13T13:00:00Z, what the --since bounds use). Earlier builds
	// stored calendar times with their offset ("...-04:00") and API times
	// with fractional seconds ("...13:00:00.100Z", which sorts AFTER a
	// 13:00:00Z bound only by accident); rewrite any value not already in
	// that form (handfixes.json: meeting-times-stored-utc). Idempotent: empty
	// and unparseable values are left alone.
	for _, stmt := range utcNormalizeSQL {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("granola schema: normalize timestamps: %w", err)
		}
	}
	return nil
}

var utcNormalizeSQL = []string{
	`UPDATE meetings SET started_at = strftime('%Y-%m-%dT%H:%M:%SZ', started_at)
		WHERE strftime('%Y-%m-%dT%H:%M:%SZ', started_at) IS NOT NULL AND started_at <> strftime('%Y-%m-%dT%H:%M:%SZ', started_at)`,
	`UPDATE meetings SET ended_at = strftime('%Y-%m-%dT%H:%M:%SZ', ended_at)
		WHERE strftime('%Y-%m-%dT%H:%M:%SZ', ended_at) IS NOT NULL AND ended_at <> strftime('%Y-%m-%dT%H:%M:%SZ', ended_at)`,
	`UPDATE meetings SET created_at = strftime('%Y-%m-%dT%H:%M:%SZ', created_at)
		WHERE strftime('%Y-%m-%dT%H:%M:%SZ', created_at) IS NOT NULL AND created_at <> strftime('%Y-%m-%dT%H:%M:%SZ', created_at)`,
	`UPDATE meetings SET updated_at = strftime('%Y-%m-%dT%H:%M:%SZ', updated_at)
		WHERE strftime('%Y-%m-%dT%H:%M:%SZ', updated_at) IS NOT NULL AND updated_at <> strftime('%Y-%m-%dT%H:%M:%SZ', updated_at)`,
}

// ftsTriggerSQL keeps the external-content FTS tables in step with their
// content tables inside the SAME transaction as each row write, so search can
// never serve a row it has not indexed or keep text the row no longer has
// (handfixes.json: fts-maintained-by-triggers). The upsert in rest_hydrate.go
// is INSERT ... ON CONFLICT DO UPDATE, which fires the UPDATE trigger; a
// REPLACE would not fire the DELETE trigger and must never be used here.
var ftsTriggerSQL = []string{
	`CREATE TRIGGER IF NOT EXISTS meetings_fts_ai AFTER INSERT ON meetings BEGIN
		INSERT INTO meetings_fts(rowid, title, notes_plain) VALUES (new.rowid, new.title, new.notes_plain);
	END`,
	`CREATE TRIGGER IF NOT EXISTS meetings_fts_ad AFTER DELETE ON meetings BEGIN
		INSERT INTO meetings_fts(meetings_fts, rowid, title, notes_plain) VALUES ('delete', old.rowid, old.title, old.notes_plain);
	END`,
	`CREATE TRIGGER IF NOT EXISTS meetings_fts_au AFTER UPDATE OF title, notes_plain ON meetings BEGIN
		INSERT INTO meetings_fts(meetings_fts, rowid, title, notes_plain) VALUES ('delete', old.rowid, old.title, old.notes_plain);
		INSERT INTO meetings_fts(rowid, title, notes_plain) VALUES (new.rowid, new.title, new.notes_plain);
	END`,
	`CREATE TRIGGER IF NOT EXISTS transcript_fts_ai AFTER INSERT ON transcript_segments BEGIN
		INSERT INTO transcript_fts(rowid, text) VALUES (new.rowid, new.text);
	END`,
	`CREATE TRIGGER IF NOT EXISTS transcript_fts_ad AFTER DELETE ON transcript_segments BEGIN
		INSERT INTO transcript_fts(transcript_fts, rowid, text) VALUES ('delete', old.rowid, old.text);
	END`,
	`CREATE TRIGGER IF NOT EXISTS transcript_fts_au AFTER UPDATE OF text ON transcript_segments BEGIN
		INSERT INTO transcript_fts(transcript_fts, rowid, text) VALUES ('delete', old.rowid, old.text);
		INSERT INTO transcript_fts(rowid, text) VALUES (new.rowid, new.text);
	END`,
}

// ensureFTSTriggers installs ftsTriggerSQL once. A 'delete' trigger is only
// correct when the index already matches the rows, and a store from an
// earlier build may hold an index that lags them, so the triggers and one
// full rebuild land in a single transaction: either both happen or neither.
func ensureFTSTriggers(ctx context.Context, db *sql.DB) error {
	var n int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type = 'trigger'
		AND name IN ('meetings_fts_ai','meetings_fts_ad','meetings_fts_au','transcript_fts_ai','transcript_fts_ad','transcript_fts_au')`).Scan(&n); err != nil {
		return fmt.Errorf("granola schema: inspect fts triggers: %w", err)
	}
	if n == len(ftsTriggerSQL) {
		return nil
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("granola schema: fts triggers: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	stmts := append(append([]string{}, ftsTriggerSQL...),
		`INSERT INTO meetings_fts(meetings_fts) VALUES ('rebuild')`,
		`INSERT INTO transcript_fts(transcript_fts) VALUES ('rebuild')`)
	for _, stmt := range stmts {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("granola schema: fts triggers: %w: %s", err, firstLine(stmt))
		}
	}
	return tx.Commit()
}

// addColumnIfMissing runs alterStmt (a constant ALTER TABLE ... ADD COLUMN)
// unless table already has column.
func addColumnIfMissing(ctx context.Context, db *sql.DB, table, column, alterStmt string) error {
	rows, err := db.QueryContext(ctx, "SELECT name FROM pragma_table_info(?)", table)
	if err != nil {
		return fmt.Errorf("granola schema: inspect %s: %w", table, err)
	}
	found := false
	for rows.Next() {
		var name string
		if rows.Scan(&name) == nil && name == column {
			found = true
		}
	}
	rerr := rows.Err()
	rows.Close()
	if rerr != nil {
		return fmt.Errorf("granola schema: inspect %s: %w", table, rerr)
	}
	if found {
		return nil
	}
	if _, err := db.ExecContext(ctx, alterStmt); err != nil {
		return fmt.Errorf("granola schema: add %s.%s: %w", table, column, err)
	}
	return nil
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
