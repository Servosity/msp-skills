// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/sophos-central/handfixes.json
// under the "fleet-freshness-per-resource" entry.

package cli

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"path/filepath"
	"sync"
	"testing"

	"sophos-central-pp-cli/internal/cliutil/testenv"
	"sophos-central-pp-cli/internal/store"
)

// TestFullRefreshPrunesFromTheLiveTenantList: the tenant list that decides
// what a full refresh prunes is read live, never from the response cache, and
// a tenant that has no ledger row yet is reported, not presented as complete.
func TestFullRefreshPrunesFromTheLiveTenantList(t *testing.T) {
	testenv.Isolate(t)
	inner, _ := fakeSophosAPI(t)
	target, _ := url.Parse(inner.URL)
	proxy := httputil.NewSingleHostReverseProxy(target)
	var mu sync.Mutex
	listed := []string{"T1"}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/partner/v1/tenants" {
			proxy.ServeHTTP(w, r)
			return
		}
		mu.Lock()
		items := make([]map[string]any, 0, len(listed))
		for _, id := range listed {
			items = append(items, map[string]any{"id": id, "name": "Tenant " + id, "billingType": "term", "apiHost": "https://api-us01.central.sophos.com"})
		}
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"items": items, "pages": map[string]int{"current": 1, "total": 1}})
	}))
	t.Cleanup(srv.Close)
	t.Setenv("SOPHOS_CENTRAL_BASE_URL", srv.URL)
	t.Setenv("SOPHOS_CENTRAL_TOKEN_URL", srv.URL+"/token")
	t.Setenv("SOPHOS_CENTRAL_CLIENT_ID", "cid")
	t.Setenv("SOPHOS_CENTRAL_CLIENT_SECRET", "secret")
	t.Setenv(sophosTenantEnv, "")
	sophosTenantFlag = ""
	db := filepath.Join(t.TempDir(), "fleet.db")
	refresh := func(args ...string) fleetSyncSummary {
		t.Helper()
		out, err := runRoot(t, append([]string{"fleet", "refresh", "--db", db, "--json"}, args...)...)
		var s fleetSyncSummary
		if err != nil || json.Unmarshal([]byte(out), &s) != nil {
			t.Fatalf("fleet refresh %v: %v\n%s", args, err, out)
		}
		return s
	}
	triage := func() triageView {
		t.Helper()
		out, err := runRoot(t, "fleet", "triage", "--db", db, "--json")
		var v triageView
		if err != nil || json.Unmarshal([]byte(out), &v) != nil {
			t.Fatalf("fleet triage: %v\n%s", err, out)
		}
		return v
	}

	refresh()
	mu.Lock()
	listed = []string{"T1", "T4"} // onboarded upstream after the first refresh
	mu.Unlock()
	if s := refresh("--tenant", "T4"); s.TenantsTotal != 1 {
		t.Fatalf("--tenant T4 refreshes just T4: %+v", s)
	}
	sophosTenantFlag = ""
	if s := refresh(); len(s.Pruned) != 0 || s.TenantsTotal != 2 {
		t.Fatalf("a full refresh must list tenants live and keep T4: %+v", s)
	}

	mu.Lock()
	listed = []string{"T1"} // T4 offboarded
	mu.Unlock()
	if s := refresh(); len(s.Pruned) != 1 || s.Pruned[0] != "T4" {
		t.Fatalf("an offboarded tenant must be pruned: %+v", s)
	}
	for _, r := range triage().Items {
		if r.TenantID == "T4" {
			t.Fatalf("pruned tenant still in triage: %+v", r)
		}
	}

	mu.Lock()
	listed = []string{"T1", "T5"}
	mu.Unlock()
	refresh("--resources", "tenants")
	v := triage()
	if len(v.BlindTenants) != 1 || v.BlindTenants[0].TenantID != "T5" || v.BlindTenants[0].Status != "not_refreshed" {
		t.Fatalf("a stored tenant with no refresh yet must be blind: %+v", v.BlindTenants)
	}

	// A --tenant refresh leaves the fleet-wide stamp alone, and a local
	// --tenant read is dated by that tenant's own fetch.
	refresh()
	st, err := store.OpenWithContext(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	_, _, before, _ := st.GetSyncState("endpoints")
	st.Close()
	refresh("--tenant", "T5")
	st, _ = store.OpenWithContext(context.Background(), db)
	_, _, after, _ := st.GetSyncState("endpoints")
	at5, _ := sophosLocalSyncedAt(st, "endpoints")
	sophosTenantFlag = "T1"
	at1, _ := sophosLocalSyncedAt(st, "endpoints")
	prov := localProvenance(st, "endpoints", "")
	sophosTenantFlag = ""
	st.Close()
	if before != 4 || after != before {
		t.Fatalf("a --tenant refresh rewrote the fleet-wide endpoints stamp: count %d -> %d", before, after)
	}
	if !at5.After(at1) || prov.SyncedAt == nil || !prov.SyncedAt.Equal(at1) {
		t.Fatalf("local reads must be dated per tenant: T5 %v, T1 %v, T1 provenance %v", at5, at1, prov.SyncedAt)
	}
	sophosTenantFlag = ""

	mu.Lock()
	listed = nil // every customer left
	mu.Unlock()
	if s := refresh(); len(s.Pruned) != 2 {
		t.Fatalf("an empty listing must prune every stored tenant: %+v", s)
	}
	if v := triage(); len(v.Items) != 0 || len(v.BlindTenants) != 0 {
		t.Fatalf("nothing may remain after every tenant left: %+v", v)
	}
}

func TestNamelessTenantsRowIsNotATenant(t *testing.T) {
	f := fakeFleetStore{}
	f.put("tenants", "T1", fleetTenant{ID: "T1", Name: "Acme"})
	f.put("tenants", "amend-1", map[string]any{"id": "amend-1", "type": "billingTypeChange"})
	if got := unrefreshedTenants(f, map[string]fleetLedgerRow{}, nil); len(got) != 1 || got[0].TenantID != "T1" {
		t.Fatalf("a cached amendment response is not a tenant: %+v", got)
	}
}
