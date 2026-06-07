// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Tests for the fleet stale (sync-freshness) command: timestamp parsing
// across the store's DATETIME shapes and threshold filtering against a
// seeded store whose rows were just written (fresh).

package cli

import (
	"bytes"
	"testing"
	"time"
)

func TestParseStoreTimestamp(t *testing.T) {
	cases := []struct {
		name string
		in   string
		zero bool
		year int
	}{
		{"sqlite current_timestamp", "2026-06-06 12:30:00", false, 2026},
		{"rfc3339", "2026-06-06T12:30:00Z", false, 2026},
		{"bare iso", "2026-06-06T12:30:00", false, 2026},
		{"empty", "", true, 0},
		{"garbage", "not-a-time", true, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseStoreTimestamp(tc.in)
			if got.IsZero() != tc.zero {
				t.Fatalf("parseStoreTimestamp(%q).IsZero() = %v, want %v", tc.in, got.IsZero(), tc.zero)
			}
			if !tc.zero && got.Year() != tc.year {
				t.Fatalf("parseStoreTimestamp(%q).Year() = %d, want %d", tc.in, got.Year(), tc.year)
			}
		})
	}
}

func TestFleetStaleFreshStoreFiltered(t *testing.T) {
	dbPath := seedFleetStore(t)
	// Rows were just written, so a generous threshold filters everything out.
	flags := &rootFlags{asJSON: true, maxAge: 24 * time.Hour}
	rows := runFleetJSONArray(t, newNovelFleetStaleCmd(flags), dbPath)
	if len(rows) != 0 {
		t.Fatalf("fresh store with 24h threshold returned %d stale rows, want 0\nrows: %+v", len(rows), rows)
	}
}

func TestFleetStaleZeroThresholdListsAll(t *testing.T) {
	dbPath := seedFleetStore(t)
	// --max-age 0 disables filtering: every agent is listed with its age.
	flags := &rootFlags{asJSON: true, maxAge: 0}
	rows := runFleetJSONArray(t, newNovelFleetStaleCmd(flags), dbPath)
	if len(rows) != 2 {
		t.Fatalf("zero threshold returned %d rows, want 2 (both agents)\nrows: %+v", len(rows), rows)
	}
	for _, r := range rows {
		if r["last_synced"] == "" || r["last_synced"] == nil {
			t.Fatalf("row missing last_synced: %+v", r)
		}
		if _, ok := r["age_hours"]; !ok {
			t.Fatalf("row missing age_hours: %+v", r)
		}
	}
}

func TestFleetStaleRejectsLiveDataSource(t *testing.T) {
	dbPath := seedFleetStore(t)
	flags := &rootFlags{asJSON: true, dataSource: "live"}
	cmd := newNovelFleetStaleCmd(flags)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"--db", dbPath})
	if err := cmd.Execute(); err == nil {
		t.Fatal("fleet stale with --data-source live must error (local-only command)")
	}
}
