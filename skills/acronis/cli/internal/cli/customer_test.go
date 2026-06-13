// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"strings"
	"testing"
	"time"
)

func TestComputeCustomerCard(t *testing.T) {
	// Use UTC so the inserted timestamps and the date-prefix assertion below
	// agree with computeCustomerCard's UTC-normalized output. With local time,
	// an evening run in a behind-UTC zone (e.g. EDT) shifts recent[:10] to the
	// previous calendar day while the card reports the UTC day — a false fail.
	now := time.Now().UTC()
	recent := now.Add(-2 * time.Hour).Format(time.RFC3339)
	old := now.Add(-30 * 24 * time.Hour).Format(time.RFC3339)

	t.Run("joins all tables for one tenant", func(t *testing.T) {
		db := newNovelTestStore(t)
		seedTenant(t, db, "t1", "Acme", "customer", "parent-1")
		seedTenant(t, db, "t1-child", "Acme Unit", "unit", "t1")
		mustExec(t, db, `INSERT INTO users(id, tenants_id, data) VALUES(?,?,?)`, "u1", "t1", `{}`)
		mustExec(t, db, `INSERT INTO offering_items(id, tenants_id, data) VALUES(?,?,?)`, "o1", "t1", `{"name":"backup"}`)
		mustExec(t, db, `INSERT INTO usages(id, tenants_id, data) VALUES(?,?,?)`, "us1", "t1", `{"name":"storage","value":5}`)
		mustExec(t, db, `INSERT INTO clients(id, data, tenant_id) VALUES(?,?,?)`, "c1", `{}`, "t1")
		mustExec(t, db, `INSERT INTO agent_manager(id, data, tenant_id, status, last_seen) VALUES(?,?,?,?,?)`,
			"a1", `{}`, "t1", "online", recent)
		mustExec(t, db, `INSERT INTO agent_manager(id, data, tenant_id, status) VALUES(?,?,?,?)`,
			"a2", `{}`, "t1", "offline")
		// recent ok + recent failed + old ok (old ok is outside 7d but counts for last_success ordering check)
		mustExec(t, db, `INSERT INTO task_manager(id, data, tenant_id, state, result_code, completed_at) VALUES(?,?,?,?,?,?)`,
			"k1", `{}`, "t1", "completed", "ok", recent)
		mustExec(t, db, `INSERT INTO task_manager(id, data, tenant_id, state, result_code, completed_at) VALUES(?,?,?,?,?,?)`,
			"k2", `{}`, "t1", "failed", "error", recent)
		mustExec(t, db, `INSERT INTO task_manager(id, data, tenant_id, state, result_code, completed_at) VALUES(?,?,?,?,?,?)`,
			"k3", `{}`, "t1", "completed", "ok", old)
		// another tenant's rows must not leak in
		seedTenant(t, db, "t2", "Globex", "customer", "")
		mustExec(t, db, `INSERT INTO users(id, tenants_id, data) VALUES(?,?,?)`, "u2", "t2", `{}`)

		card, err := computeCustomerCard(db, "t1", now)
		if err != nil {
			t.Fatalf("computeCustomerCard: %v", err)
		}
		if card.Name != "Acme" || card.Kind != "customer" || card.ParentID != "parent-1" {
			t.Fatalf("tenant fields wrong: %+v", card)
		}
		if card.ChildTenants != 1 || card.Users != 1 || card.OfferingItems != 1 || card.UsageRecords != 1 || card.OAuthClients != 1 {
			t.Fatalf("counts wrong: %+v", card)
		}
		if card.AgentsTotal != 2 || card.AgentsOnline != 1 || card.LastAgentSeen == "" {
			t.Fatalf("agent stats wrong: %+v", card)
		}
		if card.TasksTotal7d != 2 || card.TasksOK7d != 1 || card.TasksFailed7d != 1 {
			t.Fatalf("7d task stats wrong: %+v", card)
		}
		if card.LastSuccess == "" || !strings.HasPrefix(card.LastSuccess, recent[:10]) {
			t.Fatalf("last_success should be the recent ok task: %+v", card)
		}
	})

	t.Run("unknown tenant errors", func(t *testing.T) {
		db := newNovelTestStore(t)
		if _, err := computeCustomerCard(db, "nope", now); err == nil {
			t.Fatal("want error for unknown tenant")
		}
	})
}
