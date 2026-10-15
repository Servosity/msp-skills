// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/sophos-central/handfixes.json
// under the "fleet-freshness-per-resource" entry.

package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"path/filepath"
	"sync"
	"testing"

	"sophos-central-pp-cli/internal/cliutil/testenv"
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
	sophosTenantFlag = "T4"
	refresh()
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
}
