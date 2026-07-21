// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

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
//     is skipped (too ambiguous to attribute);
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
	// Assign closest-delta first, each UUID at most once (targets are already
	// unique — one entry per target above).
	sort.SliceStable(cands, func(a, b int) bool { return cands[a].delta < cands[b].delta })
	usedUUID := map[string]bool{}
	out := map[string]string{}
	for _, c := range cands {
		if usedUUID[c.uuid] {
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
	uuidByRest := assignMeetingUUIDs(byTitle, targets)
	if len(uuidByRest) == 0 {
		return 0, nil
	}
	uuids := make([]string, 0, len(uuidByRest))
	for _, u := range uuidByRest {
		uuids = append(uuids, u)
	}

	// Batch get_meetings (10 UUIDs per call) and collect private notes.
	notesByUUID := map[string]string{}
	for i := 0; i < len(uuids); i += 10 {
		end := i + 10
		if end > len(uuids) {
			end = len(uuids)
		}
		txt, isErr := mcpToolCall(access, sid, "get_meetings", map[string]any{"meeting_ids": uuids[i:end]})
		if isErr {
			continue
		}
		for k, v := range parsePrivateNotes(txt) {
			notesByUUID[k] = v
		}
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
