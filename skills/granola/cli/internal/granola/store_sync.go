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
	return nil
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
