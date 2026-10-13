// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// REST-backed hydrator for the rich SQLite schema (meetings / attendees /
// transcript_segments). On Granola v7.4x+ the encrypted desktop store and
// internal API are sealed, so the rich tables can no longer be filled from the
// cache. This fills them from the PUBLIC REST API detail endpoint
// (/v1/notes/{id}?include=transcript), keyed by the REST not_ id.
//
// Human notes are NOT sourced here — the public API omits them; the official
// MCP path (get_meetings) supplies those separately.

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"granola-pp-cli/internal/client"
	"granola-pp-cli/internal/granola"
)

// restAttendee is one attendee/invitee. Calendar invitees carry only an email.
type restAttendee struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

// restSegment is one transcript line with its speaker source (mic vs system).
type restSegment struct {
	Text      string `json:"text"`
	StartTime string `json:"start_time"`
	EndTime   string `json:"end_time"`
	Speaker   struct {
		Source string `json:"source"`
	} `json:"speaker"`
}

// restCalendarEvent is the /v1/notes/{id} calendar_event object. scheduled_*
// are the REAL meeting bounds; created_at/updated_at are note edit times, not
// meeting times, and must never be used as duration.
type restCalendarEvent struct {
	CalendarEventID    string         `json:"calendar_event_id"`
	EventTitle         string         `json:"event_title"`
	Organiser          string         `json:"organiser"`
	ScheduledStartTime string         `json:"scheduled_start_time"`
	ScheduledEndTime   string         `json:"scheduled_end_time"`
	Invitees           []restAttendee `json:"invitees"`
}

// restNoteDetail is the /v1/notes/{id}?include=transcript response shape
// (verified live 2026-07-18). Attendees and Transcript are POINTERS so we can
// distinguish "field absent/null" (nil → leave existing rows untouched) from
// "field present but empty" (non-nil [] → authoritatively clear). This is the
// guard against a partial HTTP 200 deleting valid local attendees/segments.
type restNoteDetail struct {
	ID            string          `json:"id"`
	Title         string          `json:"title"`
	CreatedAt     string          `json:"created_at"`
	UpdatedAt     string          `json:"updated_at"`
	SummaryText   string          `json:"summary_text"`
	SummaryMD     string          `json:"summary_markdown"`
	Attendees     *[]restAttendee `json:"attendees"`
	Transcript    *[]restSegment  `json:"transcript"`
	CalendarEvent json.RawMessage `json:"calendar_event"`
}

