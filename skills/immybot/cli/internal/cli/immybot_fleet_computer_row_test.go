// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"path/filepath"
	"testing"

	"immybot-pp-cli/internal/store"
)

// TestFleetStateReadsEachComputerShape: fleet-diff fingerprints a paged
// computer (ComputerListViewModel: computerName, isOnline) by its own field
// names, so a machine going offline is a change and its label is its name.
func TestFleetStateReadsEachComputerShape(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "data.db")
	state := func(online bool) store.ImmyFleetRow {
		fleetUpsert(t, dbPath, map[string][]map[string]any{
			"computers-paged": {{"id": 42, "computerName": "WS-42", "tenantName": "Contoso", "tenantId": 7, "isOnline": online, "excludeFromMaintenance": false}},
		})
		db, err := store.Open(dbPath)
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()
		got, _, err := collectFleetState(context.Background(), db)
		if err != nil {
			t.Fatal(err)
		}
		row, ok := got["computer\x0042"]
		if !ok {
			t.Fatalf("no computer row: %v", got)
		}
		return row
	}
	on := state(true)
	if on.Label != "WS-42" {
		t.Errorf("label = %q, want the computerName WS-42", on.Label)
	}
	if off := state(false); off.Fingerprint == on.Fingerprint {
		t.Error("a paged computer going offline did not change its fingerprint")
	}
}
