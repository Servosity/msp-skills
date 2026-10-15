// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/sophos-central/handfixes.json
// under the "fleet-freshness-per-resource" entry.

package cli

import (
	"io"
	"strings"
	"testing"
	"time"
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
