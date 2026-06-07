// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"strings"
	"testing"
)

func TestNovelServicesDown(t *testing.T) {
	t.Run("empty store returns honest empty envelope without network", func(t *testing.T) {
		novelTestEnv(t)
		out, _, err := runNovel(t, "services", "down", "--name", "Spooler")
		if err != nil {
			t.Fatalf("services down: %v", err)
		}
		var res map[string]any
		assertJSON(t, out, &res)
		items, ok := res["items"].([]any)
		if !ok || len(items) != 0 {
			t.Errorf("items should be empty array, got %v", res["items"])
		}
		if res["service"] != "Spooler" {
			t.Errorf("service = %v, want Spooler", res["service"])
		}
		if res["scanned_agents"].(float64) != 0 {
			t.Errorf("scanned_agents = %v, want 0", res["scanned_agents"])
		}
		// status_unknown must be absent-or-0 when nothing was scanned.
		if su, ok := res["status_unknown"].(float64); ok && su != 0 {
			t.Errorf("status_unknown = %v, want 0 or absent", res["status_unknown"])
		}
		if ff, ok := res["fetch_failures"].(float64); ok && ff != 0 {
			t.Errorf("fetch_failures = %v, want 0 or absent", res["fetch_failures"])
		}
	})

	t.Run("missing --name is a usage error in real mode", func(t *testing.T) {
		novelTestEnv(t)
		_, _, err := runNovel(t, "services", "down")
		if err == nil || !strings.Contains(err.Error(), "--name") {
			t.Fatalf("expected usage error about --name, got %v", err)
		}
	})
}
