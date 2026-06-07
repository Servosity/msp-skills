// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"testing"
	"time"
)

func TestNovelSince(t *testing.T) {
	t.Run("empty store returns honest empty envelope", func(t *testing.T) {
		novelTestEnv(t)
		out, _, err := runNovel(t, "since", "24h")
		if err != nil {
			t.Fatalf("since: %v", err)
		}
		var res map[string]any
		assertJSON(t, out, &res)
		if res["window"] != "24h" {
			t.Errorf("window = %v, want 24h", res["window"])
		}
		na, ok := res["new_alerts"].([]any)
		if !ok || len(na) != 0 {
			t.Errorf("new_alerts should be empty array, got %v", res["new_alerts"])
		}
	})

	t.Run("window flag is honored", func(t *testing.T) {
		novelTestEnv(t)
		out, _, err := runNovel(t, "since", "--window", "2h")
		if err != nil {
			t.Fatalf("since: %v", err)
		}
		var res map[string]any
		assertJSON(t, out, &res)
		if res["window"] != "2h" {
			t.Errorf("window = %v, want 2h", res["window"])
		}
	})

	t.Run("recent alert and offline agent surface; stale alert is excluded", func(t *testing.T) {
		db := novelTestEnv(t)
		recent := time.Now().UTC().Format(time.RFC3339)
		seedResource(t, db, "alerts", "fresh", `{"alert_time":"`+recent+`","hostname":"server1","severity":"error","message":"disk full"}`)
		seedResource(t, db, "agents", "down1", `{"agent_id":"down1","hostname":"server1","status":"offline","last_seen":"`+recent+`"}`)

		out, _, err := runNovel(t, "since", "24h")
		if err != nil {
			t.Fatalf("since 24h: %v", err)
		}
		var res map[string]any
		assertJSON(t, out, &res)
		na, _ := res["new_alerts"].([]any)
		if len(na) != 1 {
			t.Fatalf("new_alerts = %v, want 1 fresh alert", res["new_alerts"])
		}
		alert := na[0].(map[string]any)
		if alert["message"] != "disk full" {
			t.Errorf("alert message = %v, want 'disk full'", alert["message"])
		}
		no, _ := res["newly_offline"].([]any)
		if len(no) != 1 {
			t.Fatalf("newly_offline = %v, want 1 offline agent", res["newly_offline"])
		}
		if no[0].(map[string]any)["hostname"] != "server1" {
			t.Errorf("offline hostname = %v, want server1", no[0])
		}

		// A 2020-dated alert must NOT appear within a 2h window: this exercises
		// the ISO->datetime coercion in the cutoff comparison.
		seedResource(t, db, "alerts", "ancient", `{"alert_time":"2020-01-01T00:00:00Z","hostname":"old","severity":"info","message":"ancient"}`)
		out2, _, err := runNovel(t, "since", "2h")
		if err != nil {
			t.Fatalf("since 2h: %v", err)
		}
		var res2 map[string]any
		assertJSON(t, out2, &res2)
		na2, _ := res2["new_alerts"].([]any)
		for _, a := range na2 {
			if a.(map[string]any)["message"] == "ancient" {
				t.Fatalf("2020 alert should be excluded by the 2h window, got %v", na2)
			}
		}
	})
}
