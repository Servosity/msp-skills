// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
package cli

import (
	"testing"
)

func seedFleetFixture(t *testing.T) string {
	db, path := newTestStore(t)
	seed(t, db, "tenants", "ten1", `{"id":"ten1","name":"Contoso","kind":"o365","org_id":"org1"}`)
	seed(t, db, "tenants", "ten2", `{"id":"ten2","name":"Fabrikam","kind":"gsuite","org_id":"org1"}`)
	seed(t, db, "task_stats", "ten1", `{"tenant_id":"ten1","total":{"done":10,"failed":2,"warnings":1},"window_start":"2026-05-30T00:00:00Z","window_end":"2026-06-06T00:00:00Z","fetched_at":"2026-06-06T00:00:00Z"}`)
	seed(t, db, "task_stats", "ten2", `{"tenant_id":"ten2","total":{"done":5,"failed":0,"warnings":0},"window_start":"2026-05-30T00:00:00Z","window_end":"2026-06-06T00:00:00Z","fetched_at":"2026-06-06T00:00:00Z"}`)
	seed(t, db, "quotas", "ten1:storage", `{"tenant_id":"ten1","kind":"storage","used":"99","limit":"100","exceeded":true}`)
	seed(t, db, "quotas", "ten2:storage", `{"tenant_id":"ten2","kind":"storage","used":"10","limit":"100","exceeded":false}`)
	seed(t, db, "resources", "res1", `{"id":"res1","tenant_id":"ten1","kind":"user"}`)
	seed(t, db, "resources", "res2", `{"id":"res2","tenant_id":"ten1","kind":"user"}`)
	seed(t, db, "protections", "prot1", `{"id":"prot1","resource_id":"res1","tenant_id":"ten1"}`)
	return path
}

func TestFleetHealthRollup(t *testing.T) {
	path := seedFleetFixture(t)
	out, err := runNovel(t, newNovelFleetHealthCmd, "--db", path)
	if err != nil {
		t.Fatalf("fleet-health: %v", err)
	}
	view := decodeView[fleetHealthView](t, out)
	if len(view.Tenants) != 2 {
		t.Fatalf("tenants = %d, want 2: %s", len(view.Tenants), out)
	}
	// ten1 sorts first (2 failed tasks).
	r := view.Tenants[0]
	if r.TenantID != "ten1" || r.TasksFailed != 2 || r.TasksDone != 10 {
		t.Errorf("ten1 row wrong: %+v", r)
	}
	if len(r.QuotasExceeded) != 1 || r.QuotasExceeded[0] != "storage" {
		t.Errorf("ten1 quota breach missing: %+v", r.QuotasExceeded)
	}
	if r.Resources != 2 || r.Protected != 1 || r.CoveragePct != 50 {
		t.Errorf("ten1 coverage wrong: %+v", r)
	}
	if view.TotalFailed != 2 || view.TotalExceeded != 1 {
		t.Errorf("totals wrong: failed=%d exceeded=%d", view.TotalFailed, view.TotalExceeded)
	}
}

func TestFleetHealthFailedOnly(t *testing.T) {
	path := seedFleetFixture(t)
	out, err := runNovel(t, newNovelFleetHealthCmd, "--db", path, "--failed-only")
	if err != nil {
		t.Fatalf("fleet-health: %v", err)
	}
	view := decodeView[fleetHealthView](t, out)
	if len(view.Tenants) != 1 || view.Tenants[0].TenantID != "ten1" {
		t.Errorf("--failed-only should keep only ten1: %s", out)
	}
}

func TestFleetHealthTenantFilter(t *testing.T) {
	path := seedFleetFixture(t)
	out, err := runNovel(t, newNovelFleetHealthCmd, "--db", path, "--tenant", "ten2")
	if err != nil {
		t.Fatalf("fleet-health: %v", err)
	}
	view := decodeView[fleetHealthView](t, out)
	if len(view.Tenants) != 1 || view.Tenants[0].TenantID != "ten2" {
		t.Errorf("--tenant filter wrong: %s", out)
	}
}

func TestFleetHealthStaleSnapshotFlag(t *testing.T) {
	path := seedFleetFixture(t)
	// fetched_at 2026-06-06 is long past for a 1h window.
	out, err := runNovel(t, newNovelFleetHealthCmd, "--db", path, "--since", "1h")
	if err != nil {
		t.Fatalf("fleet-health: %v", err)
	}
	view := decodeView[fleetHealthView](t, out)
	stale := 0
	for _, r := range view.Tenants {
		if r.StaleSnapshot {
			stale++
		}
	}
	if stale != 2 {
		t.Errorf("stale snapshots = %d, want 2: %s", stale, out)
	}
}

func TestFleetHealthEmptyStore(t *testing.T) {
	_, path := newTestStore(t)
	out, err := runNovel(t, newNovelFleetHealthCmd, "--db", path)
	if err != nil {
		t.Fatalf("fleet-health: %v", err)
	}
	view := decodeView[fleetHealthView](t, out)
	if len(view.Tenants) != 0 || view.Note == "" {
		t.Errorf("empty store should return empty tenants + note: %s", out)
	}
}
