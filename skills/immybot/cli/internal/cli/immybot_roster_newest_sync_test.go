// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"path/filepath"
	"testing"

	"immybot-pp-cli/internal/store"
)

// fleetSyncedAt sets when one mirrored row was synced, as a later
// `sync --resources <type>` would.
func fleetSyncedAt(t *testing.T, dbPath, resourceType, id, at string) {
	t.Helper()
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer db.Close()
	if _, err := db.DB().Exec(`UPDATE resources SET synced_at = ? WHERE resource_type = ? AND id = ?`, at, resourceType, id); err != nil {
		t.Fatalf("set synced_at %s/%s: %v", resourceType, id, err)
	}
}

func rosterPlain(online bool) map[string]any {
	return map[string]any{"id": 42, "name": "WS-42", "tenantId": 7, "online": online, "excludeFromMaintenance": false}
}

func rosterPaged(online any) map[string]any {
	return map[string]any{"id": 42, "computerName": "WS-42", "tenantId": 7, "tenantName": "Contoso", "isOnline": online, "excludeFromMaintenance": false}
}

// The extra-round input: computers (ComputerSearch.online) and computers-paged
// (ComputerListViewModel.isOnline) both say online; after the snapshot only
// computers-paged is synced again, and says offline. fleet-diff must report
// the machine changed, whether that sync landed later or in the same second.
func TestFleetDiffSeesAPagedOnlySyncChangeTheMachine(t *testing.T) {
	for _, tc := range []struct{ name, pagedAt string }{
		{"later sync", "2026-10-27T11:00:00Z"},
		{"same second", "2026-10-27T10:00:00Z"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dbPath := filepath.Join(t.TempDir(), "immy.db")
			fleetUpsert(t, dbPath, map[string][]map[string]any{"computers": {rosterPlain(true)}, "computers-paged": {rosterPaged(true)}})
			fleetSyncedAt(t, dbPath, "computers", "42", "2026-10-27T10:00:00Z")
			fleetSyncedAt(t, dbPath, "computers-paged", "42", "2026-10-27T10:00:00Z")
			if out, err := runCLI(t, "fleet-diff", "--snapshot", "--json", "--db", dbPath); err != nil {
				t.Fatalf("snapshot: %v\n%s", err, out)
			}
			fleetUpsert(t, dbPath, map[string][]map[string]any{"computers-paged": {rosterPaged(false)}})
			fleetSyncedAt(t, dbPath, "computers-paged", "42", tc.pagedAt)

			v := runFleetDiff(t, "--db", dbPath)
			if len(v.Changed) != 1 || !hasFleetChange(v.Changed, "computer", "42") || len(v.Added)+len(v.Removed) != 0 {
				t.Fatalf("added %+v removed %+v changed %+v, want only computer 42 changed (note %q)", v.Added, v.Removed, v.Changed, v.Note)
			}
		})
	}
}

// A source first synced after the snapshot that only repeats the merged record
// is no change; one that says something else is.
func TestFleetDiffNewSourceChangesOnlyWhenItDisagrees(t *testing.T) {
	for _, tc := range []struct {
		name   string
		paged  any
		change bool
	}{
		{"agrees", true, false},
		{"null", nil, false},
		{"disagrees", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dbPath := filepath.Join(t.TempDir(), "immy.db")
			fleetUpsert(t, dbPath, map[string][]map[string]any{"computers": {rosterPlain(true)}})
			if out, err := runCLI(t, "fleet-diff", "--snapshot", "--json", "--db", dbPath); err != nil {
				t.Fatalf("snapshot: %v\n%s", err, out)
			}
			fleetUpsert(t, dbPath, map[string][]map[string]any{"computers-paged": {rosterPaged(tc.paged)}})
			v := runFleetDiff(t, "--db", dbPath)
			if got := hasFleetChange(v.Changed, "computer", "42"); got != tc.change || len(v.Added)+len(v.Removed) != 0 {
				t.Fatalf("added %+v removed %+v changed %+v, want computer 42 changed = %t", v.Added, v.Removed, v.Changed, tc.change)
			}
		})
	}
}

// computer-dossier reads the merged record: the newest sync of each field
// wins, and a null in the newer source never hides the older source's value.
func TestComputerDossierShowsTheNewestSyncedSource(t *testing.T) {
	for _, tc := range []struct {
		name             string
		paged            any
		plainAt, pagedAt string
		want             bool
	}{
		{"paged synced later says offline", false, "2026-10-27T10:00:00Z", "2026-10-27T11:00:00Z", false},
		{"plain synced later says online", false, "2026-10-27T11:00:00Z", "2026-10-27T10:00:00Z", true},
		{"paged synced later is null", nil, "2026-10-27T10:00:00Z", "2026-10-27T11:00:00Z", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dbPath := filepath.Join(t.TempDir(), "immy.db")
			fleetUpsert(t, dbPath, map[string][]map[string]any{"computers": {rosterPlain(true)}, "computers-paged": {rosterPaged(tc.paged)}})
			fleetSyncedAt(t, dbPath, "computers", "42", tc.plainAt)
			fleetSyncedAt(t, dbPath, "computers-paged", "42", tc.pagedAt)
			out, err := runCLI(t, "computer-dossier", "42", "--json", "--db", dbPath)
			if err != nil {
				t.Fatalf("computer-dossier: %v\n%s", err, out)
			}
			if v := decode[rowDossier](t, out); v.ComputerID != "42" || v.Online != tc.want {
				t.Fatalf("dossier = %+v, want computer 42 online = %t:\n%s", v, tc.want, out)
			}
		})
	}
}
