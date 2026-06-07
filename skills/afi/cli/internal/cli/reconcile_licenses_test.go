// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
package cli

import (
	"testing"
)

func seedReconcileFixture(t *testing.T) string {
	db, path := newTestStore(t)
	seed(t, db, "tenants", "ten1", `{"id":"ten1","name":"Contoso","org_id":"org1"}`)
	seed(t, db, "tenants", "ten2", `{"id":"ten2","name":"Fabrikam","org_id":"org1"}`)
	seed(t, db, "tenants", "ten3", `{"id":"ten3","name":"Initech","org_id":"org2"}`)
	// ten1: licensed 25 (resource) + a storage item that must be ignored; 1 protected -> over-provisioned
	seed(t, db, "subscriptions", "sub1", `{"id":"sub1","org_id":"org1","tenant_id":"ten1","status":"active","items":[{"kind":"resource","qty":"25"},{"kind":"storage","qty":"100"}]}`)
	// ten2: licensed 1; 2 protected -> under-licensed
	seed(t, db, "subscriptions", "sub2", `{"id":"sub2","org_id":"org1","tenant_id":"ten2","status":"active","items":[{"kind":"resource","qty":"1"}]}`)
	// ten3: licensed 1; 1 protected -> aligned (different org)
	seed(t, db, "subscriptions", "sub3", `{"id":"sub3","org_id":"org2","tenant_id":"ten3","status":"active","items":[{"kind":"resource","qty":"1"}]}`)
	seed(t, db, "protections", "p1", `{"id":"p1","resource_id":"r1","tenant_id":"ten1"}`)
	seed(t, db, "protections", "p2a", `{"id":"p2a","resource_id":"r2","tenant_id":"ten2"}`)
	seed(t, db, "protections", "p2b", `{"id":"p2b","resource_id":"r3","tenant_id":"ten2"}`)
	seed(t, db, "protections", "p3", `{"id":"p3","resource_id":"r4","tenant_id":"ten3"}`)
	return path
}

func TestReconcileLicensesVerdicts(t *testing.T) {
	path := seedReconcileFixture(t)
	out, err := runNovel(t, newNovelReconcileLicensesCmd, "--db", path)
	if err != nil {
		t.Fatalf("reconcile-licenses: %v", err)
	}
	view := decodeView[reconcileView](t, out)
	if len(view.Tenants) != 3 {
		t.Fatalf("rows = %d, want 3: %s", len(view.Tenants), out)
	}
	byTenant := map[string]reconcileRow{}
	for _, r := range view.Tenants {
		byTenant[r.TenantID] = r
	}
	if r := byTenant["ten1"]; r.Verdict != "over-provisioned" || r.LicensedResources != 25 || r.ProtectedResources != 1 || r.Delta != 24 {
		t.Errorf("ten1 row wrong (storage item must not count): %+v", r)
	}
	if r := byTenant["ten2"]; r.Verdict != "under-licensed" || r.Delta != -1 {
		t.Errorf("ten2 row wrong: %+v", r)
	}
	if r := byTenant["ten3"]; r.Verdict != "aligned" || r.Delta != 0 {
		t.Errorf("ten3 row wrong: %+v", r)
	}
	// Largest absolute delta sorts first.
	if view.Tenants[0].TenantID != "ten1" {
		t.Errorf("sort order wrong: first = %s", view.Tenants[0].TenantID)
	}
}

func TestReconcileLicensesOrgFilter(t *testing.T) {
	path := seedReconcileFixture(t)
	out, err := runNovel(t, newNovelReconcileLicensesCmd, "--db", path, "--org", "org2")
	if err != nil {
		t.Fatalf("reconcile-licenses: %v", err)
	}
	view := decodeView[reconcileView](t, out)
	if len(view.Tenants) != 1 || view.Tenants[0].TenantID != "ten3" {
		t.Errorf("--org filter wrong: %s", out)
	}
}

func TestReconcileLicensesTenantNames(t *testing.T) {
	path := seedReconcileFixture(t)
	out, err := runNovel(t, newNovelReconcileLicensesCmd, "--db", path)
	if err != nil {
		t.Fatalf("reconcile-licenses: %v", err)
	}
	view := decodeView[reconcileView](t, out)
	for _, r := range view.Tenants {
		if r.TenantName == "" {
			t.Errorf("tenant name missing for %s", r.TenantID)
		}
	}
}

func TestReconcileLicensesEmptyStoreNote(t *testing.T) {
	_, path := newTestStore(t)
	out, err := runNovel(t, newNovelReconcileLicensesCmd, "--db", path)
	if err != nil {
		t.Fatalf("reconcile-licenses: %v", err)
	}
	view := decodeView[reconcileView](t, out)
	if len(view.Tenants) != 0 || view.Note == "" {
		t.Errorf("empty store should note fleet-sync: %s", out)
	}
}
