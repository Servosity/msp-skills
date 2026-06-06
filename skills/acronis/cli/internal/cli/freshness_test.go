// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"testing"
	"time"
)

func TestComputeFreshness(t *testing.T) {
	now := time.Now()
	fresh := now.Add(-2 * time.Hour).Format(time.RFC3339)
	stale := now.Add(-50 * time.Hour).Format(time.RFC3339)

	t.Run("fresh, stale, and never-successful tenants", func(t *testing.T) {
		db := newNovelTestStore(t)
		seedTenant(t, db, "t-fresh", "Fresh Co", "customer", "")
		seedTenant(t, db, "t-stale", "Stale Co", "customer", "")
		seedTenant(t, db, "t-never", "Never Co", "customer", "")
		mustExec(t, db, `INSERT INTO task_manager(id, data, tenant_id, state, result_code, completed_at) VALUES(?,?,?,?,?,?)`,
			"k1", `{}`, "t-fresh", "completed", "ok", fresh)
		mustExec(t, db, `INSERT INTO task_manager(id, data, tenant_id, state, result_code, completed_at) VALUES(?,?,?,?,?,?)`,
			"k2", `{}`, "t-stale", "completed", "ok", stale)
		// t-never has a failed task only — still "never successful"
		mustExec(t, db, `INSERT INTO task_manager(id, data, tenant_id, state, result_code, completed_at) VALUES(?,?,?,?,?,?)`,
			"k3", `{}`, "t-never", "failed", "error", fresh)

		rows, err := computeFreshness(db, 24*time.Hour, false, "", now)
		if err != nil {
			t.Fatalf("computeFreshness: %v", err)
		}
		got := map[string]freshnessRow{}
		for _, r := range rows {
			got[r.TenantID] = r
		}
		if len(got) != 3 {
			t.Fatalf("want 3 tenants, got %d: %+v", len(got), rows)
		}
		if got["t-fresh"].Breached {
			t.Fatalf("t-fresh should not be breached: %+v", got["t-fresh"])
		}
		if !got["t-stale"].Breached {
			t.Fatalf("t-stale should be breached: %+v", got["t-stale"])
		}
		if !got["t-never"].Breached || got["t-never"].LastSuccess != "" {
			t.Fatalf("t-never should be breached with no last_success: %+v", got["t-never"])
		}
		// breached first ordering
		if !rows[0].Breached {
			t.Fatalf("breached rows should sort first: %+v", rows)
		}
	})

	t.Run("breachedOnly filter", func(t *testing.T) {
		db := newNovelTestStore(t)
		seedTenant(t, db, "t-fresh", "Fresh Co", "customer", "")
		seedTenant(t, db, "t-never", "Never Co", "customer", "")
		mustExec(t, db, `INSERT INTO task_manager(id, data, tenant_id, state, result_code, completed_at) VALUES(?,?,?,?,?,?)`,
			"k1", `{}`, "t-fresh", "completed", "ok", fresh)

		rows, err := computeFreshness(db, 24*time.Hour, true, "", now)
		if err != nil {
			t.Fatalf("computeFreshness: %v", err)
		}
		if len(rows) != 1 || rows[0].TenantID != "t-never" {
			t.Fatalf("breachedOnly should return only t-never: %+v", rows)
		}
	})

	t.Run("empty store yields empty slice", func(t *testing.T) {
		db := newNovelTestStore(t)
		rows, err := computeFreshness(db, 24*time.Hour, false, "", now)
		if err != nil {
			t.Fatalf("computeFreshness: %v", err)
		}
		if len(rows) != 0 {
			t.Fatalf("want 0 rows on empty store, got %d", len(rows))
		}
	})
}
