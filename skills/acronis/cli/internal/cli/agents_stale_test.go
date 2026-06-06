// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"
)

func TestComputeStaleAgents(t *testing.T) {
	now := time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC)
	recent := now.Add(-1 * time.Hour).Format(time.RFC3339)
	old := now.Add(-30 * 24 * time.Hour).Format(time.RFC3339)

	t.Run("flags old last_seen and offline status", func(t *testing.T) {
		db := newNovelTestStore(t)
		seedTenant(t, db, "t1", "Acme Corp", "customer", "")
		// fresh agent — not stale
		mustExec(t, db, `INSERT INTO agent_manager(id, data, tenant_id, hostname, last_seen, status, version) VALUES(?,?,?,?,?,?,?)`,
			"a1", `{}`, "t1", "host-fresh", recent, "online", "15.0.1")
		// old last_seen — stale
		mustExec(t, db, `INSERT INTO agent_manager(id, data, tenant_id, hostname, last_seen, status, version) VALUES(?,?,?,?,?,?,?)`,
			"a2", `{}`, "t1", "host-old", old, "online", "14.0.0")
		// recent but offline — stale
		mustExec(t, db, `INSERT INTO agent_manager(id, data, tenant_id, hostname, last_seen, status, version) VALUES(?,?,?,?,?,?,?)`,
			"a3", `{}`, "t1", "host-offline", recent, "offline", "15.0.1")

		rows, err := computeStaleAgents(db, 7*24*time.Hour, now)
		if err != nil {
			t.Fatalf("computeStaleAgents: %v", err)
		}
		got := map[string]staleAgentRow{}
		for _, r := range rows {
			got[r.AgentID] = r
		}
		if _, ok := got["a1"]; ok {
			t.Fatalf("a1 should not be stale: %+v", got)
		}
		if _, ok := got["a2"]; !ok {
			t.Fatalf("a2 (old) should be stale")
		}
		if _, ok := got["a3"]; !ok {
			t.Fatalf("a3 (offline) should be stale")
		}
		if got["a2"].TenantName != "Acme Corp" {
			t.Fatalf("expected tenant name joined, got %q", got["a2"].TenantName)
		}
	})

	t.Run("empty store, no stale agents, no error", func(t *testing.T) {
		db := newNovelTestStore(t)
		rows, err := computeStaleAgents(db, 7*24*time.Hour, now)
		if err != nil {
			t.Fatalf("err on empty: %v", err)
		}
		if len(rows) != 0 {
			t.Fatalf("expected 0, got %d", len(rows))
		}
	})
}

func TestAgentsStaleCommand_JSON(t *testing.T) {
	db := newNovelTestStore(t)
	seedTenant(t, db, "t1", "Acme Corp", "customer", "")
	old := time.Now().Add(-30 * 24 * time.Hour).Format(time.RFC3339)
	mustExec(t, db, `INSERT INTO agent_manager(id, data, tenant_id, hostname, last_seen, status, version) VALUES(?,?,?,?,?,?,?)`,
		"a2", `{}`, "t1", "host-old", old, "online", "14.0.0")

	flags := &rootFlags{asJSON: true}
	cmd := newNovelAgentsStaleCmd(flags)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--db", dbPathOf(t, db), "--older-than", "7d"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	var got []staleAgentRow
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out.String())
	}
	if len(got) != 1 || got[0].Hostname != "host-old" {
		t.Fatalf("got %+v", got)
	}

	// Empty store yields []
	db2 := newNovelTestStore(t)
	cmd2 := newNovelAgentsStaleCmd(&rootFlags{asJSON: true})
	var out2 bytes.Buffer
	cmd2.SetOut(&out2)
	cmd2.SetArgs([]string{"--db", dbPathOf(t, db2)})
	if err := cmd2.Execute(); err != nil {
		t.Fatalf("execute empty: %v", err)
	}
	var got2 []staleAgentRow
	if err := json.Unmarshal(out2.Bytes(), &got2); err != nil {
		t.Fatalf("empty not valid JSON: %v\n%s", err, out2.String())
	}
	if len(got2) != 0 {
		t.Fatalf("expected empty, got %+v", got2)
	}
}
