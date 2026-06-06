// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"testing"
	"time"
)

func TestComputeFailures(t *testing.T) {
	now := time.Now()
	recent := now.Add(-2 * time.Hour).Format(time.RFC3339)
	old := now.Add(-72 * time.Hour).Format(time.RFC3339)

	t.Run("window and outcome filters", func(t *testing.T) {
		db := newNovelTestStore(t)
		seedTenant(t, db, "t1", "Acme", "customer", "")
		// recent failure -> included
		mustExec(t, db, `INSERT INTO task_manager(id, data, tenant_id, type, state, result_code, created_at) VALUES(?,?,?,?,?,?,?)`,
			"k1", `{}`, "t1", "backup", "failed", "error", recent)
		// old failure -> excluded by window
		mustExec(t, db, `INSERT INTO task_manager(id, data, tenant_id, type, state, result_code, created_at) VALUES(?,?,?,?,?,?,?)`,
			"k2", `{}`, "t1", "backup", "failed", "error", old)
		// recent success -> excluded by outcome
		mustExec(t, db, `INSERT INTO task_manager(id, data, tenant_id, type, state, result_code, created_at) VALUES(?,?,?,?,?,?,?)`,
			"k3", `{}`, "t1", "backup", "completed", "ok", recent)

		rows, err := computeFailures(db, 24*time.Hour, "", now)
		if err != nil {
			t.Fatalf("computeFailures: %v", err)
		}
		if len(rows) != 1 {
			t.Fatalf("want 1 failure, got %d: %+v", len(rows), rows)
		}
		if rows[0].TaskID != "k1" || rows[0].TenantName != "Acme" {
			t.Fatalf("unexpected row: %+v", rows[0])
		}
	})

	t.Run("tenant filter", func(t *testing.T) {
		db := newNovelTestStore(t)
		seedTenant(t, db, "t1", "Acme", "customer", "")
		seedTenant(t, db, "t2", "Globex", "customer", "")
		mustExec(t, db, `INSERT INTO task_manager(id, data, tenant_id, state, result_code, created_at) VALUES(?,?,?,?,?,?)`,
			"k1", `{}`, "t1", "failed", "error", recent)
		mustExec(t, db, `INSERT INTO task_manager(id, data, tenant_id, state, result_code, created_at) VALUES(?,?,?,?,?,?)`,
			"k2", `{}`, "t2", "failed", "error", recent)

		rows, err := computeFailures(db, 24*time.Hour, "t2", now)
		if err != nil {
			t.Fatalf("computeFailures: %v", err)
		}
		if len(rows) != 1 || rows[0].TenantID != "t2" {
			t.Fatalf("tenant filter broken: %+v", rows)
		}
	})

	t.Run("empty store yields empty slice", func(t *testing.T) {
		db := newNovelTestStore(t)
		rows, err := computeFailures(db, 24*time.Hour, "", now)
		if err != nil {
			t.Fatalf("computeFailures: %v", err)
		}
		if len(rows) != 0 {
			t.Fatalf("want 0 rows on empty store, got %d", len(rows))
		}
	})

	t.Run("newest first ordering", func(t *testing.T) {
		db := newNovelTestStore(t)
		seedTenant(t, db, "t1", "Acme", "customer", "")
		older := now.Add(-10 * time.Hour).Format(time.RFC3339)
		newer := now.Add(-1 * time.Hour).Format(time.RFC3339)
		mustExec(t, db, `INSERT INTO task_manager(id, data, tenant_id, state, result_code, created_at) VALUES(?,?,?,?,?,?)`,
			"k-old", `{}`, "t1", "failed", "error", older)
		mustExec(t, db, `INSERT INTO task_manager(id, data, tenant_id, state, result_code, created_at) VALUES(?,?,?,?,?,?)`,
			"k-new", `{}`, "t1", "failed", "error", newer)

		rows, err := computeFailures(db, 24*time.Hour, "", now)
		if err != nil {
			t.Fatalf("computeFailures: %v", err)
		}
		if len(rows) != 2 || rows[0].TaskID != "k-new" {
			t.Fatalf("want k-new first, got %+v", rows)
		}
	})
}
