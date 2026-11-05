// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"path/filepath"
	"testing"
)

// Default sync mirrors GET /api/v1/computers (ComputerSearch.online, a
// boolean) and GET /api/v1/computers/paged (ComputerListViewModel.isOnline,
// nullable) for the same machines. fleet-diff fingerprints the one merged
// computer the shared roster builds, so a null in one source never hides the
// other source's state, and one machine is one object.
func TestFleetDiffSeesAComputerGoOfflineWhenAnotherSourceIsNull(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "immy.db")
	plain := func(online bool) map[string]any {
		return map[string]any{"id": 42, "name": "WS-42", "tenantId": 7, "online": online, "excludeFromMaintenance": false}
	}
	paged := map[string]any{"id": 42, "computerName": "WS-42", "tenantId": 7, "tenantName": "Contoso", "isOnline": nil, "excludeFromMaintenance": false}
	fleetUpsert(t, dbPath, map[string][]map[string]any{"computers": {plain(true)}, "computers-paged": {paged}})
	if out, err := runCLI(t, "fleet-diff", "--snapshot", "--json", "--db", dbPath); err != nil {
		t.Fatalf("snapshot: %v\n%s", err, out)
	}
	fleetUpsert(t, dbPath, map[string][]map[string]any{"computers": {plain(false)}})

	v := runFleetDiff(t, "--db", dbPath)
	if len(v.Changed) != 1 || !hasFleetChange(v.Changed, "computer", "42") || len(v.Added)+len(v.Removed) != 0 {
		t.Fatalf("added %+v removed %+v changed %+v, want only computer 42 changed (note %q)", v.Added, v.Removed, v.Changed, v.Note)
	}
	if v.Changed[0].Label != "WS-42" {
		t.Errorf("label = %q, want WS-42", v.Changed[0].Label)
	}
}

// A machine in two roster sources is one object; a machine only one source
// carries is still tracked; reordering which source is read first changes
// nothing, because both feed one merged record.
func TestFleetDiffRosterSourcesAreOneComputer(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "immy.db")
	fleetUpsert(t, dbPath, map[string][]map[string]any{
		"computers":       {{"id": 42, "name": "WS-42", "tenantId": 7, "online": true, "excludeFromMaintenance": false}},
		"computers-paged": {{"id": 42, "computerName": "WS-42", "tenantId": 7, "isOnline": true, "excludeFromMaintenance": false}},
		"computers-dx":    {{"id": 43, "computerName": "WS-43", "tenantId": 7, "isOnline": false, "excludeFromMaintenance": false}},
	})
	if out, err := runCLI(t, "fleet-diff", "--snapshot", "--json", "--db", dbPath); err != nil {
		t.Fatalf("snapshot: %v\n%s", err, out)
	}
	v := runFleetDiff(t, "--db", dbPath)
	if len(v.Added)+len(v.Removed)+len(v.Changed) != 0 {
		t.Fatalf("an unchanged mirror diffed: added %+v removed %+v changed %+v", v.Added, v.Removed, v.Changed)
	}
	fleetUpsert(t, dbPath, map[string][]map[string]any{
		"computers-dx": {{"id": 43, "computerName": "WS-43", "tenantId": 7, "isOnline": true, "excludeFromMaintenance": false}},
	})
	v = runFleetDiff(t, "--db", dbPath)
	if len(v.Changed) != 1 || !hasFleetChange(v.Changed, "computer", "43") {
		t.Fatalf("changed %+v, want computer 43 (only computers-dx carries it)", v.Changed)
	}
}
