// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/synology/handfixes.json
// under the "sync-clear-never-marks-complete" entry.

package cli

import (
	"path/filepath"
	"testing"

	"synology-pp-cli/internal/store"
)

// TestSyncUsageErrorsLeaveSyncStateUntouched: sync --full and --latest-only
// cleared every collection's cursor with last_attempt_complete=1 before
// validating --since or walking anything, so a usage error (or an interrupt)
// left five never-walked collections reading as synced with zero rows, and
// MCP search/sql reported the store "ready".
func TestSyncUsageErrorsLeaveSyncStateUntouched(t *testing.T) {
	sweepIsolate(t)
	db := filepath.Join(t.TempDir(), "data.db")
	for _, args := range [][]string{
		{"sync", "--full", "--since", "bogus", "--db", db},
		{"sync", "--latest-only", "--resources", "user,nosuchresource", "--since", "also-bogus", "--db", db},
	} {
		cmd := RootCmd()
		cmd.SetArgs(args)
		if err := cmd.Execute(); err == nil {
			t.Fatalf("%v succeeded; want a usage error", args)
		}
	}
	s, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var rows, complete int
	if err := s.DB().QueryRow(`SELECT COUNT(*), COALESCE(SUM(last_attempt_complete), 0) FROM sync_state`).Scan(&rows, &complete); err != nil {
		t.Fatal(err)
	}
	if rows != 0 || complete != 0 {
		t.Fatalf("a refused sync wrote %d sync_state row(s), %d marked complete", rows, complete)
	}
}
