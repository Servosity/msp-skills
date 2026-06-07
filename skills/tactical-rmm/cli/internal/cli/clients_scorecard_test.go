// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import "testing"

func TestNovelClientsScorecard(t *testing.T) {
	t.Run("empty store returns empty array", func(t *testing.T) {
		novelTestEnv(t)
		out, _, err := runNovel(t, "clients", "scorecard")
		if err != nil {
			t.Fatalf("clients scorecard: %v", err)
		}
		var rows []map[string]any
		assertJSON(t, out, &rows)
		if len(rows) != 0 {
			t.Errorf("empty store should yield no rows, got %d", len(rows))
		}
	})

	t.Run("one row per client with derived signals", func(t *testing.T) {
		db := novelTestEnv(t)
		seedResource(t, db, "agents", "a1", `{"agent_id":"a1","client_name":"Acme","status":"online","checks":{"failing":2},"has_patches_pending":1}`)
		seedResource(t, db, "agents", "a2", `{"agent_id":"a2","client_name":"Acme","status":"offline","checks":{"failing":0},"has_patches_pending":0}`)
		seedResource(t, db, "alerts", "al1", `{"client":"Acme","resolved":0}`)
		out, _, err := runNovel(t, "clients", "scorecard")
		if err != nil {
			t.Fatalf("clients scorecard: %v", err)
		}
		var rows []map[string]any
		assertJSON(t, out, &rows)
		if len(rows) != 1 {
			t.Fatalf("want 1 client row, got %d: %v", len(rows), rows)
		}
		r := rows[0]
		if r["client"] != "Acme" {
			t.Errorf("client = %v, want Acme", r["client"])
		}
		if r["agent_count"].(float64) != 2 {
			t.Errorf("agent_count = %v, want 2", r["agent_count"])
		}
		if r["online_pct"].(float64) != 50 {
			t.Errorf("online_pct = %v, want 50", r["online_pct"])
		}
		if r["failing_checks"].(float64) != 2 {
			t.Errorf("failing_checks = %v, want 2", r["failing_checks"])
		}
		if r["pending_patches"].(float64) != 1 {
			t.Errorf("pending_patches = %v, want 1", r["pending_patches"])
		}
		if r["open_alerts"].(float64) != 1 {
			t.Errorf("open_alerts = %v, want 1", r["open_alerts"])
		}
	})
}
