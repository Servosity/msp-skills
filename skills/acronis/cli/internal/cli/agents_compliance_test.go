// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestComputeCompliance(t *testing.T) {
	t.Run("distribution, modal, and below-target flagging", func(t *testing.T) {
		db := newNovelTestStore(t)
		seedTenant(t, db, "t1", "Acme Corp", "customer", "")
		for _, a := range []struct{ id, ver string }{
			{"a1", "15.0.1"}, {"a2", "15.0.1"}, {"a3", "14.0.0"},
		} {
			mustExec(t, db, `INSERT INTO agent_manager(id, data, tenant_id, hostname, version) VALUES(?,?,?,?,?)`,
				a.id, `{}`, "t1", "host-"+a.id, a.ver)
		}

		rep, err := computeCompliance(db, "15.0.0")
		if err != nil {
			t.Fatalf("computeCompliance: %v", err)
		}
		if rep.TotalAgents != 3 {
			t.Fatalf("total = %d, want 3", rep.TotalAgents)
		}
		if rep.ModalVersion != "15.0.1" {
			t.Fatalf("modal = %q, want 15.0.1", rep.ModalVersion)
		}
		if len(rep.NonCompliant) != 1 || rep.NonCompliant[0].Version != "14.0.0" {
			t.Fatalf("non-compliant = %+v, want one 14.0.0", rep.NonCompliant)
		}
	})

	t.Run("empty store: zero agents, empty distribution, no error", func(t *testing.T) {
		db := newNovelTestStore(t)
		rep, err := computeCompliance(db, "15.0.0")
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if rep.TotalAgents != 0 || len(rep.Distribution) != 0 || len(rep.NonCompliant) != 0 {
			t.Fatalf("expected empty report, got %+v", rep)
		}
	})
}

func TestAgentsComplianceCommand_JSON(t *testing.T) {
	db := newNovelTestStore(t)
	seedTenant(t, db, "t1", "Acme Corp", "customer", "")
	mustExec(t, db, `INSERT INTO agent_manager(id, data, tenant_id, hostname, version) VALUES(?,?,?,?,?)`,
		"a1", `{}`, "t1", "h1", "14.0.0")

	cmd := newNovelAgentsComplianceCmd(&rootFlags{asJSON: true})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--db", dbPathOf(t, db), "--target", "15.0.0"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	var rep complianceReport
	if err := json.Unmarshal(out.Bytes(), &rep); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out.String())
	}
	if rep.TotalAgents != 1 || len(rep.NonCompliant) != 1 {
		t.Fatalf("got %+v", rep)
	}

	// empty store
	db2 := newNovelTestStore(t)
	cmd2 := newNovelAgentsComplianceCmd(&rootFlags{asJSON: true})
	var out2 bytes.Buffer
	cmd2.SetOut(&out2)
	cmd2.SetArgs([]string{"--db", dbPathOf(t, db2)})
	if err := cmd2.Execute(); err != nil {
		t.Fatalf("execute empty: %v", err)
	}
	var rep2 complianceReport
	if err := json.Unmarshal(out2.Bytes(), &rep2); err != nil {
		t.Fatalf("empty not valid JSON: %v\n%s", err, out2.String())
	}
	if rep2.TotalAgents != 0 {
		t.Fatalf("expected 0 agents, got %+v", rep2)
	}
}
