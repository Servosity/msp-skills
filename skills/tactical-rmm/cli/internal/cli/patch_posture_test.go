// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import "testing"

func TestNovelPatchPosture(t *testing.T) {
	t.Run("empty store returns empty array", func(t *testing.T) {
		novelTestEnv(t)
		out, _, err := runNovel(t, "patch", "posture")
		if err != nil {
			t.Fatalf("patch posture: %v", err)
		}
		var rows []map[string]any
		assertJSON(t, out, &rows)
		if len(rows) != 0 {
			t.Errorf("empty store should yield no rows, got %d", len(rows))
		}
	})

	t.Run("rolls up pending patches by client", func(t *testing.T) {
		db := novelTestEnv(t)
		seedResource(t, db, "agents", "a1", `{"agent_id":"a1","client_name":"Acme","has_patches_pending":1,"needs_reboot":1}`)
		seedResource(t, db, "agents", "a2", `{"agent_id":"a2","client_name":"Acme","has_patches_pending":0,"needs_reboot":0}`)
		out, _, err := runNovel(t, "patch", "posture", "--by", "client")
		if err != nil {
			t.Fatalf("patch posture: %v", err)
		}
		var rows []map[string]any
		assertJSON(t, out, &rows)
		if len(rows) != 1 {
			t.Fatalf("want 1 client group, got %d", len(rows))
		}
		if rows[0]["group"] != "Acme" || rows[0]["patches_pending"].(float64) != 1 {
			t.Errorf("unexpected rollup: %v", rows[0])
		}
		if rows[0]["agents"].(float64) != 2 {
			t.Errorf("agents = %v, want 2", rows[0]["agents"])
		}
	})
}
