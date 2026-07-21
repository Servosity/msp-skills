// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"testing"
	"time"
)

func TestParseMCPMeetingsAndPrivateNotes(t *testing.T) {
	list := `<meetings_data>
<meeting id="0b9a63e7-468f-4950-a9f6-dabea60de3e7" title="Team Planning Part 2" date="Jul 16, 2026 2:09 PM EDT">
  <known_participants>x</known_participants>
</meeting>
<meeting id="446e2cdc-5b41-4106-a77f-6a59666dc993" title="Live Build Session" date="Jul 16, 2026 1:00 PM EDT">
</meeting>
</meetings_data>`
	ms := parseMCPMeetings(list)
	if len(ms) != 2 {
		t.Fatalf("expected 2 meetings, got %d", len(ms))
	}
	if ms[0].UUID != "0b9a63e7-468f-4950-a9f6-dabea60de3e7" || ms[0].Title != "Team Planning Part 2" {
		t.Fatalf("bad parse: %+v", ms[0])
	}
	if ms[0].Date.IsZero() {
		t.Fatal("date failed to parse")
	}

	gm := `<meeting id="0b9a63e7-468f-4950-a9f6-dabea60de3e7" title="X" date="Y">
  <private_notes>the human notes</private_notes>
  <summary>the ai summary</summary>
</meeting>`
	pn := parsePrivateNotes(gm)
	if pn["0b9a63e7-468f-4950-a9f6-dabea60de3e7"] != "the human notes" {
		t.Fatalf("private notes extraction wrong: %q", pn["0b9a63e7-468f-4950-a9f6-dabea60de3e7"])
	}
	// The summary must NOT leak into the private_notes value.
	if pn["0b9a63e7-468f-4950-a9f6-dabea60de3e7"] == gm {
		t.Fatal("extracted the whole block instead of just private_notes")
	}
}

func TestAssignMeetingUUIDs(t *testing.T) {
	base := time.Date(2026, 7, 16, 14, 0, 0, 0, time.UTC)
	byTitle := map[string][]mcpMeeting{
		normTitle("Unique Title"): {{UUID: "u1", Title: "Unique Title", Date: base}},
		normTitle("Weekly Sync"): {
			{UUID: "w1", Title: "Weekly Sync", Date: base},
			{UUID: "w2", Title: "Weekly Sync", Date: base.AddDate(0, 0, 7)}, // a week later
		},
	}

	// Unique title within window → matches; unique title OUTSIDE the window
	// must NOT match (a coincidental title with an implausible date).
	got := assignMeetingUUIDs(byTitle, []restTarget{
		{ID: "not_a", Title: "Unique Title", Created: base.Add(1 * time.Hour)},
		{ID: "not_b", Title: "Unique Title", Created: base.AddDate(0, 0, 30)},
	})
	if got["not_a"] != "u1" {
		t.Fatalf("in-window unique should match u1, got %q", got["not_a"])
	}
	if _, ok := got["not_b"]; ok {
		t.Fatalf("out-of-window unique must NOT match, got %q", got["not_b"])
	}

	// Recurring: each REST meeting claims its date-closest UUID, one-to-one.
	got = assignMeetingUUIDs(byTitle, []restTarget{
		{ID: "not_x", Title: "Weekly Sync", Created: base.Add(2 * time.Hour)},              // near w1
		{ID: "not_y", Title: "Weekly Sync", Created: base.AddDate(0, 0, 7).Add(time.Hour)}, // near w2
	})
	if got["not_x"] != "w1" || got["not_y"] != "w2" {
		t.Fatalf("one-to-one recurring assignment wrong: %+v", got)
	}

	// Two REST meetings, one plausible MCP meeting → only the closest wins the
	// UUID; the other gets nothing (never double-assign the same notes).
	got = assignMeetingUUIDs(byTitle, []restTarget{
		{ID: "not_close", Title: "Unique Title", Created: base.Add(30 * time.Minute)},
		{ID: "not_far", Title: "Unique Title", Created: base.Add(3 * time.Hour)},
	})
	if got["not_close"] != "u1" {
		t.Fatalf("closest should win u1, got %q", got["not_close"])
	}
	if _, ok := got["not_far"]; ok {
		t.Fatalf("u1 must not be assigned twice, got %q", got["not_far"])
	}

	// Ambiguous: two same-title candidates whose date-deltas are within the
	// ambiguity gap → decline rather than guess which meeting's notes to write.
	amb := map[string][]mcpMeeting{
		normTitle("Ambiguous"): {
			{UUID: "a1", Title: "Ambiguous", Date: base.Add(-1 * time.Hour)}, // delta 1h
			{UUID: "a2", Title: "Ambiguous", Date: base.Add(1 * time.Hour)},  // delta 1h
		},
	}
	got = assignMeetingUUIDs(amb, []restTarget{{ID: "not_amb", Title: "Ambiguous", Created: base}})
	if u, ok := got["not_amb"]; ok {
		t.Fatalf("ambiguous candidates (deltas 1h/1h) should decline, got %q", u)
	}

	// Boundary: a delta gap of EXACTLY matchAmbiguityGap (2h) is still "within"
	// the gap and must decline (<=, not <).
	ambEdge := map[string][]mcpMeeting{
		normTitle("Edge"): {
			{UUID: "e1", Title: "Edge", Date: base.Add(30 * time.Minute)},             // delta 30m
			{UUID: "e2", Title: "Edge", Date: base.Add(2*time.Hour + 30*time.Minute)}, // delta 2h30m → gap 2h
		},
	}
	got = assignMeetingUUIDs(ambEdge, []restTarget{{ID: "not_edge", Title: "Edge", Created: base}})
	if u, ok := got["not_edge"]; ok {
		t.Fatalf("exactly-2h gap should decline (<=), got %q", u)
	}

	// Empty / unknown title / zero date → no match.
	got = assignMeetingUUIDs(byTitle, []restTarget{
		{ID: "e1", Title: "", Created: base},
		{ID: "e2", Title: "Nonexistent", Created: base},
		{ID: "e3", Title: "Unique Title", Created: time.Time{}},
	})
	if len(got) != 0 {
		t.Fatalf("empty/unknown/zero-date should not match, got %+v", got)
	}
}
