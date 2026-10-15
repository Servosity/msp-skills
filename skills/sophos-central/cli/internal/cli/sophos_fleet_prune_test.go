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
	"strings"
	"sync"
	"testing"
	"time"

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

func TestPruneCoversEndpointsOfUnrecordedTenants(t *testing.T) {
	db, err := store.OpenWithContext(context.Background(), filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	// As plain `sync --tenant T9` leaves it: endpoint and alert rows, no
	// tenant or ledger row.
	for _, rt := range []string{"endpoints", "alerts"} {
		if _, _, err := db.UpsertBatch(rt, []json.RawMessage{json.RawMessage(`{"id": "00000000-0000-0000-0000-00000000000` + map[string]string{"endpoints": "1", "alerts": "2"}[rt] + `", "tenant": {"id": "T9"}}`)}); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.SaveSyncState("endpoints@T9", "", 1); err != nil {
		t.Fatal(err)
	}
	// T4 was written by a --tenant refresh that ran alongside this one.
	started := time.Now().UTC()
	row, _ := json.Marshal(fleetLedgerRow{ID: "T4", Status: "ok", SyncedAt: started.Add(time.Second)})
	if _, _, err := db.UpsertBatch(fleetLedgerType, []json.RawMessage{row}); err != nil {
		t.Fatal(err)
	}
	pruned, err := pruneOutOfScopeTenants(db, []fleetTenant{{ID: "T1", Name: "Acme"}}, started)
	if err != nil || len(pruned) != 1 || pruned[0] != "T9" {
		t.Fatalf("pruned=%v err=%v", pruned, err)
	}
	if _, at, _, _ := db.GetSyncState("endpoints@T9"); !at.IsZero() {
		t.Fatalf("a pruned tenant's checkpoint must not date later local reads: %v", at)
	}
	if ledger, _ := loadFleetLedger(db); ledger["T4"].ID != "T4" {
		t.Fatal("a tenant refreshed after the listing must not be pruned")
	}
	for _, rt := range []string{"endpoints", "alerts"} {
		n := 0
		_ = db.ListScan(rt, func(string, json.RawMessage) bool { n++; return true })
		if n != 0 {
			t.Fatalf("%s rows of an offboarded tenant survived the prune", rt)
		}
	}
}

func TestNamelessTenantsRowIsNotATenant(t *testing.T) {
	f := fakeFleetStore{}
	f.put("tenants", "T1", fleetTenant{ID: "T1", Name: "Acme"})
	f.put("tenants", "amend-1", map[string]any{"id": "amend-1", "type": "billingTypeChange"})
	if got := unrefreshedTenants(f, map[string]fleetLedgerRow{}, nil); len(got) != 1 || got[0].TenantID != "T1" {
		t.Fatalf("a cached amendment response is not a tenant: %+v", got)
	}
	f.put("endpoints", "E9", map[string]any{"id": "E9", "tenant": map[string]any{"id": "T9"}})
	got := unrefreshedTenants(f, map[string]fleetLedgerRow{"T1": {ID: "T1"}}, nil)
	if len(got) != 1 || got[0].TenantID != "T9" {
		t.Fatalf("endpoints plain sync stored for a tenant fleet refresh never reached must be blind: %+v", got)
	}
	f.put(fleetMetaType, "sync", fleetSyncMeta{ID: "sync", TrialSkipped: []string{"T9"}})
	if got := unrefreshedTenants(f, map[string]fleetLedgerRow{"T1": {ID: "T1"}}, nil); len(got) != 0 {
		t.Fatalf("a trial tenant the last refresh skipped is out of scope, not blind: %+v", got)
	}
}

func TestReceiptsAreNotCachedAsEndpointRows(t *testing.T) {
	rows := []json.RawMessage{json.RawMessage(`{"id": "scan-1", "status": "requested"}`), json.RawMessage(`{"id": "E1", "tenant": {"id": "T1"}}`)}
	if got := sophosAttributableRows("endpoints", rows); len(got) != 1 || rowTenantID(got[0]) != "T1" {
		t.Fatalf("a tenant-less receipt must not be cached among endpoints: %s", got)
	}
	if got := sophosAttributableRows("tenants", rows); len(got) != 2 {
		t.Fatal("partner-level rows carry no tenant and are kept")
	}
}

func TestFleetSnapshotsReplaceInsteadOfMerging(t *testing.T) {
	db, err := store.OpenWithContext(context.Background(), filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ten := fleetTenant{ID: "T1", Name: "Acme"}
	if err := storeTenantDoc(db, fleetLicensesType, ten, json.RawMessage(`{"licenses": [{"id": "L1", "quantity": 5}]}`)); err != nil {
		t.Fatal(err)
	}
	if err := storeTenantDoc(db, fleetLicensesType, ten, json.RawMessage(`{"licenses": []}`)); err != nil {
		t.Fatal(err)
	}
	raw, _ := db.Get(fleetLicensesType, "T1")
	if strings.Contains(string(raw), "L1") {
		t.Fatalf("a refresh that returns no licences must not keep the old ones: %s", raw)
	}
	windowed, _ := json.Marshal(fleetLedgerRow{ID: "T1", Status: "ok", Resources: map[string]fleetOutcome{"alerts": {Status: "ok", Window: "24h0m0s"}}})
	full, _ := json.Marshal(fleetLedgerRow{ID: "T1", Status: "ok", Resources: map[string]fleetOutcome{"alerts": {Status: "ok"}}})
	for _, r := range []json.RawMessage{windowed, full} {
		if err := replaceFleetDoc(db, fleetLedgerType, r); err != nil {
			t.Fatal(err)
		}
	}
	if ledger, _ := loadFleetLedger(db); ledger["T1"].Resources["alerts"].Window != "" {
		t.Fatal("a full fetch after a --since fetch must clear the window")
	}
}
