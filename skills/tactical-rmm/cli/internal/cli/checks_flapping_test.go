// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"strings"
	"testing"
	"time"

	"tactical-rmm-pp-cli/internal/store"
)

// seedCheckSnapshot inserts one row into the check_snapshots history table at a
// controlled timestamp so flapping flip-counts are deterministic.
func seedCheckSnapshot(t *testing.T, dbPath, checkID, agentID, status string, ts time.Time) {
	t.Helper()
	s, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer s.Close()
	db := s.DB()
	if _, err := db.Exec("CREATE TABLE IF NOT EXISTS check_snapshots (check_id TEXT, agent_id TEXT, status TEXT, ts TIMESTAMP DEFAULT CURRENT_TIMESTAMP)"); err != nil {
		t.Fatalf("create check_snapshots: %v", err)
	}
	if _, err := db.Exec("INSERT INTO check_snapshots(check_id,agent_id,status,ts) VALUES(?,?,?,?)", checkID, agentID, status, ts.UTC().Format("2006-01-02 15:04:05")); err != nil {
		t.Fatalf("insert snapshot: %v", err)
	}
}

func TestNovelChecksFlapping(t *testing.T) {
	t.Run("empty store returns empty array with note", func(t *testing.T) {
		novelTestEnv(t)
		out, errOut, err := runNovel(t, "checks", "flapping", "--window", "24h")
		if err != nil {
			t.Fatalf("checks flapping: %v", err)
		}
		var rows []map[string]any
		assertJSON(t, out, &rows)
		if len(rows) != 0 {
			t.Errorf("empty history should yield no rows, got %d", len(rows))
		}
		if !strings.Contains(errOut, "no flapping detected") {
			t.Errorf("expected stderr hint about building history, got %q", errOut)
		}
	})

	t.Run("snapshot records current check statuses", func(t *testing.T) {
		db := novelTestEnv(t)
		seedResource(t, db, "checks", "c1", `{"id":"c1","agent":"a1","status":"passing"}`)
		out, _, err := runNovel(t, "checks", "flapping", "--snapshot")
		if err != nil {
			t.Fatalf("checks flapping --snapshot: %v", err)
		}
		var res map[string]any
		assertJSON(t, out, &res)
		if res["snapshot_recorded"].(float64) != 1 {
			t.Errorf("snapshot_recorded = %v, want 1", res["snapshot_recorded"])
		}
	})

	t.Run("flip count includes flapping check and excludes below-threshold", func(t *testing.T) {
		db := novelTestEnv(t)
		now := time.Now()
		// flap1: passing -> failing -> passing == 2 flips (>= min-flips 2).
		seedCheckSnapshot(t, db, "flap1", "a1", "passing", now.Add(-3*time.Hour))
		seedCheckSnapshot(t, db, "flap1", "a1", "failing", now.Add(-2*time.Hour))
		seedCheckSnapshot(t, db, "flap1", "a1", "passing", now.Add(-1*time.Hour))
		// steady1: passing -> passing == 0 flips (below threshold).
		seedCheckSnapshot(t, db, "steady1", "a1", "passing", now.Add(-3*time.Hour))
		seedCheckSnapshot(t, db, "steady1", "a1", "passing", now.Add(-1*time.Hour))

		out, errOut, err := runNovel(t, "checks", "flapping", "--min-flips", "2", "--window", "24h")
		if err != nil {
			t.Fatalf("checks flapping: %v (stderr=%q)", err, errOut)
		}
		var rows []map[string]any
		assertJSON(t, out, &rows)
		if len(rows) != 1 {
			t.Fatalf("expected exactly the flapping check, got %d rows: %v", len(rows), rows)
		}
		if rows[0]["check_id"] != "flap1" {
			t.Errorf("flapping check_id = %v, want flap1", rows[0]["check_id"])
		}
		if rows[0]["flips"].(float64) != 2 {
			t.Errorf("flips = %v, want 2", rows[0]["flips"])
		}
	})
}
