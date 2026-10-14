// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"encoding/json"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"sophos-central-pp-cli/internal/store"
)

// fakeFleetStore is an in-memory fleetReader.
type fakeFleetStore map[string]map[string]json.RawMessage

func (f fakeFleetStore) put(rt, id string, v any) {
	if f[rt] == nil {
		f[rt] = map[string]json.RawMessage{}
	}
	b, _ := json.Marshal(v)
	f[rt][id] = b
}

func (f fakeFleetStore) ListScan(rt string, fn func(string, json.RawMessage) bool) error {
	ids := make([]string, 0, len(f[rt]))
	for id := range f[rt] {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if !fn(id, f[rt][id]) {
			break
		}
	}
	return nil
}

func (f fakeFleetStore) Get(rt, id string) (json.RawMessage, error) { return f[rt][id], nil }

var testNow = time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)

func fleetFixture() fakeFleetStore {
	f := fakeFleetStore{}
	f.put("tenants", "T1", fleetTenant{ID: "T1", Name: "Acme Dental"})
	f.put("tenants", "T2", fleetTenant{ID: "T2", Name: "Birch Legal"})
	f.put(fleetLedgerType, "T1", fleetLedgerRow{ID: "T1", TenantName: "Acme Dental", Status: "ok"})
	f.put(fleetLedgerType, "T2", fleetLedgerRow{ID: "T2", TenantName: "Birch Legal", Status: "ok"})
	f.put(fleetLedgerType, "T3", fleetLedgerRow{ID: "T3", TenantName: "Cedar Trial", Status: "forbidden",
		Resources: map[string]fleetOutcome{"endpoints": {Status: "forbidden", Error: "HTTP 403"}}})
	f.put(fleetMetaType, "sync", fleetSyncMeta{ID: "sync", PrevStartedAt: testNow.Add(-24 * time.Hour), FinishedAt: testNow.Add(-time.Hour)})
	f.put("alerts", "A1", map[string]any{"id": "A1", "severity": "high", "description": "Malware detected", "raisedAt": testNow.Add(-2 * time.Hour).Format(time.RFC3339),
		"tenant": map[string]any{"id": "T1", "name": "Acme Dental"}, "managedAgent": map[string]any{"id": "E1", "name": "WS-FIN-07"}, "allowedActions": []string{"acknowledge"}})
	f.put("alerts", "A2", map[string]any{"id": "A2", "severity": "low", "description": "PUA", "raisedAt": testNow.Add(-72 * time.Hour).Format(time.RFC3339),
		"tenant": map[string]any{"id": "T2", "name": "Birch Legal"}})
	f.put("endpoints", "E1", map[string]any{"id": "E1", "hostname": "WS-FIN-07", "tenant": map[string]any{"id": "T1"}, "health": map[string]any{"overall": "bad"},
		"tamperProtectionSupported": true, "tamperProtectionEnabled": false, "isolation": map[string]any{"status": "notIsolated"}, "lastSeenAt": testNow.Add(-time.Hour).Format(time.RFC3339),
		"ipv4Addresses": []string{"10.20.4.17"}, "type": "computer",
		"assignedProducts": []map[string]any{{"code": "deviceEncryption", "status": "installed"}}, "encryption": map[string]any{"overallStatus": "notEncrypted"},
		"packages": map[string]any{"protection": map[string]any{"status": "unlicensed"}}})
	f.put("endpoints", "E2", map[string]any{"id": "E2", "hostname": "SRV-DC01", "tenant": map[string]any{"id": "T2"}, "health": map[string]any{"overall": "good"},
		"tamperProtectionSupported": true, "tamperProtectionEnabled": true, "isolation": map[string]any{"status": "notIsolated"}, "lastSeenAt": testNow.Add(-30 * 24 * time.Hour).Format(time.RFC3339), "type": "server"})
	return f
}

