// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// Open P1s from the 2026-10-24 factory Codex review round 2.

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"hubspot-pp-cli/internal/cliutil/testenv"
	"hubspot-pp-cli/internal/store"
)

// [P1 timestamps] the cutoff keeps fractional seconds.
func TestR2SinceCutoffKeepsFractionalSeconds(t *testing.T) {
	testenv.Isolate(t)
	s, path := r1Store(t)
	r1Upsert(t, s.UpsertHubspotDealsCrm, map[string]any{"id": "1", "properties": map[string]any{"dealname": "A"}, "updatedAt": "2026-10-24T14:00:00.500Z"})
	s.Close()
	out, err := r1Run(t, "", "since", "2026-10-24T14:00:00.900Z", "--types", "deals", "--db", path, "--json")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, `"1"`) {
		t.Fatalf("a deal updated before the cutoff is included: %s", out)
	}
}

// [P1 timestamps] the cross-type sort is chronological.
func TestR2SinceSortsChronologically(t *testing.T) {
	testenv.Isolate(t)
	s, path := r1Store(t)
	r1Upsert(t, s.UpsertHubspotDealsCrm, map[string]any{"id": "1", "properties": map[string]any{"dealname": "A"}, "updatedAt": "2026-10-24T14:00:00Z"})
	r1Upsert(t, s.UpsertHubspotContactsCrm, map[string]any{"id": "2", "properties": map[string]any{"firstname": "B"}, "updatedAt": "2026-10-24T14:00:00.500Z"})
	s.Close()
	out, err := r1Run(t, "", "since", "2026-10-24T13:00:00Z", "--types", "deals,contacts", "--limit", "1", "--db", path, "--json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"2"`) {
		t.Fatalf("the newest change (contact 2) is not first: %s", out)
	}
}

// r2Before fails the test unless want appears in out before other.
func r2Before(t *testing.T, out, want, other string) {
	t.Helper()
	i, j := strings.Index(out, want), strings.Index(out, other)
	if i < 0 || j < 0 || i > j {
		t.Fatalf("want %s before %s: %s", want, other, out)
	}
}

// r2History inserts one history row with its timestamp stored verbatim as
// text, the shape HubSpot's ValueWithTimestamp.timestamp arrives in. The
// production writer stores a bound time.Time as the driver's time.Time text
// instead; a mirror can hold both.
func r2History(t *testing.T, s *store.Store, objType, objID, prop, value, ts string) {
	t.Helper()
	if _, err := s.DB().Exec(`INSERT INTO hubspot_property_history (object_type, object_id, property, value, timestamp, source, snapshot_at) VALUES (?, ?, ?, ?, ?, 'CRM_UI', ?)`, objType, objID, prop, value, ts, time.Now().UTC().Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
}

func r2WriterHistory(t *testing.T, s *store.Store, objType, objID, prop, value string, ts time.Time) {
	t.Helper()
	if err := s.UpsertPropertyHistoryBatch(context.Background(), objType, objID, []store.PropertyHistoryEntry{{Property: prop, Value: value, Timestamp: ts, Source: "CRM_UI", SnapshotAt: time.Now().UTC().Format(time.RFC3339)}}); err != nil {
		t.Fatal(err)
	}
}

// [P1 timestamps] engagements of (local): the timeline is newest first.
func TestR2EngagementsOfLocalSortsChronologically(t *testing.T) {
	testenv.Isolate(t)
	s, path := r1Store(t)
	createAssociations(t, s)
	r1Upsert(t, s.UpsertHubspotCallsCrm, map[string]any{"id": "101", "properties": map[string]any{"hs_call_title": "Older"}, "createdAt": "2026-10-01T00:00:00Z", "updatedAt": "2026-10-24T14:00:00Z"})
	r1Upsert(t, s.UpsertHubspotCallsCrm, map[string]any{"id": "102", "properties": map[string]any{"hs_call_title": "Newer"}, "createdAt": "2026-10-01T00:00:00Z", "updatedAt": "2026-10-24T14:00:00.500Z"})
	seedAssociation(t, s, "calls", "101", "contacts", "9")
	seedAssociation(t, s, "calls", "102", "contacts", "9")
	s.Close()
	out, err := r1Run(t, "", "engagements", "of", "contact:9", "--type", "calls", "--db", path, "--json")
	if err != nil {
		t.Fatal(err)
	}
	r2Before(t, out, `"102"`, `"101"`)
}

// [P1 timestamps] engagements of (live): --since keeps a call updated
// 500ms after the cutoff.
func TestR2EngagementsOfLiveCutoffIsChronological(t *testing.T) {
	testenv.Isolate(t)
	newR1Server(t, 200, `{"results":[{"toObjectId":"501"}],"id":"501","createdAt":"2026-10-01T00:00:00Z","updatedAt":"2026-10-24T14:00:00.500Z","properties":{"hs_call_title":"Kickoff"}}`)
	_, path := r1Store(t)
	out, err := r1Run(t, "", "engagements", "of", "contact:9", "--type", "calls", "--since", "2026-10-24T14:00:00Z", "--db", path, "--json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"501"`) || !strings.Contains(out, `"live"`) {
		t.Fatalf("a call updated after the cutoff is dropped: %s", out)
	}
}

// [P1 timestamps] engagements of (live): a timestamp that cannot be read is
// never counted as newer than the cutoff.
func TestR2EngagementsOfLiveUnknownTimestampIsNotNewer(t *testing.T) {
	testenv.Isolate(t)
	newR1Server(t, 200, `{"results":[{"toObjectId":"502"}],"id":"502","createdAt":"","updatedAt":"","properties":{"hs_call_title":"Mystery"}}`)
	_, path := r1Store(t)
	out, err := r1Run(t, "", "engagements", "of", "contact:9", "--type", "calls", "--since", "30d", "--db", path, "--json")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, `"502"`) {
		t.Fatalf("an engagement with no readable timestamp passed --since: %s", out)
	}
}

