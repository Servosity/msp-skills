// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"sophos-central-pp-cli/internal/store"
)

// Codex round 2 (2026-11-02 reprint) findings, one test per class.

func healthDocWith(policy any) json.RawMessage {
	b, _ := json.Marshal(map[string]any{"tenant": map[string]any{"id": "T1"}, "endpoint": map[string]any{
		"protection": map[string]any{"computer": map[string]any{"score": 100, "total": 1, "notFullyProtected": 0, "snoozed": false}}, "policy": policy}})
	return b
}

var scoredPolicy = map[string]any{"computer": map[string]any{"threat-protection": map[string]any{"score": 20, "total": 1, "notOnRecommended": 1, "snoozed": false}}}

// Class "incomplete health read as complete": a delta exists only between two
// snapshots that scored the same checks. A section the API returned as null
// (an unexpected error) in either snapshot, or a check only one of them
// scored, leaves the delta unset with a note naming the reason.
func TestR2HealthDeltaNeedsTwoCompleteComparableSnapshots(t *testing.T) {
	for name, tc := range map[string]struct{ prev, cur any }{
		"previous section failed": {prev: nil, cur: scoredPolicy},
		"current section failed":  {prev: scoredPolicy, cur: nil},
		"check only in current":   {prev: map[string]any{}, cur: scoredPolicy},
	} {
		t.Run(name, func(t *testing.T) {
			f := fleetFixture()
			f.put(fleetHealthPrevType, "T1", tenantHealthDoc{TenantID: "T1", SyncedAt: testNow.Add(-24 * time.Hour), Data: healthDocWith(tc.prev)})
			f.put(fleetHealthType, "T1", tenantHealthDoc{TenantID: "T1", SyncedAt: testNow, Data: healthDocWith(tc.cur)})
			view, err := buildHealth(f, map[string]bool{"T1": true})
			if err != nil {
				t.Fatal(err)
			}
			if len(view.Items) != 1 {
				t.Fatalf("items = %+v", view.Items)
			}
			r := view.Items[0]
			if r.Delta != nil {
				t.Fatalf("delta %d compares snapshots that did not score the same checks: %+v", *r.Delta, r)
			}
			if r.DeltaNote == "" {
				t.Fatalf("an unset delta must say why: %+v", r)
			}
		})
	}
	// Two complete snapshots of the same checks still get their delta.
	f := fleetFixture()
	f.put(fleetHealthPrevType, "T1", tenantHealthDoc{TenantID: "T1", SyncedAt: testNow.Add(-24 * time.Hour), Data: healthDocWith(map[string]any{"computer": map[string]any{"threat-protection": map[string]any{"score": 90, "snoozed": false}}})})
	f.put(fleetHealthType, "T1", tenantHealthDoc{TenantID: "T1", SyncedAt: testNow, Data: healthDocWith(scoredPolicy)})
	view, err := buildHealth(f, map[string]bool{"T1": true})
	if err != nil {
		t.Fatal(err)
	}
	if d := view.Items[0].Delta; d == nil || *d != -70 || view.Items[0].DeltaNote != "" {
		t.Fatalf("comparable snapshots: want delta -70, got %+v", view.Items[0])
	}
}

// Class "missing tenant attribution": rows an older build stored in the fleet
// copies without tenant.id sit outside every per-tenant reconcile. A refresh
// retires them, and because their owner is unknown no tenant's earlier
// refresh vouches for its fleet rows any more: every ledger row goes, so a
// tenant this run does not refresh reads as not refreshed, never as clean.
func TestR2UnattributedLegacyRowsDoNotSurviveRefresh(t *testing.T) {
	db, err := store.OpenWithContext(context.Background(), filepath.Join(t.TempDir(), "fleet.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	const tid, other, eid = "48bebbb0-f7b6-49c5-aec0-315858f73457", "7767a31c-dd05-4096-a721-1a0f3b006b16", "59cf25c1-974a-4899-a84a-1883e68bd498"
	legacy := json.RawMessage(`{"id":"` + eid + `","type":"computer","hostname":"WS1","os":{"platform":"windows","name":"Windows 11"},"tamperProtectionSupported":true,"tamperProtectionEnabled":false}`)
	attributed := json.RawMessage(`{"id":"e2","type":"computer","hostname":"WS2","tenant":{"id":"` + other + `"},"tamperProtectionSupported":true,"tamperProtectionEnabled":false}`)
	for _, rt := range []string{"endpoints", fleetEndpointsType} {
		if _, _, err := db.ReplaceBatch(rt, []json.RawMessage{legacy, attributed}); err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []string{tid, other} {
		raw, _ := json.Marshal(fleetLedgerRow{ID: id, TenantName: id, Status: "ok", SyncedAt: time.Now().Add(-time.Hour)})
		if err := replaceFleetDoc(db, fleetLedgerType, raw); err != nil {
			t.Fatal(err)
		}
	}

	retired, err := retireUnattributedFleetRows(db)
	if err != nil {
		t.Fatal(err)
	}
	if retired != 1 {
		t.Fatalf("retired = %d, want the 1 unattributed fleet row", retired)
	}
	if _, err := storeTenantRows(db, "endpoints", tid, []json.RawMessage{}, true); err != nil {
		t.Fatal(err)
	}
	view, err := buildTriage(db, map[string]bool{"high": true, "medium": true, "low": true}, 14*24*time.Hour, false, time.Now(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range view.Items {
		if r.ID == eid {
			t.Fatalf("an endpoint the refresh no longer returns is still queued (unattributed legacy row): %+v", r)
		}
	}
	ledger, err := loadFleetLedger(db)
	if err != nil {
		t.Fatal(err)
	}
	if len(ledger) != 0 {
		t.Fatalf("ledger rows still vouch for fleet rows after unattributed rows were retired: %+v", ledger)
	}
	// Attributed fleet rows and the plain-sync mirror are not touched.
	if raw, err := db.Get(fleetEndpointsType, "e2"); err != nil || len(raw) == 0 {
		t.Fatalf("attributed fleet row dropped: %v", err)
	}
	if raw, err := db.Get("endpoints", eid); err != nil || len(raw) == 0 {
		t.Fatalf("mirror row dropped: %v", err)
	}
	// A store with no unattributed rows keeps its ledger.
	raw, _ := json.Marshal(fleetLedgerRow{ID: other, TenantName: other, Status: "ok", SyncedAt: time.Now()})
	if err := replaceFleetDoc(db, fleetLedgerType, raw); err != nil {
		t.Fatal(err)
	}
	if n, err := retireUnattributedFleetRows(db); err != nil || n != 0 {
		t.Fatalf("second pass: n=%d err=%v", n, err)
	}
	if ledger, _ := loadFleetLedger(db); len(ledger) != 1 {
		t.Fatalf("a clean store lost its ledger: %+v", ledger)
	}
}