func healthDoc(tenantID, name string, computer, server int, tamperGlobal bool, snoozeServer bool) tenantHealthDoc {
	data := map[string]any{
		"tenant": map[string]any{"id": tenantID},
		"endpoint": map[string]any{
			"protection": map[string]any{
				"computer": map[string]any{"score": computer, "total": 10, "notFullyProtected": 2, "snoozed": false},
				"server":   map[string]any{"score": server, "total": 2, "snoozed": snoozeServer, "snoozeDetail": map[string]any{"expiry": "2026-11-01T00:00:00Z"}},
			},
			"tamperProtection": map[string]any{"globalDetail": map[string]any{"score": 100, "enabled": tamperGlobal, "snoozed": false}},
			"exclusions":       map[string]any{"global": map[string]any{"score": 60, "numberOfSecurityRisks": 3, "snoozed": false}},
		},
	}
	raw, _ := json.Marshal(data)
	return tenantHealthDoc{TenantID: tenantID, TenantName: name, SyncedAt: testNow, Data: raw}
}

func TestBuildTriageRanksAndFilters(t *testing.T) {
	view, err := buildTriage(fleetFixture(), map[string]bool{"high": true, "medium": true}, 14*24*time.Hour, false, testNow, nil)
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string]bool{}
	for _, r := range view.Items {
		kinds[r.Kind] = true
		if r.Severity == "low" {
			t.Fatalf("low-severity row leaked through --severity high,medium: %+v", r)
		}
	}
	if !kinds["alert"] || !kinds["unhealthy"] || !kinds["tamper_off"] {
		t.Fatalf("missing expected kinds, got %+v", view.Items)
	}
	if view.Items[0].Severity != "high" {
		t.Fatalf("queue not severity-ranked: first row %+v", view.Items[0])
	}
	for _, r := range view.Items {
		if r.Kind == "alert" && (!r.New || r.TenantName != "Acme Dental" || !strings.Contains(r.NextCommand, "--tenant T1")) {
			t.Fatalf("alert row wrong: %+v", r)
		}
	}
	if len(view.BlindTenants) != 1 || view.BlindTenants[0].TenantID != "T3" || view.BlindTenants[0].Status != "forbidden" {
		t.Fatalf("blind tenants = %+v, want T3 forbidden", view.BlindTenants)
	}
	// Stale + low severity: the 30-day-old server appears only when asked for.
	low, _ := buildTriage(fleetFixture(), map[string]bool{"low": true}, 14*24*time.Hour, false, testNow, nil)
	found := false
	for _, r := range low.Items {
		if r.Kind == "stale" && r.ID == "E2" {
			found = true
		}
	}
	if !found {
		t.Fatalf("stale endpoint E2 missing from low-severity queue: %+v", low.Items)
	}
	// Tenant filter: nothing from T1 when only T2 is selected.
	only2, _ := buildTriage(fleetFixture(), map[string]bool{"high": true, "medium": true, "low": true}, 14*24*time.Hour, false, testNow, map[string]bool{"T2": true})
	for _, r := range only2.Items {
		if r.TenantID != "T2" {
			t.Fatalf("tenant filter leaked %+v", r)
		}
	}
	// Empty store: empty queue with a note, not an error.
	empty, err := buildTriage(fakeFleetStore{}, map[string]bool{"high": true}, time.Hour, false, testNow, nil)
	if err != nil || len(empty.Items) != 0 || empty.Note == "" {
		t.Fatalf("empty store: %+v err=%v", empty, err)
	}
}

