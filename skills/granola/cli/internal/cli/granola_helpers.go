// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"granola-pp-cli/internal/config"
	"granola-pp-cli/internal/granola"
	"granola-pp-cli/internal/store"
)

// openGranolaCache builds the in-memory Cache the read commands consume.
//
// Granola v7.4x+ sealed the desktop cache (cache-v6.json.enc, guarded by its
// own Keychain access group), so a third-party binary can never decrypt it.
// This connector therefore sources ENTIRELY from the public REST API, which
// `sync` lands in the local SQLite store. The desktop-cache read is gone: we
// start from an empty compatibility Cache and populate every block from the
// store.
//
// A MISSING store (fresh install, before the first `sync`) is NOT an error -
// openGranolaStoreRead returns (nil, nil) and each backfill no-ops, so
// commands still run against an empty (nil-safe) cache. Only genuine store
// open/query/scan failures propagate, so a real problem is surfaced honestly
// instead of masquerading as "no data".
func openGranolaCache() (*granola.Cache, error) {
	c := &granola.Cache{}
	// Documents (show, notes-show, memo, export, tiptap) from the meetings
	// table, with people + calendar reconstructed from attendees / calendar_event_id.
	if err := backfillDocumentsFromStore(c); err != nil {
		return nil, fmt.Errorf("backfill documents from store: %w", err)
	}
	// meetingsMetadata (talktime, duplicates, export, extract, meetings) so
	// those readers get real attendee/calendar data instead of nil.
	if err := backfillMeetingsMetadataFromStore(c); err != nil {
		return nil, fmt.Errorf("backfill meetings metadata from store: %w", err)
	}
	// Transcripts (transcript get, talktime, preflight, collect, memo/export
	// transcript stream) from the REST-fed transcript_segments table.
	if err := backfillTranscriptsFromStore(c); err != nil {
		return nil, fmt.Errorf("backfill transcripts from store: %w", err)
	}
	return c, nil
}

