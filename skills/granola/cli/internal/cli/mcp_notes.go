// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// Human/private-notes enrichment via Granola's official MCP. The public REST
// API omits the user's raw typed notes; the MCP get_meetings tool returns them
// in a <private_notes> block. REST not_ ids and MCP UUIDs are disjoint id
// spaces, so meetings are joined by normalized title + closest date.
//
// This runs during `enrich` (best-effort, only when MCP is connected) and
// writes the notes into meetings.notes_markdown/notes_plain, so every
// human-notes command reads them through the normal store→cache backfill.

import (
	"context"
	"database/sql"
	"fmt"
	"html"
	"regexp"
	"sort"
	"strings"
	"time"
)

type mcpMeeting struct {
	UUID  string
	Title string
	Date  time.Time
}

// restTarget is a REST-synced meeting we want human notes for.
type restTarget struct {
	ID      string // not_ id
	Title   string
	Created time.Time
}

var (
	mcpMeetingTagRe = regexp.MustCompile(`<meeting id="([^"]+)" title="([^"]*)" date="([^"]*)">`)
	mcpBlockRe      = regexp.MustCompile(`(?s)<meeting id="([^"]+)"[^>]*>(.*?)</meeting>`)
	mcpPrivateRe    = regexp.MustCompile(`(?s)<private_notes>(.*?)</private_notes>`)
)

func normTitle(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(html.UnescapeString(s))), " ")
}

// parseMCPMeetings extracts (uuid, title, date) tuples from a list_meetings
// response.
func parseMCPMeetings(text string) []mcpMeeting {
	var out []mcpMeeting
	for _, m := range mcpMeetingTagRe.FindAllStringSubmatch(text, -1) {
		out = append(out, mcpMeeting{
			UUID:  m[1],
			Title: html.UnescapeString(m[2]),
			Date:  parseMCPDate(m[3]),
		})
	}
	return out
}

// usZoneOffsetMinutes maps the US zone abbreviations Granola emits to real UTC
// offsets. Go's time.Parse("MST") would otherwise assign a fabricated ZERO
// offset, skewing the date-delta match by the true offset (e.g. 4h for EDT) and
// risking a wrong same-title match.
var usZoneOffsetMinutes = map[string]int{
	"UTC": 0, "GMT": 0,
	"EDT": -4 * 60, "EST": -5 * 60, "CDT": -5 * 60, "CST": -6 * 60,
	"MDT": -6 * 60, "MST": -7 * 60, "PDT": -7 * 60, "PST": -8 * 60,
	"AKDT": -8 * 60, "AKST": -9 * 60, "HST": -10 * 60,
}