func TestBuildHealthRanksWorstFirstWithDelta(t *testing.T) {
	f := fleetFixture()
	f.put(fleetHealthType, "T1", healthDoc("T1", "Acme Dental", 55, 90, true, false))
	f.put(fleetHealthType, "T2", healthDoc("T2", "Birch Legal", 95, 40, true, true)) // server snoozed
	f.put(fleetHealthPrevType, "T1", healthDoc("T1", "Acme Dental", 70, 90, true, false))
	view, err := buildHealth(f, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Items) != 3 {
		t.Fatalf("want T1, T2 and forbidden T3, got %+v", view.Items)
	}
	first := view.Items[0]
	if first.TenantID != "T1" || first.Overall == nil || *first.Overall != 55 {
		t.Fatalf("worst tenant should be T1 at 55, got %+v", first)
	}
	// Previous snapshot's lowest active check was exclusions at 60.
	if first.Delta == nil || *first.Delta != -5 {
		t.Fatalf("T1 delta should be -5 (60 -> 55), got %s", intOrDash(first.Delta))
	}
	second := view.Items[1]
	// T2's snoozed server score (40) must not drag its overall down; the
	// exclusions check (60) is its lowest active score.
	if second.TenantID != "T2" || *second.Overall != 60 || len(second.SnoozedChecks) != 1 {
		t.Fatalf("T2 should be 60 with one snoozed check, got %+v", second)
	}
	if second.ProtectionScore == nil || *second.ProtectionScore != 40 {
		t.Fatalf("protection_score is min(computer, server) regardless of snooze, got %v", second.ProtectionScore)
	}
	last := view.Items[2]
	if last.TenantID != "T3" || last.Overall != nil || last.Status != "forbidden" {
		t.Fatalf("forbidden tenant should sort last with its reason, got %+v", last)
	}
}

func TestBuildGapsSecurityAndCommercial(t *testing.T) {
	f := fleetFixture()
	f.put(fleetHealthType, "T1", healthDoc("T1", "Acme Dental", 55, 90, false, false))
	licData, _ := json.Marshal(map[string]any{"licenses": []map[string]any{
		{"product": map[string]any{"name": "Intercept X Advanced"}, "quantity": 10, "usage": map[string]any{"current": map[string]any{"count": 14}}, "endDate": "2026-10-20"},
		{"product": map[string]any{"name": "Server Protection"}, "quantity": 5, "usage": map[string]any{"current": map[string]any{"count": 2}}, "endDate": "2027-10-20"},
	}})
	f.put(fleetLicensesType, "T1", tenantHealthDoc{TenantID: "T1", TenantName: "Acme Dental", Data: licData})
	view, err := buildGaps(f, nil, gapOptions{threshold: 80, includeServers: true, now: testNow})
	if err != nil {
		t.Fatal(err)
	}
	types := map[string]bool{}
	for _, tg := range view.Items {
		if tg.TenantID != "T1" {
			continue
		}
		for _, g := range tg.Gaps {
			types[g.Type] = true
		}
	}
	for _, want := range []string{"low_health_score", "risky_exclusions", "tamper_global_off", "unencrypted_devices", "unlicensed_devices", "licence_over_usage", "licence_expiring", "no_server_protection"} {
		if !types[want] {
			t.Errorf("T1 missing gap %q (got %v)", want, types)
		}
	}
	if types["products_not_installed"] {
		t.Error("no product is notInstalled; gap must not appear")
	}
	// T2 has a server: no server gap there; healthy licence not flagged.
	for _, tg := range view.Items {
		if tg.TenantID == "T2" {
			for _, g := range tg.Gaps {
				if g.Type == "no_server_protection" {
					t.Fatal("T2 has a server; no_server_protection is wrong")
				}
			}
		}
	}
	commercial, _ := buildGaps(f, nil, gapOptions{threshold: 80, kind: "commercial", now: testNow})
	for _, tg := range commercial.Items {
		for _, g := range tg.Gaps {
			if g.Kind != "commercial" {
				t.Fatalf("--kind commercial leaked %+v", g)
			}
		}
	}
}

