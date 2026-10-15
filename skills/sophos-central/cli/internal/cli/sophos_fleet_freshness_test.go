// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/sophos-central/handfixes.json
// under the "fleet-freshness-per-resource" entry.

package cli

import (
	"context"
	"encoding/json"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"sophos-central-pp-cli/internal/store"
)

func TestFleetFreshnessIsOldestTenantInput(t *testing.T) {
	now := time.Now().UTC()
	old := now.Add(-2 * time.Hour)
	ledger := map[string]fleetLedgerRow{
		"T1": {ID: "T1", Status: "ok", Resources: map[string]fleetOutcome{"endpoints": {Status: "ok", At: now}, "alerts": {Status: "ok", At: now}}},
		"T2": {ID: "T2", Status: "ok", Resources: map[string]fleetOutcome{"endpoints": {Status: "ok", At: old}, "alerts": {Status: "ok", At: old}}},
	}
	meta := fleetSyncMeta{FinishedAt: now}
	at, note := fleetFreshness(io.Discard, meta, ledger, nil, time.Hour, "endpoints", "alerts")
	if at == nil || !at.Equal(old) || !strings.Contains(note, "older than") {
		t.Fatalf("a --tenant T1 refresh must not make T2 look fresh: at=%v note=%q", at, note)
	}
	if at, _ := fleetFreshness(io.Discard, meta, ledger, map[string]bool{"T1": true}, time.Hour, "endpoints", "alerts"); at == nil || !at.Equal(now) {
		t.Fatalf("a T1-only view is as fresh as T1: %v", at)
	}
}

func TestSinceOnlyAlertsTenantIsBlind(t *testing.T) {
	ledger := map[string]fleetLedgerRow{
		"T1": {ID: "T1", Status: "ok", Resources: map[string]fleetOutcome{
			"endpoints": {Status: "ok", At: time.Now(), FullAt: time.Now()},
			"alerts":    {Status: "ok", At: time.Now(), Window: "24h0m0s"},
		}},
	}
	blind := blindTenantsFrom(ledger, nil, "endpoints", "alerts")
	if len(blind) != 1 || blind[0].Status != "partial_window" {
		t.Fatalf("a tenant whose alerts were only fetched with --since must be blind: %+v", blind)
	}
	merged := mergeLedgerRow(fleetLedgerRow{Resources: map[string]fleetOutcome{"alerts": {Status: "ok", At: time.Now(), FullAt: time.Now()}}},
		fleetLedgerRow{Resources: map[string]fleetOutcome{"alerts": {Status: "ok", At: time.Now(), Window: "24h0m0s"}}})
	if merged.Resources["alerts"].FullAt.IsZero() {
		t.Fatal("a --since fetch on top of a full one keeps the full fetch time")
	}
}

func TestFailedResourceKeepsLastGoodFetchAndFlagsIt(t *testing.T) {
	old := time.Now().UTC().Add(-time.Hour)
	ledger := map[string]fleetLedgerRow{
		"T1": {ID: "T1", Resources: map[string]fleetOutcome{"health": {Status: "error"}, "endpoints": {Status: "ok", At: old}}},
	}
	at, note := fleetFreshness(io.Discard, fleetSyncMeta{FinishedAt: old}, ledger, nil, 0, "health", "endpoints")
	if !strings.Contains(note, "could not fetch health") {
		t.Fatalf("a resource that never fetched for any tenant must be named: %q", note)
	}
	if at == nil || !at.Equal(old) {
		t.Fatalf("the last good fetch still dates the view: %v", at)
	}
	if got := gapsNeeded("commercial"); strings.Join(got, ",") != "endpoints,licenses" {
		t.Fatalf("commercial gaps do not read health checks: %v", got)
	}
}

func TestFullRefreshPrunesTenantsThatLeftScope(t *testing.T) {
	db, err := store.OpenWithContext(context.Background(), filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	keep, gone := fleetTenant{ID: "T1"}, fleetTenant{ID: "T2"}
	for _, ten := range []fleetTenant{keep, gone} {
		row, _ := json.Marshal(fleetLedgerRow{ID: ten.ID, Status: "ok"})
		if _, _, err := db.UpsertBatch(fleetLedgerType, []json.RawMessage{row}); err != nil {
			t.Fatal(err)
		}
		if err := storeTenantDoc(db, fleetHealthType, ten, json.RawMessage(`{}`)); err != nil {
			t.Fatal(err)
		}
	}
	pruned, err := pruneOutOfScopeTenants(db, []fleetTenant{keep})
	if err != nil || len(pruned) != 1 || pruned[0] != "T2" {
		t.Fatalf("pruned=%v err=%v", pruned, err)
	}
	ledger, _ := loadFleetLedger(db)
	if _, ok := ledger["T2"]; ok || len(ledger) != 1 {
		t.Fatalf("T2 must leave the ledger: %v", ledger)
	}
	if raw, _ := db.Get(fleetHealthType, "T2"); len(raw) > 0 {
		t.Fatal("T2 health must be removed")
	}
	if raw, _ := db.Get(fleetHealthType, "T1"); len(raw) == 0 {
		t.Fatal("T1 health must be kept")
	}
}
