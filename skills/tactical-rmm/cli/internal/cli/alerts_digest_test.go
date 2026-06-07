// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"testing"
	"time"
)

func TestNovelAlertsDigest(t *testing.T) {
	t.Run("empty store returns empty array", func(t *testing.T) {
		novelTestEnv(t)
		out, _, err := runNovel(t, "alerts", "digest", "--since", "24h")
		if err != nil {
			t.Fatalf("alerts digest: %v", err)
		}
		var rows []map[string]any
		assertJSON(t, out, &rows)
		if len(rows) != 0 {
			t.Errorf("empty store should yield no rows, got %d", len(rows))
		}
	})

	t.Run("groups recent alerts by severity", func(t *testing.T) {
		db := novelTestEnv(t)
		now := time.Now().UTC().Format("2006-01-02T15:04:05Z")
		seedResource(t, db, "alerts", "al1", `{"severity":"error","resolved":0,"alert_time":"`+now+`"}`)
		out, _, err := runNovel(t, "alerts", "digest", "--since", "24h", "--by", "severity")
		if err != nil {
			t.Fatalf("alerts digest: %v", err)
		}
		var rows []map[string]any
		assertJSON(t, out, &rows)
		if len(rows) != 1 || rows[0]["group"] != "error" {
			t.Fatalf("want one error group, got %v", rows)
		}
		if rows[0]["active"].(float64) != 1 {
			t.Errorf("active = %v, want 1", rows[0]["active"])
		}
	})
}
