// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
package cli

import (
	"strings"
	"testing"
)

func seedScorecardFixture(t *testing.T) string {
	db, path := newTestStore(t)
	seed(t, db, "tenants", "ten1", `{"id":"ten1","name":"Contoso","kind":"o365","region":"us","org_id":"org1"}`)
	seed(t, db, "resources", "res1", `{"id":"res1","tenant_id":"ten1","kind":"user","archived":false}`)
	seed(t, db, "resources", "res2", `{"id":"res2","tenant_id":"ten1","kind":"user","archived":false}`)
	seed(t, db, "resources", "res3", `{"id":"res3","tenant_id":"ten1","kind":"site","archived":true}`)
	seed(t, db, "protections", "p1", `{"id":"p1","resource_id":"res1","tenant_id":"ten1"}`)
	seed(t, db, "policies", "pol1", `{"id":"pol1","tenant_id":"ten1","name":"Daily"}`)
	seed(t, db, "archives", "a1", `{"id":"a1","tenant_id":"ten1","resource_id":"res1","created_at":"2026-06-01T00:00:00Z","stats":{"size":"2048"}}`)
	seed(t, db, "archives", "a2", `{"id":"a2","tenant_id":"ten1","resource_id":"res1","created_at":"2026-05-01T00:00:00Z","stats":{"size":"1024"}}`)
	seed(t, db, "quotas", "ten1:storage", `{"tenant_id":"ten1","kind":"storage","used":"99","limit":"100","units":"GB","exceeded":true}`)
	seed(t, db, "task_stats", "ten1", `{"tenant_id":"ten1","total":{"done":7,"failed":1,"warnings":0},"window_start":"2026-05-30T00:00:00Z","window_end":"2026-06-06T00:00:00Z"}`)
	return path
}

func TestTenantScorecardRollup(t *testing.T) {
	path := seedScorecardFixture(t)
	out, err := runNovel(t, newNovelTenantScorecardCmd, "ten1", "--db", path)
	if err != nil {
		t.Fatalf("tenant-scorecard: %v", err)
	}
	v := decodeView[tenantScorecardView](t, out)
	if v.TenantName != "Contoso" || v.TenantKind != "o365" || v.Region != "us" || v.OrgID != "org1" {
		t.Errorf("identity wrong: %+v", v)
	}
	if v.Resources != 3 || v.ArchivedResources != 1 || v.Protected != 1 {
		t.Errorf("counts wrong: resources=%d archived=%d protected=%d", v.Resources, v.ArchivedResources, v.Protected)
	}
	if v.CoveragePct < 33 || v.CoveragePct > 34 {
		t.Errorf("coverage = %f, want ~33.3", v.CoveragePct)
	}
	if v.Policies != 1 || v.Archives != 2 {
		t.Errorf("policies=%d archives=%d", v.Policies, v.Archives)
	}
	if v.NewestArchiveAt != "2026-06-01T00:00:00Z" || v.OldestArchiveAt != "2026-05-01T00:00:00Z" {
		t.Errorf("archive ages wrong: %s / %s", v.NewestArchiveAt, v.OldestArchiveAt)
	}
	if v.ArchiveSizeKB != 3072 {
		t.Errorf("archive size = %d, want 3072 (string-encoded sizes summed)", v.ArchiveSizeKB)
	}
	if v.TasksDone != 7 || v.TasksFailed != 1 {
		t.Errorf("task stats wrong: %+v", v)
	}
	if len(v.Quotas) != 1 || !v.Quotas[0].Exceeded || len(v.QuotasExceeded) != 1 {
		t.Errorf("quotas wrong: %+v", v.Quotas)
	}
}

func TestTenantScorecardUnknownTenant(t *testing.T) {
	path := seedScorecardFixture(t)
	out, err := runNovel(t, newNovelTenantScorecardCmd, "nope", "--db", path)
	if err != nil {
		t.Fatalf("tenant-scorecard: %v", err)
	}
	v := decodeView[tenantScorecardView](t, out)
	if !strings.Contains(v.Note, "not found") {
		t.Errorf("unknown tenant should set a note: %s", out)
	}
}

func TestTenantScorecardMissingArg(t *testing.T) {
	_, path := newTestStore(t)
	_, err := runNovel(t, newNovelTenantScorecardCmd, "--db", path)
	if err == nil {
		t.Fatal("expected usage error when tenant ID missing")
	}
}

func TestTenantScorecardNoTaskSnapshot(t *testing.T) {
	db, path := newTestStore(t)
	seed(t, db, "tenants", "ten9", `{"id":"ten9","name":"Empty","kind":"o365"}`)
	out, err := runNovel(t, newNovelTenantScorecardCmd, "ten9", "--db", path)
	if err != nil {
		t.Fatalf("tenant-scorecard without snapshot: %v", err)
	}
	v := decodeView[tenantScorecardView](t, out)
	if v.TasksDone != 0 || v.TasksFailed != 0 {
		t.Errorf("missing snapshot should produce zero task counts: %+v", v)
	}
}