// backfillTranscriptsFromStore populates c.Transcripts from the
// transcript_segments table (REST-fed by `enrich`). Segment timestamps are
// stored as Unix millis; the in-memory TranscriptSegment carries ISO strings,
// so we convert back. Quietly no-ops when the store is absent or the cache
// already carries transcripts (pre-encryption Granola / test fixtures).
func backfillTranscriptsFromStore(c *granola.Cache) error {
	if c == nil {
		return nil
	}
	if c.Transcripts == nil {
		c.Transcripts = map[string][]granola.TranscriptSegment{}
	}
	if len(c.Transcripts) > 0 {
		return nil
	}
	ctx := context.Background()
	s, err := openGranolaStoreRead(ctx)
	if err != nil || s == nil {
		return err
	}
	defer s.Close()
	rows, err := s.DB().QueryContext(ctx, `
		SELECT meeting_id, source, text, start_ts_ms, end_ts_ms, confidence
		FROM transcript_segments ORDER BY meeting_id, idx`)
	if err != nil {
		return fmt.Errorf("backfill: query transcript_segments: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var mid, source, text string
		var startMs, endMs sql.NullInt64
		var conf sql.NullFloat64
		if err := rows.Scan(&mid, &source, &text, &startMs, &endMs, &conf); err != nil {
			return fmt.Errorf("backfill: scan segment: %w", err)
		}
		c.Transcripts[mid] = append(c.Transcripts[mid], granola.TranscriptSegment{
			Source:         source,
			Text:           text,
			StartTimestamp: millisToISO(startMs),
			EndTimestamp:   millisToISO(endMs),
			Confidence:     conf.Float64,
		})
	}
	return rows.Err()
}

// millisToISO renders stored Unix-millis back to an RFC3339 string (UTC), or ""
// for a null/zero timestamp. ParseISO round-trips it for the talk-time math.
func millisToISO(ms sql.NullInt64) string {
	if !ms.Valid || ms.Int64 == 0 {
		return ""
	}
	return time.UnixMilli(ms.Int64).UTC().Format(time.RFC3339Nano)
}

// backfillDocumentsFromStore reads rows from the meetings table that
// sync populated from the public REST API and reconstructs Document structs
// into c.Documents. It also rebuilds each doc's people block (from the
// attendees table) and its google_calendar_event (id from
// meetings.calendar_event_id, start/end from the stored meeting window) so
// the readers that consult those fields (export/extract metadata, meetings
// JSON, calendar, duplicates) see real data. Quietly returns nil if the
// store does not exist yet (fresh install before first sync).
//
// Every nullable text column is scanned through sql.NullString so a partially
// populated row can never crash the scan (converting NULL to string) nor be
// silently dropped.
func backfillDocumentsFromStore(c *granola.Cache) error {
	if c == nil {
		return nil
	}
	if c.Documents == nil {
		c.Documents = map[string]granola.Document{}
	}
	if len(c.Documents) > 0 {
		// Cache already has documents (test fixtures seeded directly);
		// don't shadow them with potentially stale store rows.
		return nil
	}
	ctx := context.Background()
	s, err := openGranolaStoreRead(ctx)
	if err != nil || s == nil {
		return err
	}
	defer s.Close()

	// Load attendees grouped by meeting first (drains + closes its own rows)
	// so we can attach doc.people as we scan meetings.
	people, err := attendeesByMeeting(ctx, s)
	if err != nil {
		return err
	}

	rows, err := s.DB().QueryContext(ctx, `
		SELECT id, title, created_at, updated_at, started_at, ended_at,
		       workspace_id, calendar_event_id, deleted_at, notes_markdown,
		       notes_plain, creation_source, valid_meeting
		FROM meetings
	`)
	if err != nil {
		return fmt.Errorf("backfill: query meetings: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var d granola.Document
		var title, createdAt, updatedAt sql.NullString
		var startedAt, endedAt, workspaceID sql.NullString
		var calID, deletedAt, notesMD, notesPlain, creationSrc sql.NullString
		var validMeeting sql.NullInt64
		if err := rows.Scan(
			&d.ID, &title, &createdAt, &updatedAt,
			&startedAt, &endedAt,
			&workspaceID, &calID, &deletedAt,
			&notesMD, &notesPlain,
			&creationSrc, &validMeeting,
		); err != nil {
			return fmt.Errorf("backfill: scan meeting: %w", err)
		}
		d.Title = title.String
		d.CreatedAt = createdAt.String
		d.UpdatedAt = updatedAt.String
		d.WorkspaceID = workspaceID.String
		d.NotesMarkdown = notesMD.String
		d.NotesPlain = notesPlain.String
		d.CreationSource = creationSrc.String
		d.ValidMeeting = validMeeting.Valid && validMeeting.Int64 != 0
		if deletedAt.Valid && deletedAt.String != "" {
			da := deletedAt.String
			d.DeletedAt = &da
		}
		if ppl := people[d.ID]; len(ppl) > 0 {
			d.People = &granola.DocPeople{Attendees: ppl}
		}
		// Reconstruct the calendar event when the store carries a calendar id.
		// Start/End use Granola's {"dateTime": ...} shape so docWindow's
		// extractCalTimeRaw reads the real meeting window from the store.
		if calID.Valid && calID.String != "" {
			ev := &granola.GoogleCalendarEvent{ID: calID.String}
			if startedAt.Valid && startedAt.String != "" {
				ev.Start = calEventTimeRaw(startedAt.String)
			}
			if endedAt.Valid && endedAt.String != "" {
				ev.End = calEventTimeRaw(endedAt.String)
			}
			d.GoogleCalendarEvent = ev
		}
		c.Documents[d.ID] = d
	}
	return rows.Err()
}

// attendeesByMeeting loads the attendees table grouped by meeting_id into
// DocPerson records (the shape doc.people.attendees uses). Nullable columns
// are read through sql.NullString.
func attendeesByMeeting(ctx context.Context, s *store.Store) (map[string][]granola.DocPerson, error) {
	rows, err := s.DB().QueryContext(ctx, `
		SELECT meeting_id, email, name, response_status
		FROM attendees ORDER BY meeting_id`)
	if err != nil {
		return nil, fmt.Errorf("backfill: query attendees: %w", err)
	}
	defer rows.Close()
	out := map[string][]granola.DocPerson{}
	for rows.Next() {
		var mid string
		var email, name, resp sql.NullString
		if err := rows.Scan(&mid, &email, &name, &resp); err != nil {
			return nil, fmt.Errorf("backfill: scan attendee: %w", err)
		}
		out[mid] = append(out[mid], granola.DocPerson{
			Name:           name.String,
			Email:          email.String,
			ResponseStatus: resp.String,
		})
	}
	return out, rows.Err()
}

// calEventTimeRaw renders an ISO timestamp into the {"dateTime": ...} object
// Granola uses for google_calendar_event.start/end, so the readers'
// extractCalTimeRaw helper reads it back unchanged. Returns nil on the
// (unreachable) marshal error rather than a malformed blob.
func calEventTimeRaw(iso string) json.RawMessage {
	b, err := json.Marshal(map[string]string{"dateTime": iso})
	if err != nil {
		return nil
	}
	return json.RawMessage(b)
}

// backfillMeetingsMetadataFromStore populates c.MeetingsMetadata from the
// meetings + attendees tables so readers that consult meetingsMetadata
// (talktime, duplicates, export, extract, meetings) get real attendee data
// instead of nil. The public REST feed exposes no meeting "creator" and no
// conferencing URL, so MeetingMetadata.Creator/Conferencing/URL are left
// zero rather than fabricated. Quietly returns nil when the store is absent.
func backfillMeetingsMetadataFromStore(c *granola.Cache) error {
	if c == nil {
		return nil
	}
	if c.MeetingsMetadata == nil {
		c.MeetingsMetadata = map[string]granola.MeetingMetadata{}
	}
	if len(c.MeetingsMetadata) > 0 {
		return nil
	}
	ctx := context.Background()
	s, err := openGranolaStoreRead(ctx)
	if err != nil || s == nil {
		return err
	}
	defer s.Close()
	rows, err := s.DB().QueryContext(ctx, `
		SELECT a.meeting_id, a.email, a.name, a.response_status
		FROM attendees a
		JOIN meetings m ON m.id = a.meeting_id
		ORDER BY a.meeting_id`)
	if err != nil {
		return fmt.Errorf("backfill: query meetings metadata: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var mid string
		var email, name, resp sql.NullString
		if err := rows.Scan(&mid, &email, &name, &resp); err != nil {
			return fmt.Errorf("backfill: scan meeting metadata: %w", err)
		}
		md := c.MeetingsMetadata[mid]
		md.Attendees = append(md.Attendees, granola.CalendarInvitee{
			Name:           name.String,
			Email:          email.String,
			ResponseStatus: resp.String,
		})
		c.MeetingsMetadata[mid] = md
	}
	return rows.Err()
}

// isDogfoodEnv reports whether the CLI runs under `cli-printing-press dogfood`
// (PRINTING_PRESS_DOGFOOD=1). This printed CLI predates cliutil.IsDogfoodEnv, so
// it reads the env var directly. Network-heavy and interactive novel commands
// curtail work under it to fit the dogfood runner's per-command timeout.
func isDogfoodEnv() bool { return os.Getenv("PRINTING_PRESS_DOGFOOD") == "1" }

// granolaActiveFlags is the running command's root flag set, captured by the
// novel-command hook. Hand-wired (handfixes.json: rest-helpers-honor-config):
// the leaf REST helpers loaded config.Load(""), so `--config other.toml`
// was ignored and they used the default host and credentials.
var granolaActiveFlags *rootFlags

// granolaRESTConfig loads the config the running command was pointed at.
func granolaRESTConfig() (*config.Config, error) {
	path := ""
	if granolaActiveFlags != nil {
		path = granolaActiveFlags.configPath
	}
	return config.Load(path)
}

// granolaStoreOverride, when non-empty, redirects the local SQLite store path.
// Set by `sync --db <path>` so the whole sync (framework notes/folders +
// enrich) targets one store. Empty = the platform data dir.
// ponytail: process-scoped global (one CLI invocation = one command); a
// threaded param would churn every openGranolaStore caller for no gain.
var granolaStoreOverride string

// granolaStorePath resolves the store path, honoring a `--db` override.
func granolaStorePath() string {
	if granolaStoreOverride != "" {
		return granolaStoreOverride
	}
	return defaultDBPath("granola-cli")
}

// File-safety modes for MEMO-pipeline and store output. Meeting exports and
// the local SQLite store can contain private notes, transcripts, and attendee
// emails, so directories are owner-only 0700 and files owner-only 0600.
const (
	safeDirPerm  os.FileMode = 0o700
	safeFilePerm os.FileMode = 0o600
)

// validNoteIDFilename matches an opaque Granola note/meeting id that is safe to
// embed in a filename: letters, digits, underscore, hyphen only. It excludes
// path separators and dots, so "..", ".", and dotted traversal are all
// rejected. Real Granola ids (e.g. not_06Yq6JtogRihEr, UUIDs) satisfy it.
var validNoteIDFilename = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// validateNoteIDForFilename rejects an id that is empty or contains anything
// but [A-Za-z0-9_-] — path separators, "..", NUL, spaces — so it can never
// escape the output directory when interpolated into a filename.
func validateNoteIDForFilename(id string) error {
	if !validNoteIDFilename.MatchString(id) {
		return usageErr(fmt.Errorf("invalid meeting id %q: only letters, digits, '-', and '_' are allowed", id))
	}
	return nil
}

// ensureSafeDir creates dir (and parents) with 0700 perms and tightens an
// already-existing dir to 0700 — os.MkdirAll leaves an existing looser dir's
// mode untouched, so the explicit Chmod is load-bearing.
func ensureSafeDir(dir string) error {
	if err := os.MkdirAll(dir, safeDirPerm); err != nil {
		return err
	}
	return os.Chmod(dir, safeDirPerm)
}

// writeSafeFile writes data to path with 0600 perms. Every filesystem call
// goes through an os.Root opened on the file's own directory, so the write can
// never land outside that directory even if a path component is swapped for a
// symlink between the check and the open. On top of that, a pre-existing
// SYMLINK at path is REFUSED outright (Lstat does not follow it): writing
// through one would drop the sensitive bytes - and then chmod them - wherever
// the link points. It opens O_WRONLY|O_CREATE|O_TRUNC (not O_EXCL, so a
// legitimate re-export overwrites) and chmods explicitly afterward, because the
// perm argument only applies when the file is CREATED - a pre-existing regular
// file (possibly world-readable) keeps its old mode without the Chmod.
func writeSafeFile(path string, data []byte) error {
	// A directory-form path ("out/" or an existing directory) is refused before
	// it is split: filepath.Dir/Base drop a trailing separator, so "out/" would
	// otherwise become the file "out/out" instead of failing.
	if path == "" || strings.HasSuffix(path, string(os.PathSeparator)) || strings.HasSuffix(path, "/") {
		return ioErr(fmt.Errorf("output path %q names a directory, not a file", path))
	}
	r, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		return ioErr(err)
	}
	defer r.Close()
	name := filepath.Base(path)
	if fi, err := r.Lstat(name); err == nil {
		if fi.Mode()&os.ModeSymlink != 0 {
			return ioErr(fmt.Errorf("refusing to write through pre-existing symlink %q", path))
		}
		if fi.IsDir() {
			return ioErr(fmt.Errorf("output path %q is an existing directory, not a file", path))
		}
	} else if !os.IsNotExist(err) {
		return ioErr(err)
	}
	f, err := r.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, safeFilePerm)
	if err != nil {
		return ioErr(err)
	}
	// Tighten the handle BEFORE any private byte lands: a pre-existing 0644
	// file keeps its mode through O_TRUNC, so writing first left the new
	// contents world-readable until the chmod (handfixes.json:
	// memo-output-file-safety).
	if cerr := f.Chmod(safeFilePerm); cerr != nil {
		_ = f.Close()
		return ioErr(cerr)
	}
	if _, werr := f.Write(data); werr != nil {
		_ = f.Close()
		return ioErr(werr)
	}
	if cerr := f.Close(); cerr != nil {
		return ioErr(cerr)
	}
	return r.Chmod(name, safeFilePerm)
}

// safeJoinContained joins name onto dir and guarantees the result stays within
// dir. name must be a single path element (no separators, no "." / ".."); dir
// is resolved with filepath.Abs + filepath.EvalSymlinks (so a symlinked output
// dir is compared by its real location), and the final path must have the
// resolved dir as a prefix with a separator. Callers must create dir (via
// ensureSafeDir) before calling so EvalSymlinks resolves it.
func safeJoinContained(dir, name string) (string, error) {
	if name == "" || name == "." || name == ".." ||
		strings.ContainsAny(name, `/\`) || strings.ContainsRune(name, 0) {
		return "", usageErr(fmt.Errorf("invalid output filename %q", name))
	}
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return "", ioErr(err)
	}
	if resolved, rerr := filepath.EvalSymlinks(absDir); rerr == nil {
		absDir = resolved
	}
	final := filepath.Join(absDir, name)
	if final != absDir && !strings.HasPrefix(final, absDir+string(os.PathSeparator)) {
		return "", usageErr(fmt.Errorf("refusing to write %q outside %s", name, absDir))
	}
	// Re-resolve the FINAL path's parent, not just the dir passed in. Because
	// name is a single element its parent is absDir (already EvalSymlinks-
	// resolved above), but resolving it again defensively confirms the parent
	// still lands inside absDir — a parent swapped for a symlink escaping the
	// output dir is rejected here rather than followed at write time.
	parent := filepath.Dir(final)
	if resolvedParent, perr := filepath.EvalSymlinks(parent); perr == nil {
		if resolvedParent != absDir && !strings.HasPrefix(resolvedParent, absDir+string(os.PathSeparator)) {
			return "", usageErr(fmt.Errorf("refusing to write %q: its parent %s escapes %s", name, resolvedParent, absDir))
		}
	}
	return final, nil
}

// openGranolaStore opens (or creates) the SQLite store and ensures the
// granola-specific schema is in place.
func openGranolaStore(ctx context.Context) (*store.Store, error) {
	dbPath := granolaStorePath()
	// Create the data dir 0700 and tighten an existing looser dir; the store's
	// own writes are already 0600/0700, this secures the hand-created parent.
	if err := ensureSafeDir(filepath.Dir(dbPath)); err != nil {
		return nil, fmt.Errorf("creating data dir: %w", err)
	}
	s, err := store.OpenWithContext(ctx, dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening local store: %w", err)
	}
	if err := granola.EnsureSchema(ctx, s.DB()); err != nil {
		s.Close()
		return nil, err
	}
	return s, nil
}

// openGranolaStoreRead opens the store for reading; returns (nil, nil)
// if the database hasn't been created yet so the caller can emit a
// helpful "run sync first" message.
func openGranolaStoreRead(ctx context.Context) (*store.Store, error) {
	dbPath := granolaStorePath()
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return nil, nil
	}
	// The DB exists; tighten its containing dir to 0700 on the read path too
	// (openGranolaStore already does this on the write path). The dir holds
	// private notes/transcripts/attendee emails — a loose mode from an older
	// version or a permissive umask is corrected here rather than trusted.
	if err := ensureSafeDir(filepath.Dir(dbPath)); err != nil {
		return nil, fmt.Errorf("tightening data dir: %w", err)
	}
	s, err := store.OpenWithContext(ctx, dbPath)
	if err != nil {
		return nil, err
	}
	if err := granola.EnsureSchema(ctx, s.DB()); err != nil {
		s.Close()
		return nil, err
	}
	return s, nil
}

// emitJSON writes v to cmd's stdout as JSON, honoring --compact and
// --select via printJSONFiltered.
func emitJSON(cmd *cobra.Command, flags *rootFlags, v any) error {
	return printJSONFiltered(cmd.OutOrStdout(), v, flags)
}

// emitNDJSON writes each item on its own line.
func emitNDJSON(cmd *cobra.Command, items []any) error {
	w := cmd.OutOrStdout()
	for _, it := range items {
		b, err := json.Marshal(it)
		if err != nil {
			return err
		}
		if _, err := w.Write(append(b, '\n')); err != nil {
			return err
		}
	}
	return nil
}

// emitNDJSONLine writes one ndjson line.
func emitNDJSONLine(w io.Writer, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = w.Write(append(b, '\n'))
	return err
}

// parseTimeWindow translates --last 7d / --since DATE / --until DATE
// into an absolute [from, to] pair. Either end may be zero-valued.
// --since accepts both absolute dates ("2026-05-01") and relative
// durations ("7d", "24h") — relative durations are subtracted from now.
func parseTimeWindow(last, since, until string) (from, to time.Time, err error) {
	now := time.Now()
	if last != "" {
		d, perr := parseDurationLoose(last)
		if perr != nil {
			err = fmt.Errorf("invalid --last %q: %w", last, perr)
			return
		}
		from = now.Add(-d)
		to = now
		return
	}
	if since != "" {
		from, err = parseSinceOrDate(since, now)
		if err != nil {
			err = fmt.Errorf("invalid --since %q: %w", since, err)
			return
		}
	}
	if until != "" {
		to, err = parseSinceOrDate(until, now)
		if err != nil {
			err = fmt.Errorf("invalid --until %q: %w", until, err)
			return
		}
	}
	return
}

// parseSinceOrDate tries a relative duration first (suffixes d/w/h/m/s),
// then falls back to an absolute date.
func parseSinceOrDate(s string, now time.Time) (time.Time, error) {
	if d, err := parseDurationLoose(s); err == nil {
		return now.Add(-d), nil
	}
	return parseAnyDate(s)
}

// timeNow is a wall-clock indirection used by commands so tests can swap
// the clock. Defaults to time.Now.
var timeNow = time.Now

// parseDurationLoose accepts "7d", "30d", "3h", and standard Go durations.
func parseDurationLoose(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty duration")
	}
	// "Nd" -> N*24h
	if strings.HasSuffix(s, "d") {
		var n int
		if _, err := fmt.Sscanf(s, "%dd", &n); err == nil {
			return time.Duration(n) * 24 * time.Hour, nil
		}
	}
	// "Nw" -> N*7d
	if strings.HasSuffix(s, "w") {
		var n int
		if _, err := fmt.Sscanf(s, "%dw", &n); err == nil {
			return time.Duration(n) * 7 * 24 * time.Hour, nil
		}
	}
	return time.ParseDuration(s)
}

func parseAnyDate(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05Z", "2006-01-02 15:04", "2006-01-02", "01/02/2006"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	if d, err := parseDurationLoose(s); err == nil {
		return time.Now().Add(-d), nil
	}
	return time.Time{}, fmt.Errorf("unrecognized date %q", s)
}

// withinWindow returns true if t is inside [from, to] when those are set.
// Zero from/to are unbounded.
func withinWindow(t time.Time, from, to time.Time) bool {
	if t.IsZero() {
		return false
	}
	if !from.IsZero() && t.Before(from) {
		return false
	}
	if !to.IsZero() && t.After(to) {
		return false
	}
	return true
}

// stderr writes a one-line user-visible note to stderr (warnings only).
func stderr(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
}

// Ensure sql import is referenced even when no .go file under cli/ uses
// it directly; we re-export from this package.
var _ = sql.Open
