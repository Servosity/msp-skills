// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

// HAND-FIX(sync-clear-never-marks-complete): a --full / --latest-only /
// archive clear never marks a collection complete; only a finished walk does.
func TestClearSyncCursorNeverMarksComplete(t *testing.T) {
	s, err := OpenWithContext(context.Background(), filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	complete := func(r string) int {
		var c int
		if err := s.DB().QueryRow(`SELECT last_attempt_complete FROM sync_state WHERE resource_type = ?`, r).Scan(&c); err != nil {
			t.Fatal(err)
		}
		return c
	}
	if err := s.ClearSyncCursor("health"); err != nil || complete("health") != 0 {
		t.Fatalf("a clear on a never-synced collection must stay incomplete: %v", err)
	}
	if err := s.SaveSyncStateAt("devices", "c1", 5, time.Now()); err != nil || complete("devices") != 1 {
		t.Fatalf("a finished walk is complete: %v", err)
	}
	if err := s.ClearSyncCursor("devices"); err != nil || complete("devices") != 0 {
		t.Fatalf("a clear before a re-walk must mark it incomplete: %v", err)
	}
	if cur, at, _, err := s.GetSyncState("devices"); err != nil || cur != "" || !at.IsZero() {
		t.Fatalf("cursor and watermark reset: %q %v %v", cur, at, err)
	}
}
