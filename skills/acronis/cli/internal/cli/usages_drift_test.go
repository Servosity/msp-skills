// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

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

	t.Run("fewer than two snapshots: honest message, no error", func(t *testing.T) {
		db := newNovelTestStore(t)
		seedTenant(t, db, "t1", "Acme Corp", "customer", "")
		mustExec(t, db, `INSERT INTO usages(id, tenants_id, data) VALUES(?,?,?)`,
			"u1", "t1", `{"name":"backup_storage","value":10}`)
		_, _ = captureUsageSnapshot(db, "2026-05-01")

		cmd := newNovelUsagesDriftCmd(&rootFlags{asJSON: false})
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetArgs([]string{"--db", dbPathOf(t, db)})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("execute should not error: %v", err)
		}
		if !strings.Contains(out.String(), "need two snapshots") {
			t.Fatalf("expected honest message, got %q", out.String())
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

	// empty store -> message + empty drift, valid JSON, no error
	db2 := newNovelTestStore(t)
	cmd2 := newNovelUsagesDriftCmd(&rootFlags{asJSON: true})
	var out2 bytes.Buffer
	cmd2.SetOut(&out2)
	cmd2.SetArgs([]string{"--db", dbPathOf(t, db2)})
	if err := cmd2.Execute(); err != nil {
		t.Fatalf("execute empty: %v", err)
	}
	var env2 map[string]any
	if err := json.Unmarshal(out2.Bytes(), &env2); err != nil {
		t.Fatalf("empty not valid JSON: %v\n%s", err, out2.String())
	}
	if _, ok := env2["message"]; !ok {
		t.Fatalf("expected message key on empty, got %v", env2)
	}
}
