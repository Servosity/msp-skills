// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import "testing"

func TestNovelAgentsStale(t *testing.T) {
	t.Run("empty store returns empty array", func(t *testing.T) {
		novelTestEnv(t)
		out, _, err := runNovel(t, "agents", "stale", "--days", "7")
		if err != nil {
			t.Fatalf("agents stale: %v", err)
		}
		var rows []map[string]any
		assertJSON(t, out, &rows)
		if len(rows) != 0 {
			t.Errorf("empty store should yield no rows, got %d", len(rows))
		}
	})

	t.Run("includes offline agents", func(t *testing.T) {
		db := novelTestEnv(t)
		seedResource(t, db, "agents", "off", `{"agent_id":"off","status":"offline","hostname":"down","last_seen":"2020-01-01T00:00:00Z"}`)
		out, _, err := runNovel(t, "agents", "stale", "--days", "7")
		if err != nil {
			t.Fatalf("agents stale: %v", err)
		}
		var rows []map[string]any
		assertJSON(t, out, &rows)
		if len(rows) != 1 || rows[0]["agent_id"] != "off" {
			t.Fatalf("expected the offline agent, got %v", rows)
		}
	})
}