// [P1 timestamps] notes signals: last_note_at is the latest note.
func TestR2NotesSignalsLastNoteAtIsChronological(t *testing.T) {
	testenv.Isolate(t)
	s, path := r1Store(t)
	createAssociations(t, s)
	r1Deal(t, s, "1", map[string]any{"dealname": "Acme", "dealstage": "qualifiedtobuy"})
	r1Upsert(t, s.UpsertHubspotNotesCrm, map[string]any{"id": "201", "properties": map[string]any{"hs_note_body": "budget approved"}, "createdAt": "2026-10-01T00:00:00Z", "updatedAt": "2026-10-24T14:00:00.500Z"})
	r1Upsert(t, s.UpsertHubspotNotesCrm, map[string]any{"id": "202", "properties": map[string]any{"hs_note_body": "budget approved"}, "createdAt": "2026-10-01T00:00:00Z", "updatedAt": "2026-10-24T14:00:00Z"})
	seedAssociation(t, s, "notes", "201", "deals", "1")
	seedAssociation(t, s, "notes", "202", "deals", "1")
	s.Close()
	out, err := r1Run(t, "", "notes", "signals", "--db", path, "--json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"last_note_at": "2026-10-24T14:00:00.500Z"`) {
		t.Fatalf("last_note_at is not the latest note: %s", out)
	}
}

// [P1 timestamps] nurture-mine (and nurture queue, which scores on it):
// latest_deal_stage is the most recently updated open deal.
func TestR2NurtureMineLatestDealIsChronological(t *testing.T) {
	testenv.Isolate(t)
	s, path := r1Store(t)
	createAssociations(t, s)
	r1Upsert(t, s.UpsertHubspotContactsCrm, map[string]any{"id": "9", "archived": false, "properties": map[string]any{"firstname": "Pat", "notes_last_contacted": "2026-01-01T00:00:00Z"}, "createdAt": "2025-01-01T00:00:00Z", "updatedAt": "2026-01-01T00:00:00Z"})
	r1Upsert(t, s.UpsertHubspotDealsCrm, map[string]any{"id": "1", "archived": false, "properties": map[string]any{"dealstage": "appointmentscheduled"}, "createdAt": "2026-01-01T00:00:00Z", "updatedAt": "2026-10-24T14:00:00Z"})
	r1Upsert(t, s.UpsertHubspotDealsCrm, map[string]any{"id": "2", "archived": false, "properties": map[string]any{"dealstage": "qualifiedtobuy"}, "createdAt": "2026-01-01T00:00:00Z", "updatedAt": "2026-10-24T14:00:00.500Z"})
	seedAssociation(t, s, "contacts", "9", "deals", "1")
	seedAssociation(t, s, "contacts", "9", "deals", "2")
	s.Close()
	out, err := r1Run(t, "", "nurture-mine", "--owner", "", "--stale-days", "14", "--db", path, "--json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"latest_deal_stage": "qualifiedtobuy"`) {
		t.Fatalf("latest_deal_stage is not the most recently updated deal: %s", out)
	}
}

// [P1 timestamps] meetings history: newest first across both storage shapes,
// and the writer's own shape still orders (bare julianday() reads it as NULL).
func TestR2MeetingsHistoryNewestFirst(t *testing.T) {
	testenv.Isolate(t)
	s, path := r1Store(t)
	r2WriterHistory(t, s, "meetings", "m1", "hs_meeting_outcome", "COMPLETED", time.Date(2026, 10, 24, 14, 0, 0, 500e6, time.UTC))
	r2WriterHistory(t, s, "meetings", "m1", "hs_meeting_outcome", "RESCHEDULED", time.Date(2026, 10, 24, 14, 0, 0, 0, time.UTC))
	r2History(t, s, "meetings", "m1", "hs_meeting_outcome", "SCHEDULED", "2026-10-24T13:00:00Z")
	s.Close()
	out, err := r1Run(t, "", "meetings", "history", "m1", "--db", path, "--json")
	if err != nil {
		t.Fatal(err)
	}
	r2Before(t, out, `"COMPLETED"`, `"RESCHEDULED"`)
	r2Before(t, out, `"RESCHEDULED"`, `"SCHEDULED"`)
}

