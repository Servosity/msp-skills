// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// Human/private-notes enrichment via Granola's official MCP. The public REST
// API omits the user's raw typed notes; the MCP get_meetings tool returns them
// in a <private_notes> block. get_meetings accepts REST note ids directly
// (note_ids), so each meeting is fetched by its own id - no title/date join.
//
// This runs during `enrich` (best-effort, only when MCP is connected) and
// writes the notes into meetings.notes_markdown/notes_plain, so every
// human-notes command reads them through the normal store→cache backfill.

import (
	"context"
	"database/sql"
	"html"
	"regexp"
	"strings"
	"time"
)

// restTarget is a REST-synced meeting we want human notes for.
type restTarget struct {
	ID      string // not_ id
	Title   string
	Created time.Time
}

var (
	// A meeting block's attributes vary by server version (id, title, date,
	// meeting_at, created_at, url, ...), so the block is matched loosely.
	mcpBlockRe   = regexp.MustCompile(`(?s)<meeting\b([^>]*)>(.*?)</meeting>`)
	mcpPrivateRe = regexp.MustCompile(`(?s)<private_notes>(.*?)</private_notes>`)
)

// enrichHumanNotes fetches private notes for the given REST meetings via
// Granola's official MCP and writes them into meetings.notes_markdown /
// notes_plain. Best-effort: returns (0, errMCPNotConnected) when MCP isn't
// connected so the caller can hint without failing the sync. Returns the
// count of meetings whose notes were set.
//
// Hand-wired (handfixes.json: mcp-notes-by-note-id): each meeting is asked
// for BY ITS REST NOTE ID (get_meetings note_ids), one id per call, so the
// one block that comes back is that meeting's - no title/date matching. The
// old path listed meetings and matched them by title and date; the
// list_meetings tag grew extra attributes, the parser matched nothing, and
// every private note was silently dropped (proved against a live account:
// 0 of 150 meetings matched while get_meetings by note id returned notes).
func enrichHumanNotes(ctx context.Context, db *sql.DB, targets []restTarget) (int, error) {
	if len(targets) == 0 {
		return 0, nil
	}
	access, err := mcpAccessToken()
	if err != nil {
		return 0, err // errMCPNotConnected or expired — caller decides how to hint
	}
	sid, err := mcpSession(access)
	if err != nil {
		return 0, err
	}
	checkedAt := time.Now().UTC().Format(time.RFC3339)
	n := 0
	for _, t := range targets {
		if validateNoteIDForFilename(t.ID) != nil {
			continue
		}
		txt, isErr := mcpToolCall(access, sid, "get_meetings", map[string]any{"note_ids": []string{t.ID}})
		if isErr {
			continue // left pending; retried on a later pass
		}
		note, resolved := privateNoteFromGetMeetings(txt)
		if err := recordHumanNotes(ctx, db, t.ID, note, resolved, checkedAt); err != nil {
			return n, err
		}
		if note != "" {
			n++
		}
	}
	if n > 0 {
		_, _ = db.ExecContext(ctx, `INSERT INTO meetings_fts(meetings_fts) VALUES ('rebuild')`)
	}
	return n, nil
}

// recordHumanNotes stores a meeting's private notes (when non-empty) and its
// MCP completion state together, in one transaction.
func recordHumanNotes(ctx context.Context, db *sql.DB, id, note string, resolved bool, checkedAt string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if note != "" {
		if _, err := tx.ExecContext(ctx, `UPDATE meetings SET notes_markdown = ?, notes_plain = ? WHERE id = ?`, note, note, id); err != nil {
			return err
		}
	}
	r := 0
	if resolved {
		r = 1
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO human_notes_state(meeting_id, checked_at, resolved) VALUES (?, ?, ?)
		ON CONFLICT(meeting_id) DO UPDATE SET checked_at = excluded.checked_at, resolved = excluded.resolved`, id, checkedAt, r); err != nil {
		return err
	}
	return tx.Commit()
}

// markHumanNotesChecked records that MCP was asked about these meetings and
// matched none of them (unresolved: retried after a day).
func markHumanNotesChecked(ctx context.Context, db *sql.DB, targets []restTarget) {
	checkedAt := time.Now().UTC().Format(time.RFC3339)
	for _, t := range targets {
		_ = recordHumanNotes(ctx, db, t.ID, "", false, checkedAt)
	}
}

// pendingHumanNoteTargets returns up to limit hydrated meetings (newest first)
// that Granola's MCP has not answered for: never asked (hydrated before MCP
// was connected, or the MCP pass failed), or listed no match more than a day
// ago.
func pendingHumanNoteTargets(ctx context.Context, db *sql.DB, limit int) ([]restTarget, error) {
	lim := limit
	if lim <= 0 {
		lim = -1
	}
	retryBefore := time.Now().UTC().Add(-24 * time.Hour).Format(time.RFC3339)
	rows, err := db.QueryContext(ctx, `SELECT m.id, COALESCE(m.title,''), COALESCE(m.created_at,'') FROM meetings m
		LEFT JOIN human_notes_state h ON h.meeting_id = m.id
		WHERE m.id LIKE 'not_%'
		  AND (h.meeting_id IS NULL OR (h.resolved = 0 AND COALESCE(h.checked_at,'') < ?))
		ORDER BY m.created_at DESC
		LIMIT ?`, retryBefore, lim)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []restTarget
	for rows.Next() {
		var id, title, created string
		if err := rows.Scan(&id, &title, &created); err != nil {
			return nil, err
		}
		out = append(out, restTarget{ID: id, Title: title, Created: rfcToTime(created)})
	}
	return out, rows.Err()
}

// privateNoteFromGetMeetings reads a get_meetings answer for ONE requested
// note. resolved is true only when exactly one meeting block came back (that
// note's); zero blocks means MCP does not know the note, more than one is
// ambiguous, and either way the meeting is left unresolved for a later retry.
func privateNoteFromGetMeetings(txt string) (note string, resolved bool) {
	blocks := mcpBlockRe.FindAllStringSubmatch(txt, -1)
	if len(blocks) != 1 {
		return "", false
	}
	if pn := mcpPrivateRe.FindStringSubmatch(blocks[0][2]); pn != nil {
		note = strings.TrimSpace(html.UnescapeString(pn[1]))
	}
	return note, true
}
