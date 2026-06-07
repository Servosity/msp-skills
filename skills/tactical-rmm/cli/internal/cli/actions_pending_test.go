// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import "testing"

func TestNovelActionsPending(t *testing.T) {
	t.Run("empty store returns honest empty envelope without network", func(t *testing.T) {
		novelTestEnv(t)
		out, _, err := runNovel(t, "actions", "pending")
		if err != nil {
			t.Fatalf("actions pending: %v", err)
		}
		var res map[string]any
		assertJSON(t, out, &res)
		items, ok := res["items"].([]any)
		if !ok || len(items) != 0 {
			t.Errorf("items should be empty array, got %v", res["items"])
		}
		if res["scanned_agents"].(float64) != 0 {
			t.Errorf("scanned_agents = %v, want 0 (no agents synced)", res["scanned_agents"])
		}
		if _, ok := res["note"]; !ok {
			t.Errorf("expected an explanatory note when no agents are synced, got %v", res)
		}
		if res["total_pending"].(float64) != 0 {
			t.Errorf("total_pending = %v, want 0", res["total_pending"])
		}
		// Nothing was attempted, so the failure accounting must be zero and the
		// command must still exit 0 (the all-fail error only fires when
		// scanned > 0).
		if ff, ok := res["fetch_failures"].(float64); !ok || ff != 0 {
			t.Errorf("fetch_failures = %v, want 0", res["fetch_failures"])
		}
		if _, ok := res["first_error"]; ok {
			t.Errorf("first_error should be omitted when nothing failed, got %v", res["first_error"])
		}
	})
}
