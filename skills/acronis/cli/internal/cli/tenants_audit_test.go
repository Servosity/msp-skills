// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"strings"
	"testing"
)

func TestComputeTenantAudit(t *testing.T) {
	t.Run("flags gaps on enabled customer tenants only", func(t *testing.T) {
		db := newNovelTestStore(t)
		// complete tenant
		seedTenant(t, db, "t-full", "Full Co", "customer", "")
		mustExec(t, db, `INSERT INTO users(id, tenants_id, data) VALUES(?,?,?)`, "u1", "t-full", `{}`)
		mustExec(t, db, `INSERT INTO offering_items(id, tenants_id, data) VALUES(?,?,?)`, "o1", "t-full", `{"name":"x"}`)
		mustExec(t, db, `INSERT INTO agent_manager(id, data, tenant_id, status) VALUES(?,?,?,?)`, "a1", `{}`, "t-full", "online")
		mustExec(t, db, `INSERT INTO clients(id, data, tenant_id) VALUES(?,?,?)`, "c1", `{}`, "t-full")
		// tenant missing agents + clients
		seedTenant(t, db, "t-gap", "Gap Co", "customer", "")
		mustExec(t, db, `INSERT INTO users(id, tenants_id, data) VALUES(?,?,?)`, "u2", "t-gap", `{}`)
		mustExec(t, db, `INSERT INTO offering_items(id, tenants_id, data) VALUES(?,?,?)`, "o2", "t-gap", `{"name":"y"}`)
		// partner kind -> excluded by default kind filter
		seedTenant(t, db, "t-partner", "Partner Co", "partner", "")
		// disabled tenant -> always excluded
		mustExec(t, db, `INSERT INTO tenants(id, data, name, kind, enabled) VALUES(?,?,?,?,?)`,
			"t-off", `{}`, "Off Co", "customer", 0)

		rows, err := computeTenantAudit(db, "customer", false)
		if err != nil {
			t.Fatalf("computeTenantAudit: %v", err)
		}
		if len(rows) != 1 {
			t.Fatalf("want exactly t-gap flagged, got %+v", rows)
		}
		r := rows[0]
		if r.TenantID != "t-gap" {
			t.Fatalf("want t-gap, got %+v", r)
		}
		missing := strings.Join(r.Missing, ",")
		if missing != "agents,oauth_clients" {
			t.Fatalf("want missing agents,oauth_clients, got %q", missing)
		}
	})

	t.Run("includeComplete returns conforming tenants with empty missing", func(t *testing.T) {
		db := newNovelTestStore(t)
		seedTenant(t, db, "t-full", "Full Co", "customer", "")
		mustExec(t, db, `INSERT INTO users(id, tenants_id, data) VALUES(?,?,?)`, "u1", "t-full", `{}`)
		mustExec(t, db, `INSERT INTO offering_items(id, tenants_id, data) VALUES(?,?,?)`, "o1", "t-full", `{"name":"x"}`)
		mustExec(t, db, `INSERT INTO agent_manager(id, data, tenant_id, status) VALUES(?,?,?,?)`, "a1", `{}`, "t-full", "online")
		mustExec(t, db, `INSERT INTO clients(id, data, tenant_id) VALUES(?,?,?)`, "c1", `{}`, "t-full")

		rows, err := computeTenantAudit(db, "customer", true)
		if err != nil {
			t.Fatalf("computeTenantAudit: %v", err)
		}
		if len(rows) != 1 || len(rows[0].Missing) != 0 {
			t.Fatalf("want 1 conforming row with empty missing, got %+v", rows)
		}
	})

	t.Run("empty kind audits all kinds", func(t *testing.T) {
		db := newNovelTestStore(t)
		seedTenant(t, db, "t-partner", "Partner Co", "partner", "")

		rows, err := computeTenantAudit(db, "", false)
		if err != nil {
			t.Fatalf("computeTenantAudit: %v", err)
		}
		if len(rows) != 1 || rows[0].TenantID != "t-partner" {
			t.Fatalf("want partner tenant flagged when kind filter empty, got %+v", rows)
		}
	})
}
