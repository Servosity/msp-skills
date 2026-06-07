// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import "testing"

func TestNovelCoverage(t *testing.T) {
	t.Run("empty store returns empty array", func(t *testing.T) {
		novelTestEnv(t)
		out, _, err := runNovel(t, "coverage")
		if err != nil {
			t.Fatalf("coverage: %v", err)
		}
		var rows []map[string]any
		assertJSON(t, out, &rows)
		if len(rows) != 0 {
			t.Errorf("empty store should yield no rows, got %d", len(rows))
		}
	})

	t.Run("flags agents with zero checks", func(t *testing.T) {
		db := novelTestEnv(t)
		seedResource(t, db, "agents", "covered", `{"agent_id":"covered","checks":{"total":3}}`)
		seedResource(t, db, "agents", "gap", `{"agent_id":"gap","hostname":"naked","checks":{"total":0}}`)
		out, _, err := runNovel(t, "coverage")
		if err != nil {
			t.Fatalf("coverage: %v", err)
		}
		var rows []map[string]any
		assertJSON(t, out, &rows)
		if len(rows) != 1 || rows[0]["agent_id"] != "gap" {
			t.Fatalf("expected only the unmonitored agent, got %v", rows)
		}
	})
}
