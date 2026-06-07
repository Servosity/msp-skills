// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import "testing"

func TestNovelChecksWorst(t *testing.T) {
	t.Run("empty store returns empty array", func(t *testing.T) {
		novelTestEnv(t)
		out, _, err := runNovel(t, "checks", "worst")
		if err != nil {
			t.Fatalf("checks worst: %v", err)
		}
		var rows []map[string]any
		assertJSON(t, out, &rows)
		if len(rows) != 0 {
			t.Errorf("empty store should yield no rows, got %d", len(rows))
		}
	})

	t.Run("ranks failing checks by blast radius", func(t *testing.T) {
		db := novelTestEnv(t)
		// "Disk Space" fails on 2 agents, "CPU" on 1.
		seedResource(t, db, "checks", "c1", `{"id":"c1","readable_desc":"Disk Space","check_type":"diskspace","status":"failing"}`)
		seedResource(t, db, "checks", "c2", `{"id":"c2","readable_desc":"Disk Space","check_type":"diskspace","status":"failing"}`)
		seedResource(t, db, "checks", "c3", `{"id":"c3","readable_desc":"CPU","check_type":"cpuload","status":"failing"}`)
		seedResource(t, db, "checks", "c4", `{"id":"c4","readable_desc":"Memory","check_type":"memory","status":"passing"}`)
		out, _, err := runNovel(t, "checks", "worst")
		if err != nil {
			t.Fatalf("checks worst: %v", err)
		}
		var rows []map[string]any
		assertJSON(t, out, &rows)
		if len(rows) != 2 {
			t.Fatalf("want 2 failing check groups, got %d: %v", len(rows), rows)
		}
		if rows[0]["check"] != "Disk Space" || rows[0]["agents_failing"].(float64) != 2 {
			t.Errorf("top row should be Disk Space x2, got %v", rows[0])
		}
	})

	t.Run("limit caps results", func(t *testing.T) {
		db := novelTestEnv(t)
		seedResource(t, db, "checks", "c1", `{"id":"c1","readable_desc":"A","status":"failing"}`)
		seedResource(t, db, "checks", "c2", `{"id":"c2","readable_desc":"B","status":"failing"}`)
		out, _, err := runNovel(t, "checks", "worst", "--limit", "1")
		if err != nil {
			t.Fatalf("checks worst: %v", err)
		}
		var rows []map[string]any
		assertJSON(t, out, &rows)
		if len(rows) != 1 {
			t.Errorf("--limit 1 should yield 1 row, got %d", len(rows))
		}
	})
}