func TestBuildBillingPerCurrencyAndJoin(t *testing.T) {
	lines := []billingLine{
		{AccountID: "T1", BillableQuantity: 10, UnitPrice: "3.50", UnitPriceCurrency: "USD", ProductDesc: "Intercept X"},
		{AccountID: "T1", BillableQuantity: 2, UnitPrice: "10", UnitPriceCurrency: "EUR", ProductDesc: "Server"},
		{AccountID: "X9", BillableQuantity: 1, UnitPrice: "100", UnitPriceCurrency: "USD", ContactName: "Unknown Co"},
	}
	view := buildBilling("2026-09", lines, map[string]string{"T1": "Acme Dental"}, map[string]int{"T1": 8})
	if view.Customers != 2 || view.Lines != 3 {
		t.Fatalf("customers/lines = %d/%d", view.Customers, view.Lines)
	}
	var acme *billingCustomer
	for i := range view.Items {
		if view.Items[i].AccountID == "T1" {
			acme = &view.Items[i]
		}
	}
	if acme == nil || !acme.Matched || acme.TenantName != "Acme Dental" {
		t.Fatalf("T1 not joined: %+v", view.Items)
	}
	if acme.Revenue["USD"] != 35 || acme.Revenue["EUR"] != 20 {
		t.Fatalf("revenue must stay per currency, got %v", acme.Revenue)
	}
	if acme.Variance == nil || *acme.Variance != 4 {
		t.Fatalf("billable 12 - deployed 8 = 4, got %v", acme.Variance)
	}
	if len(view.Unmatched) != 1 || view.Unmatched[0] != "X9" {
		t.Fatalf("unmatched = %v, want [X9]", view.Unmatched)
	}
	if view.TotalsCurrency["USD"] != 135 {
		t.Fatalf("USD total = %v", view.TotalsCurrency["USD"])
	}
	empty := buildBilling("2026-09", nil, nil, nil)
	if empty.Items == nil || len(empty.Items) != 0 {
		t.Fatal("empty report must give [] items, not null")
	}
}

func TestSiemPageParamsWindow(t *testing.T) {
	p, stale := siemPageParams(nil, 1000, testNow)
	if stale || p["cursor"] != "" || p["from_date"] == "" {
		t.Fatalf("no cursor: %v stale=%v", p, stale)
	}
	fresh := &siemCursor{Cursor: "abc", SavedAt: testNow.Add(-2 * time.Hour)}
	if p, stale = siemPageParams(fresh, 500, testNow); stale || p["cursor"] != "abc" || p["from_date"] != "" {
		t.Fatalf("fresh cursor: %v stale=%v", p, stale)
	}
	old := &siemCursor{Cursor: "abc", SavedAt: testNow.Add(-30 * time.Hour)}
	if p, stale = siemPageParams(old, 500, testNow); !stale || p["cursor"] != "" || p["from_date"] == "" {
		t.Fatalf("stale cursor must restart at the window edge and warn: %v stale=%v", p, stale)
	}
}

func TestContainHelpers(t *testing.T) {
	e := indexedEndpoint{ID: "E1", Hostname: "WS-FIN-07", IPv4: []string{"10.20.4.17"}}
	for _, target := range []string{"ws-fin-07", "10.20.4.17", "e1"} {
		if !matchEndpoint(e, target) {
			t.Errorf("%q should match", target)
		}
	}
	if matchEndpoint(e, "WS-FIN-0") || matchEndpoint(e, "10.20.4.1") {
		t.Error("partial hostname/IP must not match")
	}
	raw := func(v any) json.RawMessage { b, _ := json.Marshal(v); return b }
	alerts := []json.RawMessage{
		raw(map[string]any{"id": "A1", "allowedActions": []string{"acknowledge"}, "managedAgent": map[string]any{"id": "E1"}}),
		raw(map[string]any{"id": "A2", "allowedActions": []string{"cleanVirus"}, "managedAgent": map[string]any{"id": "E1"}}),
		raw(map[string]any{"id": "A3", "allowedActions": []string{"acknowledge"}, "managedAgent": map[string]any{"id": "E9"}}),
	}
	got, err := ackableAlerts(alerts, map[string]bool{"E1": true})
	if err != nil || len(got) != 1 || got[0] != "A1" {
		t.Fatalf("ackable = %v (%v), want [A1]", got, err)
	}
	if _, err := ackableAlerts(append(alerts, raw(map[string]any{"id": "A4", "managedAgent": "E1"})), map[string]bool{"E1": true}); err == nil {
		t.Fatal("an unreadable alert must fail planning, not be skipped")
	}
}

