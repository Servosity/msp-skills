// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import "testing"

func TestNovelFleetHealth(t *testing.T) {
	t.Run("empty store emits honest zeros", func(t *testing.T) {
		novelTestEnv(t)
		out, _, err := runNovel(t, "fleet", "health")
		if err != nil {
			t.Fatalf("fleet health: %v", err)
		}
		var res map[string]any
		assertJSON(t, out, &res)
		agents, ok := res["agents"].(map[string]any)
		if !ok {
			t.Fatalf("missing agents block: %v", res)
		}
		if agents["total"].(float64) != 0 {
			t.Errorf("empty store should report 0 agents, got %v", agents["total"])
		}
	})

	t.Run("counts seeded agents", func(t *testing.T) {
		db := novelTestEnv(t)
		seedResource(t, db, "agents", "a1", `{"agent_id":"a1","status":"online","client_name":"Acme","site_name":"HQ"}`)
		seedResource(t, db, "agents", "a2", `{"agent_id":"a2","status":"offline","client_name":"Acme","site_name":"HQ"}`)
		out, _, err := runNovel(t, "fleet", "health")
		if err != nil {
			t.Fatalf("fleet health: %v", err)
		}
		var res map[string]any
		assertJSON(t, out, &res)
		agents := res["agents"].(map[string]any)
		if agents["total"].(float64) != 2 {
			t.Errorf("total agents = %v, want 2", agents["total"])
		}
		if agents["offline"].(float64) != 1 {
			t.Errorf("offline = %v, want 1", agents["offline"])
		}
	})
}