// recentNoteIDs returns up to limit note ids from the REST-fed notes table,
// newest first. limit <= 0 returns all of them.
func recentNoteIDs(ctx context.Context, db *sql.DB, limit int) ([]string, error) {
	q := `SELECT id FROM notes WHERE id LIKE 'not_%' ORDER BY julianday(updated_at) DESC LIMIT ?`
	// LIMIT is a bound parameter, never spliced into the SQL text. SQLite
	// reads a negative LIMIT as "no limit", which is what limit <= 0 means here.
	lim := limit
	if lim <= 0 {
		lim = -1
	}
	rows, err := db.QueryContext(ctx, q, lim)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// hydrateRichFromREST fetches detail for each id and upserts the rich tables,
// all keyed by the REST not_ id, then best-effort enriches human/private notes
// via the official MCP (when connected). Returns (meetings written, notes set).
func hydrateRichFromREST(ctx context.Context, db *sql.DB, c *client.Client, ids []string) (int, int, error) {
	if err := granola.EnsureSchema(ctx, db); err != nil {
		return 0, 0, err
	}
	written := 0
	// FTS is maintained by triggers inside each row's own transaction
	// (handfixes.json: fts-maintained-by-triggers), so a pass that fails part
	// way still leaves every written row searchable.
	//
	// One note that cannot be fetched (deleted on the server, a 5xx, a bad
	// response) must not stop the rest: it is recorded in hydrate_failures,
	// the pass moves on, and the first error is returned at the end
	// (handfixes.json: hydrate-continues-past-failures). A database error
	// still stops the pass. A --dry-run client prints each request and
	// writes nothing.
	var targets []restTarget
	var firstErr error
	fail := func(id string, err error) {
		recordHydrateFailure(ctx, db, id)
		if firstErr == nil {
			firstErr = err
		}
	}
	for _, id := range ids {
		if ctx.Err() != nil {
			if firstErr == nil {
				firstErr = ctx.Err()
			}
			break
		}
		// Escape the opaque note id as a single path segment (it is not a path).
		raw, err := c.Get(ctx, "/v1/notes/"+url.PathEscape(id), map[string]string{"include": "transcript"})
		if c.DryRun {
			continue
		}
		if err != nil {
			if ctx.Err() != nil {
				break
			}
			// Account-level failures (network down, key refused, rate
			// limited) say nothing about this note and would fail every
			// remaining fetch: stop without recording, so fixing the cause
			// is not followed by a day of skipped notes.
			var apiE *client.APIError
			if !errors.As(err, &apiE) || apiE.StatusCode == 401 || apiE.StatusCode == 403 || apiE.StatusCode == 429 {
				if firstErr == nil {
					firstErr = fmt.Errorf("fetch note %s: %w", id, err)
				}
				break
			}
			fail(id, fmt.Errorf("fetch note %s: %w", id, err))
			continue
		}
		var d restNoteDetail
		if err := json.Unmarshal(raw, &d); err != nil {
			fail(id, fmt.Errorf("parse note %s: %w", id, err))
			continue
		}
		// Reject a response whose id doesn't match the one requested: writing it
		// would attribute one meeting's attendees/transcript to another id.
		if d.ID != "" && d.ID != id {
			fail(id, fmt.Errorf("note %s: response id mismatch (got %q)", id, d.ID))
			continue
		}
		if d.ID == "" {
			d.ID = id
		}
		if err := upsertRichMeeting(ctx, db, &d); err != nil {
			return written, 0, err
		}
		_, _ = db.ExecContext(ctx, `DELETE FROM hydrate_failures WHERE note_id = ?`, id)
		written++
		targets = append(targets, effectiveRESTTarget(ctx, db, &d))
	}
	if c.DryRun {
		return 0, 0, nil
	}
	// Best-effort human/private notes via MCP. Not connected/expired → notes
	// stay empty (commands hint); never fails the sync.
	notesSet, nerr := enrichHumanNotes(ctx, db, targets)
	if nerr != nil && len(targets) > 0 {
		// MCP is optional/additive, so this never fails the sync — but surface
		// it so the user knows private/human notes were skipped (rows are
		// otherwise silently notes-less in notes-show/export/MEMO).
		stderr("note: private/human notes not enriched (Granola MCP unavailable): %v — run 'granola-cli mcp-auth login'", nerr)
	}
	return written, notesSet, firstErr
}

// recordHydrateFailure notes that a detail fetch failed, so auto-refresh puts
// the note behind every other candidate and retries it after a day instead of
// spending its whole budget on notes that keep failing.
func recordHydrateFailure(ctx context.Context, db *sql.DB, id string) {
	_, _ = db.ExecContext(ctx, `INSERT INTO hydrate_failures(note_id, failed_at) VALUES (?, ?)
		ON CONFLICT(note_id) DO UPDATE SET failed_at = excluded.failed_at`, id, time.Now().UTC().Format(time.RFC3339))
}

// rfcToTime parses an RFC3339 timestamp, returning the zero time on failure.
func rfcToTime(s string) time.Time {
	for _, l := range []string{time.RFC3339Nano, time.RFC3339} {
		if t, err := time.Parse(l, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

// upsertRichMeeting writes one REST note detail into meetings + attendees +
// transcript_segments in a single transaction. It is PRESENCE-AWARE and
// non-destructive: attendee/segment rows are replaced only when the response
// actually carried that field, so a partial 200 (field absent) never converts
// "not provided" into "authoritatively empty" and deletes valid local rows.
// Meeting bounds come from calendar_event.scheduled_* (the real meeting times)
// or, failing that, the transcript span — never fabricated from note
// created/updated times.
func upsertRichMeeting(ctx context.Context, db *sql.DB, d *restNoteDetail) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	// Parse calendar_event (present iff non-empty raw that decodes).
	var cal restCalendarEvent
	calPresent := false
	if len(d.CalendarEvent) > 0 && strings.TrimSpace(string(d.CalendarEvent)) != "null" {
		if err := json.Unmarshal(d.CalendarEvent, &cal); err == nil {
			calPresent = true
		}
	}

	// Real meeting bounds: calendar scheduled_* first, else transcript span,
	// else leave existing untouched. started_at and ended_at are tracked
	// INDEPENDENTLY so a calendar_event that carries only scheduled_start_time
	// (no scheduled_end_time) never blanks a previously known ended_at, and
	// vice-versa. Note edit times are NEVER used as meeting bounds.
	startedAt, endedAt := "", ""
	startedPresent, endedPresent := false, false
	if calPresent {
		if cal.ScheduledStartTime != "" {
			startedAt = cal.ScheduledStartTime
			startedPresent = true
		}
		if cal.ScheduledEndTime != "" {
			endedAt = cal.ScheduledEndTime
			endedPresent = true
		}
	}
	// Transcript span fills only the bound(s) the calendar did not supply.
	if d.Transcript != nil && len(*d.Transcript) > 0 {
		segs := *d.Transcript
		if !startedPresent && segs[0].StartTime != "" {
			startedAt = segs[0].StartTime
			startedPresent = true
		}
		if !endedPresent && segs[len(segs)-1].EndTime != "" {
			endedAt = segs[len(segs)-1].EndTime
			endedPresent = true
		}
	}

	// Provenance per bound (handfixes.json: window-provenance): a calendar
	// bound always wins; a transcript span may only fill a bound that is
	// empty or was itself transcript-derived. A partial response that omits
	// calendar_event must not shrink a stored scheduled window to the
	// transcript span, while a transcript that grew may still extend one.
	startedSrc, endedSrc := "", ""
	if startedPresent {
		startedSrc = "transcript"
		if calPresent && cal.ScheduledStartTime != "" {
			startedSrc = "calendar"
		}
	}
	if endedPresent {
		endedSrc = "transcript"
		if calPresent && cal.ScheduledEndTime != "" {
			endedSrc = "calendar"
		}
	}

	// calendar_event_id is guarded on the sub-field itself: overwrite only when
	// this response actually carried a non-empty id, else keep the stored one.
	calendarEventID := ""
	calendarIDPresent := false
	if calPresent && cal.CalendarEventID != "" {
		calendarEventID = cal.CalendarEventID
		calendarIDPresent = true
	}

	transcriptPresent := d.Transcript != nil
	transcriptAvail := 0
	if transcriptPresent && len(*d.Transcript) > 0 {
		transcriptAvail = 1
	}

	b2i := func(b bool) int {
		if b {
			return 1
		}
		return 0
	}

	// notes_markdown/notes_plain/deleted_at are OMITTED from the UPDATE set so
	// re-enrichment preserves MCP-filled human notes. title/created_at/updated_at
	// overwrite only when this response carried a non-empty value (a partial 200
	// that omits them must not blank a stored title/timestamps). started_at/
	// ended_at/calendar_event_id/transcript_available update only when this
	// response actually carried that specific sub-field (presence flags), else
	// keep the existing value.
	// Stored as UTC "Z" text so string comparison and ordering stay
	// chronological (handfixes.json: meeting-times-stored-utc).
	startedAt, endedAt = utcRFC3339(startedAt), utcRFC3339(endedAt)
	createdAt, updatedAt := utcRFC3339(d.CreatedAt), utcRFC3339(d.UpdatedAt)
	if _, err := tx.ExecContext(ctx, `INSERT INTO meetings(
		id, title, created_at, updated_at, started_at, ended_at, workspace_id,
		calendar_event_id, deleted_at, notes_markdown, notes_plain,
		transcript_available, recipes_applied, creation_source, valid_meeting,
		source_updated_at, started_src, ended_src
	) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
	ON CONFLICT(id) DO UPDATE SET
		title=CASE WHEN excluded.title != '' THEN excluded.title ELSE meetings.title END,
		created_at=CASE WHEN excluded.created_at != '' THEN excluded.created_at ELSE meetings.created_at END,
		updated_at=CASE WHEN excluded.updated_at != '' THEN excluded.updated_at ELSE meetings.updated_at END,
		source_updated_at=CASE WHEN excluded.source_updated_at != '' THEN excluded.source_updated_at ELSE meetings.source_updated_at END,
		started_at=CASE WHEN excluded.started_src = 'calendar' THEN excluded.started_at
			WHEN excluded.started_src = 'transcript' AND (COALESCE(meetings.started_at, '') = '' OR meetings.started_src = 'transcript') THEN excluded.started_at
			ELSE meetings.started_at END,
		started_src=CASE WHEN excluded.started_src = 'calendar' THEN 'calendar'
			WHEN excluded.started_src = 'transcript' AND (COALESCE(meetings.started_at, '') = '' OR meetings.started_src = 'transcript') THEN 'transcript'
			ELSE meetings.started_src END,
		ended_at=CASE WHEN excluded.ended_src = 'calendar' THEN excluded.ended_at
			WHEN excluded.ended_src = 'transcript' AND (COALESCE(meetings.ended_at, '') = '' OR meetings.ended_src = 'transcript') THEN excluded.ended_at
			ELSE meetings.ended_at END,
		ended_src=CASE WHEN excluded.ended_src = 'calendar' THEN 'calendar'
			WHEN excluded.ended_src = 'transcript' AND (COALESCE(meetings.ended_at, '') = '' OR meetings.ended_src = 'transcript') THEN 'transcript'
			ELSE meetings.ended_src END,
		calendar_event_id=CASE WHEN ?=1 THEN excluded.calendar_event_id ELSE meetings.calendar_event_id END,
		transcript_available=CASE WHEN ?=1 THEN excluded.transcript_available ELSE meetings.transcript_available END,
		recipes_applied=excluded.recipes_applied, creation_source=excluded.creation_source,
		valid_meeting=excluded.valid_meeting`,
		// VALUES
		d.ID, d.Title, createdAt, updatedAt, startedAt, endedAt, "",
		calendarEventID, "", "", "", transcriptAvail, "[]", "api", 1,
		freshnessWatermark(d.UpdatedAt), startedSrc, endedSrc,
		// CASE presence flags (calendar_id, transcript). The bounds are guarded
		// by their own provenance (started_src / ended_src) above, each
		// independently, so a half-populated calendar_event can't blank the
		// other bound.
		b2i(calendarIDPresent), b2i(transcriptPresent),
	); err != nil {
		return fmt.Errorf("upsert meeting %s: %w", d.ID, err)
	}

	// Attendees: merge note attendees + calendar invitees. Rows are touched
	// only when at least one source was present, else the stored set is kept.
	attendeesPresent := d.Attendees != nil || (calPresent && len(cal.Invitees) > 0)
	if attendeesPresent {
		merged := map[string]string{} // email -> best-known name
		order := []string{}
		add := func(email, name string) {
			email = strings.ToLower(strings.TrimSpace(email))
			if email == "" {
				return
			}
			if existing, ok := merged[email]; ok {
				if existing == "" && name != "" {
					merged[email] = name
				}
				return
			}
			merged[email] = name
			order = append(order, email)
		}
		if d.Attendees != nil {
			for _, a := range *d.Attendees {
				add(a.Email, a.Name)
			}
		}
		if calPresent {
			for _, inv := range cal.Invitees {
				add(inv.Email, inv.Name)
			}
		}
		// Rows are merged, never replaced wholesale (handfixes.json:
		// rest-hydrate-presence-aware). The store does not record which source
		// (note attendees or calendar invitees) contributed a row, so a
		// response carrying only one source must not delete rows the other
		// one added: invitees-only must keep the attendees, and attendees-only
		// must keep the invitees. The one authoritative clear is an explicit,
		// present-but-empty `attendees: []`. An empty name never overwrites a
		// known one.
		if d.Attendees != nil && len(*d.Attendees) == 0 {
			if _, err := tx.ExecContext(ctx, `DELETE FROM attendees WHERE meeting_id = ?`, d.ID); err != nil {
				return err
			}
		}
		for _, email := range order {
			if _, err := tx.ExecContext(ctx, `INSERT INTO attendees(meeting_id,email,name,response_status) VALUES (?,?,?,?)
				ON CONFLICT(meeting_id, email) DO UPDATE SET
					name=CASE WHEN excluded.name != '' THEN excluded.name ELSE attendees.name END`,
				d.ID, email, merged[email], ""); err != nil {
				return fmt.Errorf("upsert attendee %s/%s: %w", d.ID, email, err)
			}
		}
	}

	// AI summary: stored so local exports carry it; an empty value never
	// overwrites a stored one (a partial 200 must not erase it).
	if strings.TrimSpace(d.SummaryMD) != "" || strings.TrimSpace(d.SummaryText) != "" {
		if _, err := tx.ExecContext(ctx, `INSERT INTO meeting_summaries(meeting_id, summary_markdown, summary_text) VALUES (?,?,?)
			ON CONFLICT(meeting_id) DO UPDATE SET
				summary_markdown=CASE WHEN excluded.summary_markdown != '' THEN excluded.summary_markdown ELSE meeting_summaries.summary_markdown END,
				summary_text=CASE WHEN excluded.summary_text != '' THEN excluded.summary_text ELSE meeting_summaries.summary_text END`,
			d.ID, d.SummaryMD, d.SummaryText); err != nil {
			return fmt.Errorf("upsert summary %s: %w", d.ID, err)
		}
	}

	// Transcript segments: replace only when the response carried the field.
	if d.Transcript != nil {
		if _, err := tx.ExecContext(ctx, `DELETE FROM transcript_segments WHERE meeting_id = ?`, d.ID); err != nil {
			return err
		}
		for i, seg := range *d.Transcript {
			startMs := rfcToMillis(seg.StartTime)
			endMs := rfcToMillis(seg.EndTime)
			if _, err := tx.ExecContext(ctx, `INSERT INTO transcript_segments(meeting_id,idx,source,text,start_ts_ms,end_ts_ms,confidence) VALUES (?,?,?,?,?,?,?)`,
				d.ID, i, seg.Speaker.Source, seg.Text, startMs, endMs, 0.0); err != nil {
				return fmt.Errorf("upsert segment %s/%d: %w", d.ID, i, err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	committed = true
	return nil
}

// rfcToMillis parses an RFC3339 timestamp (with or without sub-second) to Unix
// milliseconds, returning 0 on failure.
func rfcToMillis(s string) int64 {
	if s == "" {
		return 0
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UnixMilli()
		}
	}
	return 0
}

// runRESTFallbackSync is the v7.4x sync path: the desktop cache is sealed, so
// populate the store from the public REST API (notes/folders via the generated
// sync command) and then enrich the rich tables from the detail endpoint.
// enrichLimit bounds the per-note detail fetch (0 = all).
// enrichRecent hydrates the rich tables (meetings/attendees/transcript_segments)
// for the most-recent notes from the REST detail endpoint, plus best-effort
// human/private notes via the official MCP. enrichLimit 0 = the whole library.
// Returns (meetings enriched, human-notes set, notes requested, error). Assumes
// the thin notes/folders sync already ran (the caller drives the framework sync).
func enrichRecent(ctx context.Context, flags *rootFlags, enrichLimit int) (int, int, int, error) {
	s, err := openGranolaStore(ctx)
	if err != nil {
		return 0, 0, 0, err
	}
	defer s.Close()
	ids, err := recentNoteIDs(ctx, s.DB(), enrichLimit)
	if err != nil {
		return 0, 0, 0, err
	}
	c, err := flags.newClient()
	if err != nil {
		return 0, 0, 0, err
	}
	n, notes, err := hydrateRichFromREST(ctx, s.DB(), c, ids)
	return n, notes, len(ids), err
}

// newEnrichCmd fills the rich tables from the REST detail endpoint. Bounded by
// --limit (default 50 most-recent) unless --full.
func newEnrichCmd(flags *rootFlags) *cobra.Command {
	var limit int
	var full bool
	cmd := &cobra.Command{
		Use:   "enrich",
		Short: "Populate meetings/transcripts/attendees from the REST detail endpoint",
		Long: `Fetches /v1/notes/{id}?include=transcript for recent notes and fills the
local meetings, attendees, and transcript_segments tables. Needed on Granola
v7.4x+ where the desktop cache is sealed. Run after 'sync'; use --full for the
whole library.

Note: a pre-v7.4x cache sync may have left older UUID-keyed meetings in the
store, which can appear alongside the REST-keyed ones. For a clean REST-only
store, delete the local database and re-sync:
  rm ~/.local/share/granola-cli/data.db && granola-cli sync --full`,
		Example: strings.Trim(`
  granola-cli enrich
  granola-cli enrich --limit 100
  granola-cli enrich --full`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "false"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			s, err := openGranolaStore(ctx)
			if err != nil {
				return err
			}
			defer s.Close()
			lim := limit
			if full {
				lim = 0
			}
			if isDogfoodEnv() {
				lim = 2 // bound per-note detail fetches to fit the dogfood timeout
			}
			ids, err := recentNoteIDs(ctx, s.DB(), lim)
			if err != nil {
				return err
			}
			if len(ids) == 0 {
				return fmt.Errorf("no synced notes to enrich — run 'granola-cli sync' first")
			}
			n, notes, err := hydrateRichFromREST(ctx, s.DB(), c, ids)
			if err != nil {
				return apiErr(fmt.Errorf("enriched %d/%d before error: %w", n, len(ids), err))
			}
			return emitJSON(cmd, flags, map[string]any{
				"event": "enrich_summary", "enriched": n, "requested": len(ids),
				"human_notes": notes,
			})
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 50, "Enrich the N most-recent notes (0 = all)")
	cmd.Flags().BoolVar(&full, "full", false, "Enrich the entire library (overrides --limit)")
	return cmd
}

// effectiveRESTTarget builds the private-notes match target from the
// EFFECTIVE stored title and created_at (handfixes.json:
// mcp-notes-backfill-tracked): a partial response may omit them, and the
// upsert kept the stored values, so matching on the response alone could miss
// a meeting MCP does know.
func effectiveRESTTarget(ctx context.Context, db *sql.DB, d *restNoteDetail) restTarget {
	title, created := d.Title, d.CreatedAt
	var st, sc string
	if db.QueryRowContext(ctx, `SELECT COALESCE(title,''), COALESCE(created_at,'') FROM meetings WHERE id = ?`, d.ID).Scan(&st, &sc) == nil {
		if st != "" {
			title = st
		}
		if sc != "" {
			created = sc
		}
	}
	return restTarget{ID: d.ID, Title: title, Created: rfcToTime(created)}
}

// utcRFC3339 rewrites an RFC 3339 timestamp in UTC "Z" form so stored meeting
// times compare and sort correctly as text. Empty or unparseable input is
// returned unchanged.
func utcRFC3339(s string) string {
	if s == "" {
		return s
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return s
	}
	return t.UTC().Format(time.RFC3339)
}

// freshnessWatermark is what meetings.source_updated_at records for a
// hydrated detail: the API's updated_at exactly as sent (full precision), or,
// when the response omits it, the hydration instant - any revision after that
// is newer. Never empty, so a hydrated row is never re-queued as "no
// watermark" (handfixes.json: meeting-times-stored-utc).
func freshnessWatermark(apiUpdatedAt string) string {
	if apiUpdatedAt != "" {
		return apiUpdatedAt
	}
	return time.Now().UTC().Format(time.RFC3339Nano)
}
