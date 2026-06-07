// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"strings"
	"testing"
)

func TestNovelAgentsBulkRun(t *testing.T) {
	t.Run("previews by default without --execute", func(t *testing.T) {
		db := novelTestEnv(t)
		seedResource(t, db, "agents", "a1", `{"agent_id":"a1","hostname":"h1","status":"online","plat":"windows"}`)
		out, _, err := runNovel(t, "agents", "bulk-run", "--command", "whoami")
		if err != nil {
			t.Fatalf("bulk-run: %v", err)
		}
		var res map[string]any
		assertJSON(t, out, &res)
		if res["dry_run"] != true {
			t.Errorf("expected dry_run preview, got %v", res)
		}
		if res["cohort_size"].(float64) != 1 {
			t.Errorf("cohort_size = %v, want 1", res["cohort_size"])
		}
	})

	t.Run("filter narrows cohort", func(t *testing.T) {
		db := novelTestEnv(t)
		seedResource(t, db, "agents", "win", `{"agent_id":"win","plat":"windows"}`)
		seedResource(t, db, "agents", "lin", `{"agent_id":"lin","plat":"linux"}`)
		out, _, err := runNovel(t, "agents", "bulk-run", "--command", "whoami", "--filter", "os=windows")
		if err != nil {
			t.Fatalf("bulk-run: %v", err)
		}
		var res map[string]any
		assertJSON(t, out, &res)
		if res["cohort_size"].(float64) != 1 {
			t.Errorf("filter os=windows cohort_size = %v, want 1", res["cohort_size"])
		}
	})

	t.Run("missing command/script is a usage error", func(t *testing.T) {
		novelTestEnv(t)
		_, _, err := runNovel(t, "agents", "bulk-run")
		if err == nil || !strings.Contains(err.Error(), "--script") {
			t.Fatalf("expected usage error about --script/--command, got %v", err)
		}
	})

	t.Run("apostrophe-bearing client name matches via parameterized WHERE", func(t *testing.T) {
		db := novelTestEnv(t)
		seedResource(t, db, "agents", "ob", `{"agent_id":"ob","hostname":"obrien-pc","client_name":"O'Brien"}`)
		seedResource(t, db, "agents", "other", `{"agent_id":"other","hostname":"acme-pc","client_name":"Acme"}`)
		out, _, err := runNovel(t, "agents", "bulk-run", "--command", "whoami", "--filter", "client=O'Brien")
		if err != nil {
			t.Fatalf("bulk-run: %v", err)
		}
		var res map[string]any
		assertJSON(t, out, &res)
		if res["cohort_size"].(float64) != 1 {
			t.Fatalf("filter client=O'Brien cohort_size = %v, want 1 (parameterization should match the apostrophe)", res["cohort_size"])
		}
		hosts, _ := res["hostnames"].([]any)
		if len(hosts) != 1 || hosts[0] != "obrien-pc" {
			t.Errorf("expected only the O'Brien agent, got %v", res["hostnames"])
		}
	})
}
