// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestBuildTenantForest(t *testing.T) {
	t.Run("hierarchy with per-node counts", func(t *testing.T) {
		db := newNovelTestStore(t)
		seedTenant(t, db, "root", "Partner", "partner", "")
		seedTenant(t, db, "cust", "Customer A", "customer", "root")
		seedTenant(t, db, "unit", "Unit 1", "unit", "cust")
		// counts on the unit
		mustExec(t, db, `INSERT INTO agent_manager(id, data, tenant_id) VALUES(?,?,?)`, "a1", `{}`, "unit")
		mustExec(t, db, `INSERT INTO agent_manager(id, data, tenant_id) VALUES(?,?,?)`, "a2", `{}`, "unit")
		mustExec(t, db, `INSERT INTO users(id, tenants_id, data) VALUES(?,?,?)`, "us1", "unit", `{}`)

		roots, err := buildTenantForest(db)
		if err != nil {
			t.Fatalf("buildTenantForest: %v", err)
		}
		if len(roots) != 1 || roots[0].ID != "root" {
			t.Fatalf("roots = %+v, want single root", roots)
		}
		r := roots[0]
		if len(r.Children) != 1 || r.Children[0].ID != "cust" {
			t.Fatalf("root children = %+v", r.Children)
		}
		unit := r.Children[0].Children[0]
		if unit.ID != "unit" || unit.AgentCount != 2 || unit.UserCount != 1 {
			t.Fatalf("unit = %+v, want agents=2 users=1", unit)
		}
	})

	t.Run("empty store: no roots, no error", func(t *testing.T) {
		db := newNovelTestStore(t)
		roots, err := buildTenantForest(db)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if len(roots) != 0 {
			t.Fatalf("expected 0 roots, got %d", len(roots))
		}
	})
}

func TestTreeRenderText(t *testing.T) {
	db := newNovelTestStore(t)
	seedTenant(t, db, "root", "Partner", "partner", "")
	seedTenant(t, db, "cust", "Customer A", "customer", "root")
	roots, err := buildTenantForest(db)
	if err != nil {
		t.Fatalf("buildTenantForest: %v", err)
	}
	var sb strings.Builder
	for _, r := range roots {
		renderTree(&sb, r, 0, 0)
	}
	got := sb.String()
	if !strings.Contains(got, "Partner (partner)") || !strings.Contains(got, "  Customer A (customer)") {
		t.Fatalf("unexpected tree text:\n%s", got)
	}
}

func TestTreeCommand_JSON(t *testing.T) {
	db := newNovelTestStore(t)
	seedTenant(t, db, "root", "Partner", "partner", "")
	seedTenant(t, db, "cust", "Customer A", "customer", "root")

	// JSON output
	cmdJSON := newNovelTreeCmd(&rootFlags{asJSON: true})
	var outJSON bytes.Buffer
	cmdJSON.SetOut(&outJSON)
	cmdJSON.SetArgs([]string{"--db", dbPathOf(t, db)})
	if err := cmdJSON.Execute(); err != nil {
		t.Fatalf("execute json: %v", err)
	}
	var roots []treeNode
	if err := json.Unmarshal(outJSON.Bytes(), &roots); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, outJSON.String())
	}
	if len(roots) != 1 || len(roots[0].Children) != 1 {
		t.Fatalf("got %+v", roots)
	}

	// empty store -> []
	db2 := newNovelTestStore(t)
	cmd2 := newNovelTreeCmd(&rootFlags{asJSON: true})
	var out2 bytes.Buffer
	cmd2.SetOut(&out2)
	cmd2.SetArgs([]string{"--db", dbPathOf(t, db2)})
	if err := cmd2.Execute(); err != nil {
		t.Fatalf("execute empty: %v", err)
	}
	var roots2 []treeNode
	if err := json.Unmarshal(out2.Bytes(), &roots2); err != nil {
		t.Fatalf("empty not valid JSON: %v\n%s", err, out2.String())
	}
	if len(roots2) != 0 {
		t.Fatalf("expected empty, got %+v", roots2)
	}
}
