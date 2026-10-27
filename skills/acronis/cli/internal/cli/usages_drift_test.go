// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestUsagesSnapshotAndDrift(t *testing.T) {
	t.Run("two snapshots produce a delta", func(t *testing.T) {
		db := newNovelTestStore(t)
		seedTenant(t, db, "t1", "Acme Corp", "customer", "")

		// First snapshot: value 10.
		mustExec(t, db, `INSERT INTO usages(id, tenants_id, data) VALUES(?,?,?)`,
			"u1", "t1", `{"name":"backup_storage","value":10}`)
		if n, err := captureUsageSnapshot(db, "2026-05-01"); err != nil || n != 1 {
			t.Fatalf("snapshot1 n=%d err=%v", n, err)
		}

		// Usage grows to 25; re-snapshot a later date.
		mustExec(t, db, `UPDATE usages SET data=? WHERE id=?`, `{"name":"backup_storage","value":25}`, "u1")
		if n, err := captureUsageSnapshot(db, "2026-05-28"); err != nil || n != 1 {
			t.Fatalf("snapshot2 n=%d err=%v", n, err)
		}

		rows, err := computeDrift(db, "2026-05-01", "2026-05-28", "")
		if err != nil {
			t.Fatalf("computeDrift: %v", err)
		}
		if len(rows) != 1 {
			t.Fatalf("want 1 drift row, got %+v", rows)
		}
		r := rows[0]
		if r.FromValue == nil || *r.FromValue != 10 || r.ToValue == nil || *r.ToValue != 25 || r.Delta == nil || *r.Delta != 15 {
			t.Fatalf("drift row = %+v, want from=10 to=25 delta=15", r)
		}
	})

	t.Run("fewer than two snapshots: an error, never an empty comparison", func(t *testing.T) {
		db := newNovelTestStore(t)
		seedTenant(t, db, "t1", "Acme Corp", "customer", "")
		mustExec(t, db, `INSERT INTO usages(id, tenants_id, data) VALUES(?,?,?)`,
			"u1", "t1", `{"name":"backup_storage","value":10}`)
		_, _ = captureUsageSnapshot(db, "2026-05-01")

		cmd := newNovelUsagesDriftCmd(&rootFlags{asJSON: false})
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetArgs([]string{"--db", dbPathOf(t, db)})
		err := cmd.Execute()
		if err == nil || !strings.Contains(err.Error(), "need two usage snapshots") {
			t.Fatalf("want a need-two-snapshots error, got %v (out %q)", err, out.String())
		}
	})
}

func TestUsagesDriftCommand_JSON(t *testing.T) {
	db := newNovelTestStore(t)
	seedTenant(t, db, "t1", "Acme Corp", "customer", "")
	mustExec(t, db, `INSERT INTO usages(id, tenants_id, data) VALUES(?,?,?)`,
		"u1", "t1", `{"name":"backup_storage","value":10}`)
	_, _ = captureUsageSnapshot(db, "2026-05-01")
	mustExec(t, db, `UPDATE usages SET data=? WHERE id=?`, `{"name":"backup_storage","value":25}`, "u1")
	_, _ = captureUsageSnapshot(db, "2026-05-28")

	cmd := newNovelUsagesDriftCmd(&rootFlags{asJSON: true})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--db", dbPathOf(t, db), "--from", "2026-05-01", "--to", "2026-05-28"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	var env struct {
		From  string     `json:"from"`
		To    string     `json:"to"`
		Drift []driftRow `json:"drift"`
	}
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out.String())
	}
	if len(env.Drift) != 1 || env.Drift[0].Delta == nil || *env.Drift[0].Delta != 15 {
		t.Fatalf("got %+v", env)
	}

	// empty store -> an error, never a successful empty comparison
	db2 := newNovelTestStore(t)
	cmd2 := newNovelUsagesDriftCmd(&rootFlags{asJSON: true})
	var out2 bytes.Buffer
	cmd2.SetOut(&out2)
	cmd2.SetArgs([]string{"--db", dbPathOf(t, db2)})
	if err := cmd2.Execute(); err == nil || !strings.Contains(err.Error(), "need two usage snapshots") {
		t.Fatalf("empty store: want a need-two-snapshots error, got %v (out %q)", err, out2.String())
	}
}
