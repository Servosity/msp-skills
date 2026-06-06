// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"testing"
	"time"
)

func TestNovelSnapshotDiffCommand(t *testing.T) {
	t.Run("first run captures baseline and exits", func(t *testing.T) {
		db, dbPath := newNovelTestStore(t)
		seedResource(t, db, "customers", "C1", map[string]any{"id": "C1", "name": "Acme"})

		cmd := newNovelSnapshotDiffCmd(&rootFlags{asJSON: true})
		parsed, _ := execNovel(t, cmd, []string{"--db", dbPath})
		if parsed["baseline_captured"] != true {
			t.Errorf("expected baseline_captured=true, got %v", parsed)
		}
	})

	t.Run("empty store still captures a baseline", func(t *testing.T) {
		_, dbPath := newNovelTestStore(t)
		cmd := newNovelSnapshotDiffCmd(&rootFlags{asJSON: true})
		parsed, _ := execNovel(t, cmd, []string{"--db", dbPath})
		if parsed["baseline_captured"] != true {
			t.Errorf("expected baseline_captured=true on empty store, got %v", parsed)
		}
	})

	t.Run("diff reports added and changed against an aged prior snapshot", func(t *testing.T) {
		db, dbPath := newNovelTestStore(t)

		// Seed initial state and a backdated prior snapshot directly so it
		// qualifies as "older than --since".
		seedResource(t, db, "customers", "C1", map[string]any{"id": "C1", "name": "Acme"})
		seedResource(t, db, "customers", "C2", map[string]any{"id": "C2", "name": "Beta"})

		sdb := db.DB()
		if _, err := sdb.Exec(`CREATE TABLE IF NOT EXISTS snapshots (
			captured_at DATETIME NOT NULL,
			resource_type TEXT NOT NULL,
			id TEXT NOT NULL,
			content_hash TEXT NOT NULL
		)`); err != nil {
			t.Fatalf("create snapshots: %v", err)
		}
		old := time.Now().Add(-10 * 24 * time.Hour).UTC().Format(time.RFC3339Nano)
		// Prior snapshot: C1 with an old hash (will be "changed"), C2 not
		// present in current after we mutate, plus the original C2 hash.
		c1OldHash := contentHash16([]byte(`{"id":"C1","name":"OLD"}`))
		c2Hash := contentHash16([]byte(string(mustJSON(t, map[string]any{"id": "C2", "name": "Beta"}))))
		if _, err := sdb.Exec(`INSERT INTO snapshots(captured_at,resource_type,id,content_hash) VALUES(?,?,?,?)`, old, "customers", "C1", c1OldHash); err != nil {
			t.Fatalf("insert prior C1: %v", err)
		}
		if _, err := sdb.Exec(`INSERT INTO snapshots(captured_at,resource_type,id,content_hash) VALUES(?,?,?,?)`, old, "customers", "C3", "deadbeefdeadbeef"); err != nil {
			t.Fatalf("insert prior C3: %v", err)
		}
		_ = c2Hash

		// Add a brand-new resource so current has an "added" relative to prior.
		seedResource(t, db, "customers", "C2b", map[string]any{"id": "C2b", "name": "Gamma"})

		cmd := newNovelSnapshotDiffCmd(&rootFlags{asJSON: true})
		parsed, _ := execNovel(t, cmd, []string{"--db", dbPath, "--since", "1h"})

		if _, ok := parsed["from"]; !ok {
			t.Fatalf("expected a diff (with 'from'), got %v", parsed)
		}
		// C1 changed (hash differs), C3 removed (not in current),
		// C1/C2/C2b added relative to prior except C1 which existed.
		totalChanged := int(jsonFloat(t, parsed, "total_changed"))
		totalRemoved := int(jsonFloat(t, parsed, "total_removed"))
		totalAdded := int(jsonFloat(t, parsed, "total_added"))
		if totalChanged < 1 {
			t.Errorf("total_changed = %d, want >=1 (C1 hash differs)", totalChanged)
		}
		if totalRemoved < 1 {
			t.Errorf("total_removed = %d, want >=1 (C3 gone)", totalRemoved)
		}
		if totalAdded < 1 {
			t.Errorf("total_added = %d, want >=1 (C2/C2b new)", totalAdded)
		}
	})

	t.Run("capture-only does not diff", func(t *testing.T) {
		db, dbPath := newNovelTestStore(t)
		seedResource(t, db, "customers", "C1", map[string]any{"id": "C1"})
		cmd := newNovelSnapshotDiffCmd(&rootFlags{asJSON: true})
		parsed, _ := execNovel(t, cmd, []string{"--db", dbPath, "--capture-only"})
		if _, ok := parsed["captured_at"]; !ok {
			t.Errorf("expected captured_at, got %v", parsed)
		}
		if _, ok := parsed["from"]; ok {
			t.Errorf("capture-only should not produce a diff: %v", parsed)
		}
	})
}
