// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"sophos-central-pp-cli/internal/cliutil/testenv"
	"sophos-central-pp-cli/internal/store"
)

// Codex round 1 (2026-11-02 reprint) findings, one test per class.

// Class "wrong tenant's data": every fleet command honors --tenant. The usage
// report lists every customer (items[].accountId); revenue must keep only the
// selected ones.
func TestFleetRevenueHonorsTenantSelector(t *testing.T) {
	testenv.Isolate(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/token":
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "tok", "token_type": "bearer", "expires_in": 3600})
		case r.URL.Path == "/whoami/v1":
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "P1", "idType": "partner", "apiHosts": map[string]string{"global": "https://api.central.sophos.com"}})
		case r.URL.Path == "/partner/v1/billing/usage/2026/09":
			_ = json.NewEncoder(w).Encode(map[string]any{"items": []any{
				map[string]any{"accountId": "48bebbb0-f7b6-49c5-aec0-315858f73457", "billableQuantity": 5, "unitPrice": "4.00", "unitPriceCurrency": "USD"},
				map[string]any{"accountId": "7767a31c-dd05-4096-a721-1a0f3b006b16", "billableQuantity": 9, "unitPrice": "4.00", "unitPriceCurrency": "USD"},
			}, "pages": map[string]any{}})
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(srv.Close)
	t.Setenv("SOPHOS_CENTRAL_BASE_URL", srv.URL)
	t.Setenv("SOPHOS_CENTRAL_TOKEN_URL", srv.URL+"/token")
	t.Setenv("SOPHOS_CENTRAL_CLIENT_ID", "cid")
	t.Setenv("SOPHOS_CENTRAL_CLIENT_SECRET", "secret")
	t.Setenv(sophosTenantEnv, "48bebbb0-f7b6-49c5-aec0-315858f73457")
	sophosTenantFlag = ""
	out, err := runRoot(t, "fleet", "revenue", "--month", "2026-09", "--db", filepath.Join(t.TempDir(), "none.db"), "--json")
	if err != nil {
		t.Fatalf("fleet revenue: %v\n%s", err, out)
	}
	var bv billingView
	if err := json.Unmarshal([]byte(out), &bv); err != nil {
		t.Fatalf("decode: %v\n%s", err, out)
	}
	if len(bv.Items) != 1 || bv.Items[0].AccountID != "48bebbb0-f7b6-49c5-aec0-315858f73457" {
		t.Fatalf("--tenant selected one customer; revenue returned %+v", bv.Items)
	}
}

// Class "incomplete health read as complete": the health-check spec documents
// a null endpoint.{protection,policy,exclusions,tamperProtection} as an
// unexpected error, never a pass.
func TestFleetHealthNullSectionIsAFailedCheck(t *testing.T) {
	f := fleetFixture()
	data, _ := json.Marshal(map[string]any{
		"tenant": map[string]any{"id": "T1"},
		"endpoint": map[string]any{
			"protection":       map[string]any{"computer": map[string]any{"score": 100, "total": 1, "notFullyProtected": 0, "snoozed": false}},
			"policy":           nil,
			"exclusions":       nil,
			"tamperProtection": nil,
		},
	})
	f.put(fleetHealthType, "T1", tenantHealthDoc{TenantID: "T1", TenantName: "Acme Dental", SyncedAt: testNow, Data: data})
	view, err := buildHealth(f, map[string]bool{"T1": true})
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Items) != 1 || view.Items[0].Status == "ok" {
		t.Fatalf("three failed health sections reported as %+v", view.Items)
	}
	blind := false
	for _, b := range view.BlindTenants {
		blind = blind || b.TenantID == "T1"
	}
	if !blind {
		t.Fatalf("T1's failed checks must make it a blind tenant: %+v", view.BlindTenants)
	}
	gaps, err := buildGaps(f, map[string]bool{"T1": true}, gapOptions{threshold: 80, kind: "security", now: testNow})
	if err != nil {
		t.Fatal(err)
	}
	failed := false
	for _, tg := range gaps.Items {
		for _, g := range tg.Gaps {
			failed = failed || g.Type == "health_check_failed"
		}
	}
	if !failed {
		t.Fatalf("fleet gaps lost the failed health checks: %+v", gaps.Items)
	}
}

