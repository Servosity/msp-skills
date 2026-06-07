// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import "testing"

func TestNovelMaintenanceSet(t *testing.T) {
	t.Run("previews empty cohort without --execute", func(t *testing.T) {
		novelTestEnv(t)
		out, _, err := runNovel(t, "maintenance", "set", "--filter", "site=HQ")
		if err != nil {
			t.Fatalf("maintenance set: %v", err)
		}
		var res map[string]any
		assertJSON(t, out, &res)
		if res["dry_run"] != true {
			t.Errorf("expected dry_run preview, got %v", res)
		}
		if res["action"] != "enable" {
			t.Errorf("action = %v, want enable", res["action"])
		}
		if res["cohort_size"].(float64) != 0 {
			t.Errorf("empty store cohort_size = %v, want 0", res["cohort_size"])
		}
	})

	t.Run("clear flag flips action", func(t *testing.T) {
		db := novelTestEnv(t)
		seedResource(t, db, "agents", "a1", `{"agent_id":"a1","site_name":"HQ"}`)
		out, _, err := runNovel(t, "maintenance", "set", "--filter", "site=HQ", "--clear")
		if err != nil {
			t.Fatalf("maintenance set --clear: %v", err)
		}
		var res map[string]any
		assertJSON(t, out, &res)
		if res["action"] != "clear" || res["maintenance_mode"] != false {
			t.Errorf("clear should disable maintenance, got %v", res)
		}
		if res["cohort_size"].(float64) != 1 {
			t.Errorf("cohort_size = %v, want 1", res["cohort_size"])
		}
	})

	t.Run("until window records a reminder hint, not server expiry", func(t *testing.T) {
		db := novelTestEnv(t)
		seedResource(t, db, "agents", "a1", `{"agent_id":"a1","site_name":"HQ"}`)
		out, _, err := runNovel(t, "maintenance", "set", "--filter", "site=HQ", "--until", "4h")
		if err != nil {
			t.Fatalf("maintenance set --until: %v", err)
		}
		var res map[string]any
		assertJSON(t, out, &res)
		if _, ok := res["expires_hint"]; !ok {
			t.Errorf("expected expires_hint in preview, got %v", res)
		}
	})
}