// parseMCPDate parses "Jul 16, 2026 2:09 PM EDT" into a correct absolute time by
// resolving the trailing zone abbreviation to its real offset. Returns the zero
// time when it can't parse (the caller then declines the match — never guesses).
func parseMCPDate(s string) time.Time {
	s = strings.TrimSpace(s)
	if fields := strings.Fields(s); len(fields) > 0 {
		zone := fields[len(fields)-1]
		if off, ok := usZoneOffsetMinutes[zone]; ok {
			body := strings.TrimSpace(strings.TrimSuffix(s, zone))
			loc := time.FixedZone(zone, off*60)
			for _, layout := range []string{"Jan 2, 2006 3:04 PM", "Jan 2, 2006"} {
				if t, err := time.ParseInLocation(layout, body, loc); err == nil {
					return t
				}
			}
		}
	}
	// No/unknown zone: parse what we can (treated as UTC).
	for _, layout := range []string{"Jan 2, 2006 3:04 PM", "Jan 2, 2006"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

// parsePrivateNotes maps each meeting UUID to its <private_notes> text.
func parsePrivateNotes(text string) map[string]string {
	out := map[string]string{}
	for _, b := range mcpBlockRe.FindAllStringSubmatch(text, -1) {
		uuid := b[1]
		if pn := mcpPrivateRe.FindStringSubmatch(b[2]); pn != nil {
			note := strings.TrimSpace(html.UnescapeString(pn[1]))
			if note != "" {
				out[uuid] = note
			}
		}
	}
	return out
}

// matchMeetingDateWindow bounds how far an MCP meeting's date may be from a REST
// meeting's created_at to be considered the same meeting. Writing the WRONG
// meeting's private notes is worse than writing none, so EVERY match — unique
// title or not — must land inside this window. Granola stamps a note's
// created_at at meeting time, so a true match is minutes apart; 12h is generous
// slack while still cleanly rejecting a different day's same-title instance.
const matchMeetingDateWindow = 12 * time.Hour

// matchAmbiguityGap: when a REST meeting has two same-title MCP candidates whose
// date-deltas differ by LESS than this, we can't tell them apart — decline
// rather than guess. A true recurring match is minutes from one instance and
// hours/days from the next, so the gap is large; two candidates within 2h of
// each other are genuinely ambiguous.
const matchAmbiguityGap = 2 * time.Hour

// assignMeetingUUIDs resolves REST meetings (not_ ids) to MCP meeting UUIDs by
// normalized title + a date within matchMeetingDateWindow. It is deliberately
// conservative — writing the WRONG meeting's private notes is worse than writing
// none:
//   - a target with NO title/date, or no in-window same-title candidate, is skipped;
//   - a target whose two closest same-title candidates are within matchAmbiguityGap
//     is skipped (MCP-side ambiguity: too close to tell which meeting to read);
//   - an MCP meeting whose two closest REST targets are within matchAmbiguityGap is
//     declined for BOTH (REST-side ambiguity: a repeated title / delayed note means
//     we can't tell WHICH meeting the private notes belong to — writing to the
//     closest would risk leaking one meeting's notes onto another);
//   - each remaining target offers only its single closest candidate, and UUIDs are
//     assigned closest-delta first, one-to-one, so one MCP meeting's notes are never
//     written under several REST meetings.
func assignMeetingUUIDs(byTitle map[string][]mcpMeeting, targets []restTarget) map[string]string {
	type cand struct {
		targetIdx int
		uuid      string
		delta     time.Duration
	}
	var cands []cand
	for i, t := range targets {
		key := normTitle(t.Title)
		if key == "" || t.Created.IsZero() {
			continue
		}
		type opt struct {
			uuid  string
			delta time.Duration
		}
		var opts []opt
		for _, m := range byTitle[key] {
			if m.Date.IsZero() {
				continue
			}
			d := m.Date.Sub(t.Created)
			if d < 0 {
				d = -d
			}
			if d <= matchMeetingDateWindow {
				opts = append(opts, opt{m.UUID, d})
			}
		}
		if len(opts) == 0 {
			continue
		}
		sort.Slice(opts, func(a, b int) bool { return opts[a].delta < opts[b].delta })
		if len(opts) >= 2 && opts[1].delta-opts[0].delta <= matchAmbiguityGap {
			continue // two candidates within the gap — too close to tell apart, decline
		}
		cands = append(cands, cand{i, opts[0].uuid, opts[0].delta})
	}
	// REST-side ambiguity: an MCP meeting (uuid) that is the closest candidate
	// for MULTIPLE same-title REST targets whose deltas are within the ambiguity
	// gap is unattributable — writing its private notes to the closest REST
	// meeting would risk the WRONG meeting. Decline the uuid entirely (mirror of
	// the MCP-side check above). A clearly separated pair (gap > matchAmbiguityGap)
	// is still resolvable: the closest target legitimately wins below.
	deltasByUUID := map[string][]time.Duration{}
	for _, c := range cands {
		deltasByUUID[c.uuid] = append(deltasByUUID[c.uuid], c.delta)
	}
	ambiguousUUID := map[string]bool{}
	for uuid, deltas := range deltasByUUID {
		if len(deltas) < 2 {
			continue
		}
		sort.Slice(deltas, func(a, b int) bool { return deltas[a] < deltas[b] })
		if deltas[1]-deltas[0] <= matchAmbiguityGap {
			ambiguousUUID[uuid] = true
		}
	}
	// Assign closest-delta first, each UUID at most once (targets are already
	// unique — one entry per target above), skipping any REST-side-ambiguous uuid.
	sort.SliceStable(cands, func(a, b int) bool { return cands[a].delta < cands[b].delta })
	usedUUID := map[string]bool{}
	out := map[string]string{}
	for _, c := range cands {
		if ambiguousUUID[c.uuid] || usedUUID[c.uuid] {
			continue
		}
		usedUUID[c.uuid] = true
		out[targets[c.targetIdx].ID] = c.uuid
	}
	return out
}

// enrichHumanNotes fetches private notes for the given REST meetings via MCP and
// writes them into meetings.notes_markdown/notes_plain. Best-effort: returns
// (0, errMCPNotConnected) when MCP isn't connected so the caller can hint
// without failing the sync. Returns the count of meetings whose notes were set.
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
	listText, isErr := mcpToolCall(access, sid, "list_meetings", map[string]any{})
	if isErr {
		return 0, errMCPNotConnected
	}
	byTitle := map[string][]mcpMeeting{}
	for _, m := range parseMCPMeetings(listText) {
		key := normTitle(m.Title)
		if key == "" {
			continue
		}
		byTitle[key] = append(byTitle[key], m)
	}

	// Resolve targets to UUIDs as a one-to-one, date-windowed assignment.
	// Hand-wired (handfixes.json: mcp-notes-attribution-all-candidates): the
	// assignment must see EVERY stored meeting that shares a target's title,
	// not just this batch. Otherwise, with REST meetings A (09:00) and B
	// (10:00) titled alike and only B in the batch, the MCP copy of A had no
	// competitor and its private notes were written under B.
	candidates, ok := withSameTitleStoredMeetings(ctx, db, targets)
	if !ok {
		return 0, fmt.Errorf("could not list same-title meetings to attribute private notes safely")
	}
	uuidByRest := assignMeetingUUIDs(byTitle, candidates)
	if len(uuidByRest) == 0 {
		markHumanNotesChecked(ctx, db, targets)
		return 0, nil
	}
	uuids := make([]string, 0, len(uuidByRest))
	for _, u := range uuidByRest {
		uuids = append(uuids, u)
	}

	// Batch get_meetings (10 UUIDs per call) and collect private notes.
	notesByUUID := map[string]string{}
	answered := map[string]bool{} // uuids whose batch call succeeded
	for i := 0; i < len(uuids); i += 10 {
		end := i + 10
		if end > len(uuids) {
			end = len(uuids)
		}
		txt, isErr := mcpToolCall(access, sid, "get_meetings", map[string]any{"meeting_ids": uuids[i:end]})
		if isErr {
			continue
		}
		for _, u := range uuids[i:end] {
			answered[u] = true
		}
		for k, v := range parsePrivateNotes(txt) {
			notesByUUID[k] = v
		}
	}
	// Record which targets MCP has now answered for: those whose batch
	// succeeded, and those MCP does not list at all (nothing to fetch). A
	// target whose batch failed stays unrecorded and is retried.
	checkedAt := time.Now().UTC().Format(time.RFC3339)
	for _, t := range targets {
		u, resolved := uuidByRest[t.ID]
		if resolved && !answered[u] {
			continue
		}
		_, _ = db.ExecContext(ctx, `INSERT INTO human_notes_state(meeting_id, checked_at) VALUES (?, ?)
			ON CONFLICT(meeting_id) DO UPDATE SET checked_at = excluded.checked_at`, t.ID, checkedAt)
	}

	// Write notes into the store.
	n := 0
	for _, t := range targets {
		note := notesByUUID[uuidByRest[t.ID]]
		if note == "" {
			continue
		}
		if _, err := db.ExecContext(ctx,
			`UPDATE meetings SET notes_markdown = ?, notes_plain = ? WHERE id = ?`,
			note, note, t.ID); err != nil {
			return n, err
		}
		n++
	}
	if n > 0 {
		_, _ = db.ExecContext(ctx, `INSERT INTO meetings_fts(meetings_fts) VALUES ('rebuild')`)
	}
	return n, nil
}

// withSameTitleStoredMeetings returns targets plus every other meeting this
// store knows - hydrated (meetings) or only listed by sync (notes) - whose
// normalized title matches one of theirs, deduplicated by REST id, so the
// UUID assignment weighs all plausible owners of a private note. Only targets
// are ever written to. ok is false when the candidates could not be listed;
// the caller then writes nothing rather than guess.
func withSameTitleStoredMeetings(ctx context.Context, db *sql.DB, targets []restTarget) ([]restTarget, bool) {
	titles := map[string]bool{}
	seen := map[string]bool{}
	for _, t := range targets {
		if k := normTitle(t.Title); k != "" {
			titles[k] = true
		}
		seen[t.ID] = true
	}
	out := append([]restTarget(nil), targets...)
	if len(titles) == 0 {
		return out, true
	}
	for _, q := range []string{
		`SELECT id, COALESCE(title,''), COALESCE(created_at,'') FROM meetings`,
		`SELECT id, COALESCE(title,''), COALESCE(CAST(created_at AS TEXT),'') FROM notes WHERE id LIKE 'not_%'`,
	} {
		rows, err := db.QueryContext(ctx, q)
		if err != nil {
			if strings.Contains(err.Error(), "no such table") {
				continue
			}
			return nil, false
		}
		for rows.Next() {
			var id, title, created string
			if rows.Scan(&id, &title, &created) != nil || seen[id] {
				continue
			}
			if titles[normTitle(title)] {
				seen[id] = true
				out = append(out, restTarget{ID: id, Title: title, Created: rfcToTime(created)})
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, false
		}
	}
	return out, true
}

// markHumanNotesChecked records that MCP was asked about these meetings and
// listed none of them, so the backfill does not retry them on every command.
func markHumanNotesChecked(ctx context.Context, db *sql.DB, targets []restTarget) {
	checkedAt := time.Now().UTC().Format(time.RFC3339)
	for _, t := range targets {
		_, _ = db.ExecContext(ctx, `INSERT INTO human_notes_state(meeting_id, checked_at) VALUES (?, ?)
			ON CONFLICT(meeting_id) DO UPDATE SET checked_at = excluded.checked_at`, t.ID, checkedAt)
	}
}

// pendingHumanNoteTargets returns up to limit hydrated meetings (newest first)
// that Granola's MCP has never answered for: meetings hydrated before MCP was
// connected, or whose MCP pass failed.
func pendingHumanNoteTargets(ctx context.Context, db *sql.DB, limit int) ([]restTarget, error) {
	lim := limit
	if lim <= 0 {
		lim = -1
	}
	rows, err := db.QueryContext(ctx, `SELECT m.id, COALESCE(m.title,''), COALESCE(m.created_at,'') FROM meetings m
		LEFT JOIN human_notes_state h ON h.meeting_id = m.id
		WHERE h.meeting_id IS NULL AND m.id LIKE 'not_%'
		ORDER BY m.created_at DESC
		LIMIT ?`, lim)
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
