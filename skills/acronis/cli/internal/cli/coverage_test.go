// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"
)

func TestComputeCoverage(t *testing.T) {
	now := time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC)
	recent := now.Add(-1 * time.Hour).Format(time.RFC3339)

	t.Run("protected vs paying-but-unprotected", func(t *testing.T) {
		db := newNovelTestStore(t)
		seedTenant(t, db, "t1", "Protected Co", "customer", "")
		seedTenant(t, db, "t2", "Exposed Co", "customer", "")
		// t1 pays, has online agent + recent success -> protected
		mustExec(t, db, `INSERT INTO offering_items(id, tenants_id, data) VALUES(?,?,?)`, "o1", "t1", `{"name":"x"}`)
		mustExec(t, db, `INSERT INTO agent_manager(id, data, tenant_id, status) VALUES(?,?,?,?)`, "a1", `{}`, "t1", "online")
		mustExec(t, db, `INSERT INTO task_manager(id, data, tenant_id, state, result_code, created_at) VALUES(?,?,?,?,?,?)`,
			"k1", `{}`, "t1", "completed", "ok", recent)
		// t2 pays but no online agent and no success -> unprotected
		mustExec(t, db, `INSERT INTO offering_items(id, tenants_id, data) VALUES(?,?,?)`, "o2", "t2", `{"name":"y"}`)

		all, err := computeCoverage(db, 7, false, now)
		if err != nil {
			t.Fatalf("computeCoverage: %v", err)
		}
		byT := map[string]coverageRow{}
		for _, r := range all {
			byT[r.TenantID] = r
		}
		if !byT["t1"].Protected {
			t.Fatalf("t1 should be protected: %+v", byT["t1"])
		}
		if byT["t2"].Protected {
			t.Fatalf("t2 should be unprotected: %+v", byT["t2"])
		}

		// --unprotected filter
		un, err := computeCoverage(db, 7, true, now)
		if err != nil {
			t.Fatalf("computeCoverage unprotected: %v", err)
		}
		if len(un) != 1 || un[0].TenantID != "t2" {
			t.Fatalf("unprotected-only = %+v, want only t2", un)
		}
	})

	t.Run("empty store, no rows, no error", func(t *testing.T) {
		db := newNovelTestStore(t)
		rows, err := computeCoverage(db, 7, false, now)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if len(rows) != 0 {
			t.Fatalf("expected 0, got %d", len(rows))
		}
	})
}

func TestCoverageCommand_UnprotectedJSON(t *testing.T) {
	db := newNovelTestStore(t)
	seedTenant(t, db, "t2", "Exposed Co", "customer", "")
	mustExec(t, db, `INSERT INTO offering_items(id, tenants_id, data) VALUES(?,?,?)`, "o2", "t2", `{"name":"y"}`)

	cmd := newNovelCoverageCmd(&rootFlags{asJSON: true})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--db", dbPathOf(t, db), "--unprotected"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	var got []coverageRow
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out.String())
	}
	if len(got) != 1 || got[0].TenantID != "t2" || got[0].Protected {
		t.Fatalf("got %+v", got)
	}

	// empty
	db2 := newNovelTestStore(t)
	cmd2 := newNovelCoverageCmd(&rootFlags{asJSON: true})
	var out2 bytes.Buffer
	cmd2.SetOut(&out2)
	cmd2.SetArgs([]string{"--db", dbPathOf(t, db2)})
	if err := cmd2.Execute(); err != nil {
		t.Fatalf("execute empty: %v", err)
	}
	var got2 []coverageRow
	if err := json.Unmarshal(out2.Bytes(), &got2); err != nil {
		t.Fatalf("empty not valid JSON: %v\n%s", err, out2.String())
	}
	if len(got2) != 0 {
		t.Fatalf("expected empty, got %+v", got2)
	}
}
