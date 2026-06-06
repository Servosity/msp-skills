// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"
)

func TestComputeAlertsRepeat(t *testing.T) {
	now := time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC)
	day := func(d int) string { return now.Add(-time.Duration(d) * 24 * time.Hour).Format(time.RFC3339) }

	t.Run("ranks by distinct fail days, backup-typed preferred", func(t *testing.T) {
		db := newNovelTestStore(t)
		seedTenant(t, db, "t1", "Acme Corp", "customer", "")
		// resource r1: backup failures on two distinct days
		mustExec(t, db, `INSERT INTO task_manager(id, data, resource_id, tenant_id, state, result_code, type, created_at) VALUES(?,?,?,?,?,?,?,?)`,
			"k1", `{}`, "r1", "t1", "failed", "error", "backup", day(1))
		mustExec(t, db, `INSERT INTO task_manager(id, data, resource_id, tenant_id, state, result_code, type, created_at) VALUES(?,?,?,?,?,?,?,?)`,
			"k2", `{}`, "r1", "t1", "failed", "error", "backup", day(3))
		// same day repeat should not double-count
		mustExec(t, db, `INSERT INTO task_manager(id, data, resource_id, tenant_id, state, result_code, type, created_at) VALUES(?,?,?,?,?,?,?,?)`,
			"k3", `{}`, "r1", "t1", "failed", "error", "backup", day(3))
		// resource r2: one backup failure
		mustExec(t, db, `INSERT INTO task_manager(id, data, resource_id, tenant_id, state, result_code, type, created_at) VALUES(?,?,?,?,?,?,?,?)`,
			"k4", `{}`, "r2", "t1", "failed", "error", "backup", day(2))
		// a successful task should be ignored
		mustExec(t, db, `INSERT INTO task_manager(id, data, resource_id, tenant_id, state, result_code, type, created_at) VALUES(?,?,?,?,?,?,?,?)`,
			"k5", `{}`, "r3", "t1", "completed", "ok", "backup", day(1))

		rows, err := computeAlertsRepeat(db, 14, now)
		if err != nil {
			t.Fatalf("computeAlertsRepeat: %v", err)
		}
		if len(rows) != 2 {
			t.Fatalf("want 2 ranked resources, got %+v", rows)
		}
		if rows[0].ResourceID != "r1" || rows[0].DistinctFailDay != 2 {
			t.Fatalf("top = %+v, want r1 with 2 fail days", rows[0])
		}
		if rows[0].TenantName != "Acme Corp" {
			t.Fatalf("expected tenant name joined, got %q", rows[0].TenantName)
		}
	})

	t.Run("empty store: no rows, no error", func(t *testing.T) {
		db := newNovelTestStore(t)
		rows, err := computeAlertsRepeat(db, 14, now)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if len(rows) != 0 {
			t.Fatalf("expected 0, got %d", len(rows))
		}
	})
}

func TestAlertsRepeatCommand_JSON(t *testing.T) {
	now := time.Now()
	db := newNovelTestStore(t)
	seedTenant(t, db, "t1", "Acme Corp", "customer", "")
	mustExec(t, db, `INSERT INTO task_manager(id, data, resource_id, tenant_id, state, result_code, type, created_at) VALUES(?,?,?,?,?,?,?,?)`,
		"k1", `{}`, "r1", "t1", "failed", "error", "backup", now.Add(-24*time.Hour).Format(time.RFC3339))

	cmd := newNovelAlertsRepeatCmd(&rootFlags{asJSON: true})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--db", dbPathOf(t, db), "--days", "14"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	var got []repeatRow
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out.String())
	}
	if len(got) != 1 || got[0].ResourceID != "r1" || got[0].DistinctFailDay != 1 {
		t.Fatalf("got %+v", got)
	}

	// empty
	db2 := newNovelTestStore(t)
	cmd2 := newNovelAlertsRepeatCmd(&rootFlags{asJSON: true})
	var out2 bytes.Buffer
	cmd2.SetOut(&out2)
	cmd2.SetArgs([]string{"--db", dbPathOf(t, db2)})
	if err := cmd2.Execute(); err != nil {
		t.Fatalf("execute empty: %v", err)
	}
	var got2 []repeatRow
	if err := json.Unmarshal(out2.Bytes(), &got2); err != nil {
		t.Fatalf("empty not valid JSON: %v\n%s", err, out2.String())
	}
	if len(got2) != 0 {
		t.Fatalf("expected empty, got %+v", got2)
	}
}