// Class "missing tenant attribution": the endpoint schema makes tenant
// optional, so a fleet row records the tenant it was fetched for.
func TestStoreTenantRowsAttributesTheFetchTenant(t *testing.T) {
	db, err := store.OpenWithContext(context.Background(), filepath.Join(t.TempDir(), "fleet.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	const tid, eid = "48bebbb0-f7b6-49c5-aec0-315858f73457", "59cf25c1-974a-4899-a84a-1883e68bd498"
	row := json.RawMessage(`{"id":"` + eid + `","type":"computer","hostname":"WS1","os":{"platform":"windows","name":"Windows 11"},"tamperProtectionSupported":true,"tamperProtectionEnabled":false}`)
	if _, err := storeTenantRows(db, "endpoints", tid, []json.RawMessage{row}, true); err != nil {
		t.Fatal(err)
	}
	for _, rt := range []string{"endpoints", fleetEndpointsType} {
		raw, err := db.Get(rt, eid)
		if err != nil || rowTenantID(raw) != tid {
			t.Fatalf("%s row not attributed to its fetch tenant: %s err=%v", rt, raw, err)
		}
	}
	view, err := buildTriage(db, map[string]bool{"high": true, "medium": true, "low": true}, 14*24*time.Hour, false, time.Now(), map[string]bool{tid: true})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, r := range view.Items {
		found = found || (r.Kind == "tamper_off" && r.ID == eid)
	}
	if !found {
		t.Fatalf("the unprotected endpoint vanished from its tenant's queue: %+v", view.Items)
	}
	// A row that names another tenant is not this fetch's to store.
	other := json.RawMessage(`{"id":"6a1d2c3b-0000-4000-8000-000000000001","hostname":"WS2","tenant":{"id":"7767a31c-dd05-4096-a721-1a0f3b006b16"}}`)
	if _, err := storeTenantRows(db, "endpoints", tid, []json.RawMessage{other}, true); err == nil {
		t.Fatal("a row for another tenant was stored under this tenant's fetch")
	}
}

func gapTypes(t *testing.T, f fakeFleetStore, now time.Time) map[string]bool {
	t.Helper()
	view, err := buildGaps(f, map[string]bool{"T1": true}, gapOptions{threshold: 80, kind: "commercial", now: now})
	if err != nil {
		t.Fatal(err)
	}
	types := map[string]bool{}
	for _, tg := range view.Items {
		for _, g := range tg.Gaps {
			types[g.Type] = true
		}
	}
	return types
}

// Class "wrong field read from a documented response": the endpoint schema
// documents both deviceSoftware.protection.status and packages.protection.status.
func TestGapsReadDeviceSoftwareUnlicensed(t *testing.T) {
	f := fakeFleetStore{}
	f.put(fleetLedgerType, "T1", fleetLedgerRow{ID: "T1", Status: "ok", Resources: map[string]fleetOutcome{"endpoints": {Status: "ok"}, "licenses": {Status: "ok"}}})
	f.put(fleetEndpointsType, "E1", map[string]any{"id": "E1", "type": "computer", "hostname": "WS1", "tenant": map[string]any{"id": "T1"},
		"deviceSoftware": map[string]any{"protection": map[string]any{"status": "unlicensed"}}})
	if !gapTypes(t, f, testNow)["unlicensed_devices"] {
		t.Fatal("deviceSoftware.protection.status unlicensed produced no unlicensed_devices gap")
	}
}

// Class "wrong licence result": quantity 0 is a finite entitlement, and
// endDate is valid until the end of that UTC day (licensing spec).
func TestGapsLicenceZeroQuantityAndEndOfDay(t *testing.T) {
	put := func(lic map[string]any) fakeFleetStore {
		f := fakeFleetStore{}
		f.put(fleetLedgerType, "T1", fleetLedgerRow{ID: "T1", Status: "ok", Resources: map[string]fleetOutcome{"licenses": {Status: "ok"}}})
		data, _ := json.Marshal(map[string]any{"licenses": []any{lic}})
		f.put(fleetLicensesType, "T1", tenantHealthDoc{TenantID: "T1", Data: data})
		return f
	}
	over := put(map[string]any{"id": "L1", "licenseIdentifier": "L1", "product": map[string]any{"code": "EP"}, "startDate": "2026-01-01", "endDate": "2027-12-31",
		"perpetual": false, "type": "term", "unlimited": false, "quantity": 0, "usage": map[string]any{"current": map[string]any{"count": 5}}})
	if !gapTypes(t, over, testNow)["licence_over_usage"] {
		t.Fatal("five seats on a zero-quantity licence produced no licence_over_usage gap")
	}
	noQty := put(map[string]any{"id": "L2", "product": map[string]any{"code": "EP"}, "endDate": "2027-12-31", "unlimited": false, "usage": map[string]any{"current": map[string]any{"count": 5}}})
	if gapTypes(t, noQty, testNow)["licence_over_usage"] {
		t.Fatal("a licence that states no quantity must not be read as quantity 0")
	}
	today := time.Date(2026, 11, 2, 15, 0, 0, 0, time.UTC)
	ending := put(map[string]any{"id": "L1", "product": map[string]any{"code": "EP"}, "startDate": "2026-01-01", "endDate": "2026-11-02", "perpetual": false, "type": "term", "unlimited": true})
	if !gapTypes(t, ending, today)["licence_expiring"] {
		t.Fatal("a licence valid until the end of today produced no licence_expiring gap")
	}
}
