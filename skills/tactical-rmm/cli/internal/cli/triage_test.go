// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import "testing"

func TestNovelTriage(t *testing.T) {
	t.Run("empty store returns empty array", func(t *testing.T) {
		novelTestEnv(t)
		out, _, err := runNovel(t, "triage")
		if err != nil {
			t.Fatalf("triage: %v", err)
		}
		var rows []map[string]any
		assertJSON(t, out, &rows)
		if len(rows) != 0 {
			t.Errorf("empty store should yield no rows, got %d", len(rows))
		}
	})

	t.Run("ranks offline above healthy and skips healthy", func(t *testing.T) {
		db := novelTestEnv(t)
		seedResource(t, db, "agents", "ok", `{"agent_id":"ok","status":"online"}`)
		seedResource(t, db, "agents", "bad", `{"agent_id":"bad","status":"offline","hostname":"server1"}`)
		out, _, err := runNovel(t, "triage")
		if err != nil {
			t.Fatalf("triage: %v", err)
		}
		var rows []map[string]any
		assertJSON(t, out, &rows)
		if len(rows) != 1 {
			t.Fatalf("only the offline agent should score > 0, got %d rows", len(rows))
		}
		if rows[0]["agent_id"] != "bad" {
			t.Errorf("top row = %v, want bad", rows[0]["agent_id"])
		}
		if rows[0]["score"].(float64) <= 0 {
			t.Errorf("score should be positive, got %v", rows[0]["score"])
		}
	})

	t.Run("limit caps results", func(t *testing.T) {
		db := novelTestEnv(t)
		seedResource(t, db, "agents", "a", `{"agent_id":"a","status":"offline"}`)
		seedResource(t, db, "agents", "b", `{"agent_id":"b","status":"offline"}`)
		out, _, err := runNovel(t, "triage", "--limit", "1")
		if err != nil {
			t.Fatalf("triage: %v", err)
		}
		var rows []map[string]any
		assertJSON(t, out, &rows)
		if len(rows) != 1 {
			t.Errorf("--limit 1 should yield 1 row, got %d", len(rows))
		}
	})
}
