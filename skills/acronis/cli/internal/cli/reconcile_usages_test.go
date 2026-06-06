// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestComputeReconcile(t *testing.T) {
	t.Run("flags unlicensed usage and idle license", func(t *testing.T) {
		db := newNovelTestStore(t)
		seedTenant(t, db, "t1", "Acme Corp", "customer", "")
		// matched: usage + offering -> ok
		mustExec(t, db, `INSERT INTO usages(id, tenants_id, data) VALUES(?,?,?)`,
			"u1", "t1", `{"name":"backup_storage","value":42}`)
		mustExec(t, db, `INSERT INTO offering_items(id, tenants_id, data) VALUES(?,?,?)`,
			"o1", "t1", `{"name":"backup_storage"}`)
		// usage without offering -> unlicensed_usage
		mustExec(t, db, `INSERT INTO usages(id, tenants_id, data) VALUES(?,?,?)`,
			"u2", "t1", `{"name":"orphan_metric","value":7}`)
		// offering without usage -> idle_license
		mustExec(t, db, `INSERT INTO offering_items(id, tenants_id, data) VALUES(?,?,?)`,
			"o2", "t1", `{"name":"unused_edition"}`)

		rows, err := computeReconcile(db, "")
		if err != nil {
			t.Fatalf("computeReconcile: %v", err)
		}
		findings := map[string]string{}
		for _, r := range rows {
			findings[r.Offering] = r.Finding
		}
		if findings["backup_storage"] != "ok" {
			t.Fatalf("backup_storage finding = %q, want ok", findings["backup_storage"])
		}
		if findings["orphan_metric"] != "unlicensed_usage" {
			t.Fatalf("orphan_metric finding = %q, want unlicensed_usage", findings["orphan_metric"])
		}
		if findings["unused_edition"] != "idle_license" {
			t.Fatalf("unused_edition finding = %q, want idle_license", findings["unused_edition"])
		}
	})

	t.Run("empty store, no rows, no error", func(t *testing.T) {
		db := newNovelTestStore(t)
		rows, err := computeReconcile(db, "")
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if len(rows) != 0 {
			t.Fatalf("expected 0, got %d", len(rows))
		}
	})
}

func TestReconcileUsagesCommand_JSON(t *testing.T) {
	db := newNovelTestStore(t)
	seedTenant(t, db, "t1", "Acme Corp", "customer", "")
	mustExec(t, db, `INSERT INTO usages(id, tenants_id, data) VALUES(?,?,?)`,
		"u2", "t1", `{"name":"orphan_metric","value":7}`)

	cmd := newNovelReconcileUsagesCmd(&rootFlags{asJSON: true})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--db", dbPathOf(t, db)})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	var got []reconcileRow
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out.String())
	}
	if len(got) != 1 || got[0].Finding != "unlicensed_usage" {
		t.Fatalf("got %+v", got)
	}

	// empty
	db2 := newNovelTestStore(t)
	cmd2 := newNovelReconcileUsagesCmd(&rootFlags{asJSON: true})
	var out2 bytes.Buffer
	cmd2.SetOut(&out2)
	cmd2.SetArgs([]string{"--db", dbPathOf(t, db2)})
	if err := cmd2.Execute(); err != nil {
		t.Fatalf("execute empty: %v", err)
	}
	var got2 []reconcileRow
	if err := json.Unmarshal(out2.Bytes(), &got2); err != nil {
		t.Fatalf("empty not valid JSON: %v\n%s", err, out2.String())
	}
	if len(got2) != 0 {
		t.Fatalf("expected empty, got %+v", got2)
	}
}
