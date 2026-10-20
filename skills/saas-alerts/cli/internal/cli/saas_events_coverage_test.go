// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/saas-alerts/handfixes.json
// under the "events-window-coverage" entry.

package cli

import (
	"context"
	"strings"
	"testing"
	"time"

	"saas-alerts-pp-cli/internal/store"
)

func TestSaasEventsCoverageFloor(t *testing.T) {
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	d24, d7 := now.Add(-24*time.Hour), now.Add(-7*24*time.Hour)
	w := now.Add(-time.Hour) // watermark of the previous completed sync
	for _, tc := range []struct {
		name     string
		since    string
		last     time.Time
		full     bool
		prev     time.Time
		havePrev bool
		want     time.Time
	}{
		{"first sync, no --since: full history", "", time.Time{}, false, time.Time{}, false, time.Time{}},
		{"first sync --since 24h", d24.Format(time.RFC3339), time.Time{}, false, time.Time{}, false, d24},
		{"incremental keeps the floor", "", w, false, d24, true, d24},
		{"--since 7d before the watermark extends", d7.Format(time.RFC3339), w, false, d24, true, d7},
		{"--since 2h before the watermark keeps the older floor", now.Add(-2 * time.Hour).Format(time.RFC3339), w, false, d24, true, d24},
		{"--since after the watermark leaves a hole", now.Add(-30 * time.Minute).Format(time.RFC3339), w, false, d24, true, now.Add(-30 * time.Minute)},
		{"--full without --since: full history", "", w, true, d24, true, time.Time{}},
	} {
		if got := saasEventsCoverageFloor(tc.since, tc.last, tc.full, tc.prev, tc.havePrev); !got.Equal(tc.want) {
			t.Errorf("%s: floor %v, want %v", tc.name, got, tc.want)
		}
	}
}

// TestSaasWindowOlderThanCoverageIsNotSynced: a store synced with --since 24h
// must not answer a 7d question as complete.
func TestSaasWindowOlderThanCoverageIsNotSynced(t *testing.T) {
	saasIsolate(t)
	dbPath := saasSeedFixtureStore(t)
	rw, err := store.OpenWithContext(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := rw.SaveEventsCoverage(saasLocalNow().UTC().Add(-24 * time.Hour)); err != nil {
		t.Fatal(err)
	}
	rw.Close()
	type env struct {
		Synced bool   `json:"synced"`
		Note   string `json:"note"`
	}
	for _, args := range [][]string{{"triage", "--since", "7d"}, {"cross-tenant", "--since", "7d"}, {"investigate", "jane.doe@example.com", "--since", "7d"}, {"coverage", "--silent", "72h"}} {
		var got env
		saasRunJSON(t, &got, append(args, "--db", dbPath, "--json")...)
		if got.Synced || !strings.HasPrefix(got.Note, "the local store holds events only from ") || !strings.Contains(got.Note, "sync --resources events --since "+args[len(args)-1]) {
			t.Errorf("%v: synced=%v note=%q", args, got.Synced, got.Note)
		}
	}
	var ok env
	saasRunJSON(t, &ok, "triage", "--since", "12h", "--db", dbPath, "--json")
	if !ok.Synced || ok.Note != "" {
		t.Errorf("12h window inside 24h coverage: synced=%v note=%q", ok.Synced, ok.Note)
	}
}