// [P1 timestamps] meetings ever-had / status-report: the window and the
// first hit are chronological, not lexical.
func TestR2MeetingsEverHadWindowIsChronological(t *testing.T) {
	testenv.Isolate(t)
	s, path := r1Store(t)
	// 08:00 on May 1 is before --from (12:00 on May 1) and must not count.
	r2History(t, s, "meetings", "m1", "hs_meeting_outcome", "SCHEDULED", "2026-05-01T08:00:00Z")
	r2WriterHistory(t, s, "meetings", "m1", "hs_meeting_outcome", "SCHEDULED", time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC))
	r2WriterHistory(t, s, "meetings", "m2", "hs_meeting_outcome", "SCHEDULED", time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC))
	s.Close()
	out, err := r1Run(t, "", "meetings", "ever-had", "--property", "hs_meeting_outcome", "--value", "SCHEDULED", "--from", "2026-05-01T12:00:00Z", "--to", "2026-05-31", "--db", path, "--json")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "2026-05-01T08") {
		t.Fatalf("a hit before --from counted as the first hit: %s", out)
	}
	if !strings.Contains(out, "2026-05-10 12:00:00") {
		t.Fatalf("m1's first in-window hit is missing: %s", out)
	}
	r2Before(t, out, `"m2"`, `"m1"`)
}

// [P1 timestamps] deals velocity: the current stage's entry is the latest
// transition by time, not the largest string.
func TestR2DealsVelocityLatestTransitionIsChronological(t *testing.T) {
	testenv.Isolate(t)
	s, path := r1Store(t)
	r1Pipeline(t, s)
	r1Deal(t, s, "1", map[string]any{"dealname": "Acme", "dealstage": "qualifiedtobuy", "pipeline": "default", "hs_lastmodifieddate": "2026-10-14T08:00:00Z"})
	r2History(t, s, "deals", "1", "dealstage", "appointmentscheduled", "2026-10-14T06:00:00Z")
	r2WriterHistory(t, s, "deals", "1", "dealstage", "qualifiedtobuy", time.Date(2026, 10, 14, 8, 0, 0, 0, time.UTC))
	r1Deal(t, s, "2", map[string]any{"dealname": "Beta", "dealstage": "qualifiedtobuy", "pipeline": "default", "hs_lastmodifieddate": "2026-10-14T08:00:00Z"})
	r2WriterHistory(t, s, "deals", "2", "dealstage", "qualifiedtobuy", time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC))
	s.Close()
	out, err := r1Run(t, "", "deals", "velocity", "--db", path, "--json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"entered_stage_at": "2026-10-14 08:00:00`) {
		t.Fatalf("deal 1 entered its stage at the earlier transition: %s", out)
	}
	if !strings.Contains(out, `"entered_stage_at": "2026-09-01 00:00:00`) {
		t.Fatalf("a deal with only writer-shaped history is missing: %s", out)
	}
}

// The Go compare and the SQL compare agree on every shape, and an unknown
// value is never newer than anything.
func TestR2TimeCompareGoMatchesSQL(t *testing.T) {
	s, _ := r1Store(t)
	vals := []string{
		"2026-10-24T14:00:00Z", "2026-10-24T14:00:00.500Z", "2026-10-24T14:00:00.5Z",
		"2026-10-24T14:00:00.45Z", "2026-10-24T16:00:00.25+02:00",
		"2026-10-24 14:00:00 +0000 UTC", "2026-10-24 14:00:00.5 +0000 UTC",
		"2026-10-24 10:00:00.7 -0400 EDT", "2026-10-24 14:00:00.9+00:00",
		"2026-10-24", "", "garbage", "now", "2461338",
	}
	for _, a := range vals {
		for _, b := range vals {
			var sqlAfter sql.NullBool
			if err := s.DB().QueryRow(`SELECT `+hsJulianSQL("?1")+` > `+hsJulianSQL("?2"), a, b).Scan(&sqlAfter); err != nil {
				t.Fatal(err)
			}
			if got, want := hsTimeAfter(a, b), sqlAfter.Valid && sqlAfter.Bool; got != want {
				t.Errorf("hsTimeAfter(%q, %q) = %v, SQL says %v", a, b, got, want)
			}
		}
		if _, ok := parseHubSpotTime(a); !ok && hsNewerFirst(a, "1970-01-01T00:00:00Z") {
			t.Errorf("unknown %q sorts before a real timestamp", a)
		}
	}
	if !hsNewerFirst("2026-10-24T14:00:00.500Z", "2026-10-24T14:00:00Z") || hsNewerFirst("2026-10-24T14:00:00Z", "2026-10-24T14:00:00.500Z") {
		t.Fatal("hsNewerFirst is not chronological")
	}
}
