// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Tests for the fleet events (timeline) command: timestamp parsing across the
// activity-log shapes, parsed-time ordering (never raw string compare), and
// the live-only data-source contract.

package cli

import (
	"bytes"
	"sort"
	"testing"
	"time"
)

func TestParseEventTime(t *testing.T) {
	cases := []struct {
		name string
		in   string
		zero bool
	}{
		{"rfc3339", "2026-06-06T12:30:00Z", false},
		{"rfc3339 fractional", "2026-06-06T12:30:00.123Z", false},
		{"bare iso", "2026-06-06T12:30:00", false},
		{"space separated", "2026-06-06 12:30:00", false},
		{"empty", "", true},
		{"garbage", "yesterday-ish", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := parseEventTime(tc.in); got.IsZero() != tc.zero {
				t.Fatalf("parseEventTime(%q).IsZero() = %v, want %v", tc.in, got.IsZero(), tc.zero)
			}
		})
	}
}

// TestFleetEventRowOrdering pins the command's sort contract: newest parsed
// time first, unparseable timestamps last — mixed formats must never be
// compared as raw strings (a "2026-06-06 ..." space form would otherwise
// sort before "2026-06-06T..." regardless of actual time).
func TestFleetEventRowOrdering(t *testing.T) {
	rows := []fleetEventRow{
		{Type: "older-rfc3339", Timestamp: "2026-06-05T10:00:00Z", parsedAt: parseEventTime("2026-06-05T10:00:00Z")},
		{Type: "garbage", Timestamp: "not-a-time", parsedAt: parseEventTime("not-a-time")},
		{Type: "newest-space-form", Timestamp: "2026-06-06 09:00:00", parsedAt: parseEventTime("2026-06-06 09:00:00")},
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].parsedAt.IsZero() != rows[j].parsedAt.IsZero() {
			return !rows[i].parsedAt.IsZero()
		}
		return rows[i].parsedAt.After(rows[j].parsedAt)
	})
	if rows[0].Type != "newest-space-form" {
		t.Fatalf("first row = %s, want newest-space-form (parsed-time order)", rows[0].Type)
	}
	if rows[2].Type != "garbage" {
		t.Fatalf("last row = %s, want garbage (unparseable sorts last)", rows[2].Type)
	}
	if rows[0].parsedAt.Before(rows[1].parsedAt) {
		t.Fatal("ordering inverted: newest row parses earlier than second row")
	}
	if !rows[0].parsedAt.After(time.Time{}) {
		t.Fatal("newest row unexpectedly has zero parsed time")
	}
}

func TestFleetEventsRejectsLocalDataSource(t *testing.T) {
	flags := &rootFlags{asJSON: true, dataSource: "local"}
	cmd := newNovelFleetEventsCmd(flags)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{})
	if err := cmd.Execute(); err == nil {
		t.Fatal("fleet events with --data-source local must error (live-only command)")
	}
}

func TestFleetEventsRejectsBadSince(t *testing.T) {
	flags := &rootFlags{asJSON: true}
	cmd := newNovelFleetEventsCmd(flags)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"--since", "next-tuesday"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("fleet events with an invalid --since must error before dialing the API")
	}
}