func TestFleetParsers(t *testing.T) {
	if _, err := parseFleetResources("endpoints,bogus"); err == nil {
		t.Error("unknown resource must be rejected")
	}
	if r, err := parseFleetResources(" Alerts ,health"); err != nil || len(r) != 2 || r[0] != "alerts" {
		t.Errorf("parse = %v %v", r, err)
	}
	y, m, err := parseBillingMonth("", testNow)
	if err != nil || y != 2026 || m != 9 {
		t.Errorf("default month = %d-%d %v, want 2026-9", y, m, err)
	}
	y, m, _ = parseBillingMonth("", time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC))
	if y != 2025 || m != 12 {
		t.Errorf("January default should be previous December, got %d-%d", y, m)
	}
	if _, _, err := parseBillingMonth("2026/09", testNow); err == nil {
		t.Error("bad month format must be rejected")
	}
}

// storeTenantRows must delete a tenant's vanished rows without touching other
// tenants: the reconcile is partitioned on tenant.id.
func TestStoreTenantRowsReconcilesPerTenant(t *testing.T) {
	db, err := store.OpenWithContext(context.Background(), filepath.Join(t.TempDir(), "fleet.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ep := func(id, tenant string) json.RawMessage {
		b, _ := json.Marshal(map[string]any{"id": id, "hostname": "h-" + id, "tenant": map[string]any{"id": tenant}})
		return b
	}
	if _, err := storeTenantRows(db, "endpoints", "T1", []json.RawMessage{ep("11111111-1111-1111-1111-111111111111", "T1"), ep("22222222-2222-2222-2222-222222222222", "T1")}, true); err != nil {
		t.Fatal(err)
	}
	if _, err := storeTenantRows(db, "endpoints", "T2", []json.RawMessage{ep("33333333-3333-3333-3333-333333333333", "T2")}, true); err != nil {
		t.Fatal(err)
	}
	// Second T1 sync no longer returns endpoint 2222...
	if _, err := storeTenantRows(db, "endpoints", "T1", []json.RawMessage{ep("11111111-1111-1111-1111-111111111111", "T1")}, true); err != nil {
		t.Fatal(err)
	}
	ids := map[string]bool{}
	_ = db.ListScan("endpoints", func(id string, _ json.RawMessage) bool { ids[id] = true; return true })
	if ids["22222222-2222-2222-2222-222222222222"] {
		t.Fatal("vanished T1 endpoint should have been reconciled away")
	}
	if !ids["11111111-1111-1111-1111-111111111111"] || !ids["33333333-3333-3333-3333-333333333333"] {
		t.Fatalf("reconcile removed the wrong rows: %v", ids)
	}
}

func TestWalkHealthChecksIgnoresSnoozedForOverall(t *testing.T) {
	doc := healthDoc("T1", "x", 80, 20, true, true)
	var data map[string]any
	_ = json.Unmarshal(doc.Data, &data)
	var checks []healthCheck
	walkHealthChecks("", data, &checks)
	if len(checks) != 4 {
		t.Fatalf("want 4 scored checks, got %+v", checks)
	}
	if o, ok := overallHealth(checks); !ok || o != 60 {
		t.Fatalf("overall = %d %v, want 60 (server 20 is snoozed)", o, ok)
	}
	if _, ok := overallHealth(nil); ok {
		t.Fatal("no checks must report no data")
	}
}
