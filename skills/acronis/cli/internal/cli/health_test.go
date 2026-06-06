// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestComputeHealth(t *testing.T) {
	now := time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC)
	recent := now.Add(-1 * time.Hour).Format(time.RFC3339)
	old := now.Add(-10 * 24 * time.Hour).Format(time.RFC3339)

	t.Run("happy path classifies and flags stale", func(t *testing.T) {
		db := newNovelTestStore(t)
		seedTenant(t, db, "t1", "Acme Corp", "customer", "")
		seedTenant(t, db, "t2", "Globex", "customer", "")
		// t1: one ok recent, one failed recent -> not stale.
		mustExec(t, db, `INSERT INTO task_manager(id, data, tenant_id, state, result_code, created_at) VALUES(?,?,?,?,?,?)`,
			"k1", `{}`, "t1", "completed", "ok", recent)
		mustExec(t, db, `INSERT INTO task_manager(id, data, tenant_id, state, result_code, created_at) VALUES(?,?,?,?,?,?)`,
			"k2", `{}`, "t1", "failed", "error", recent)
		// t2: one failed but only old -> stale.
		mustExec(t, db, `INSERT INTO task_manager(id, data, tenant_id, state, result_code, created_at) VALUES(?,?,?,?,?,?)`,
			"k3", `{}`, "t2", "failed", "error", old)

		rows, err := computeHealth(db, 1, now)
		if err != nil {
			t.Fatalf("computeHealth: %v", err)
		}
		byTenant := map[string]healthRow{}
		for _, r := range rows {
			byTenant[r.TenantID] = r
		}
		t1 := byTenant["t1"]
		if t1.Name != "Acme Corp" || t1.Total != 2 || t1.OK != 1 || t1.Failed != 1 || t1.Stale {
			t.Fatalf("t1 = %+v, want total=2 ok=1 failed=1 stale=false name=Acme Corp", t1)
		}
		t2 := byTenant["t2"]
		if !t2.Stale || t2.Failed != 1 {
			t.Fatalf("t2 = %+v, want stale=true failed=1", t2)
		}
	})

	t.Run("empty store yields empty result, no error", func(t *testing.T) {
		db := newNovelTestStore(t)
		rows, err := computeHealth(db, 1, now)
		if err != nil {
			t.Fatalf("computeHealth on empty: %v", err)
		}
		if len(rows) != 0 {
			t.Fatalf("expected 0 rows on empty store, got %d", len(rows))
		}
	})
}

func TestHealthCommand_JSONOutput(t *testing.T) {
	db := newNovelTestStore(t)
	seedTenant(t, db, "t1", "Acme Corp", "customer", "")
	mustExec(t, db, `INSERT INTO task_manager(id, data, tenant_id, state, result_code, created_at) VALUES(?,?,?,?,?,?)`,
		"k1", `{}`, "t1", "failed", "error", time.Now().Format(time.RFC3339))

	flags := &rootFlags{asJSON: true}
	cmd := newNovelHealthCmd(flags)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--db", dbPathOf(t, db)})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	var got []healthRow
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("output not valid JSON array: %v\n%s", err, out.String())
	}
	if len(got) != 1 || got[0].TenantID != "t1" || got[0].Failed != 1 {
		t.Fatalf("got %+v", got)
	}
	if !strings.Contains(out.String(), "Acme Corp") {
		t.Fatalf("expected tenant name in output, got %s", out.String())
	}
}

func TestHealthCommand_EmptyStoreJSON(t *testing.T) {
	db := newNovelTestStore(t)
	flags := &rootFlags{asJSON: true}
	cmd := newNovelHealthCmd(flags)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--db", dbPathOf(t, db)})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute on empty store errored: %v", err)
	}
	var got []healthRow
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("empty-store output not valid JSON: %v\n%s", err, out.String())
	}
	if len(got) != 0 {
		t.Fatalf("expected empty array, got %+v", got)
	}
}
