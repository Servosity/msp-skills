// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Behavioural acceptance: whether a fleet-diff baseline is comparable kind by
// kind is decided by what the snapshot recorded when it was written, never
// guessed from which rows it happens to hold. Local and global deployments
// number their ids separately (LocalTargetAssignmentResource.id,
// GlobalTargetAssignmentResource.id), so a baseline that keyed both as one
// kind cannot be compared on deployments at all.

package cli

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"immybot-pp-cli/internal/store"
)

// fleetDiffOut decodes fleet-diff JSON by field name, so these tests read the
// command's contract rather than its Go struct.
type fleetDiffOut struct {
	Baseline    string        `json:"baseline_snapshot"`
	Added       []fleetChange `json:"added"`
	Removed     []fleetChange `json:"removed"`
	Changed     []fleetChange `json:"changed"`
	NotCompared []string      `json:"not_compared"`
	Note        string        `json:"note"`
}

func fleetUpsert(t *testing.T, dbPath string, rows map[string][]map[string]any) {
	t.Helper()
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer db.Close()
	for rt, items := range rows {
		for _, item := range items {
			raw, _ := json.Marshal(item)
			if err := db.Upsert(rt, fmt.Sprintf("%v", item["id"]), raw); err != nil {
				t.Fatalf("upsert %s: %v", rt, err)
			}
		}
	}
}

func fleetDelete(t *testing.T, dbPath, resourceType, id string) {
	t.Helper()
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer db.Close()
	if _, err := db.DB().Exec(`DELETE FROM resources WHERE resource_type = ? AND id = ?`, resourceType, id); err != nil {
		t.Fatalf("delete %s %s: %v", resourceType, id, err)
	}
}

func fleetExec(t *testing.T, dbPath, query string, args ...any) {
	t.Helper()
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer db.Close()
	if _, err := db.DB().Exec(query, args...); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
}

// writeUnversionedFleetSnapshot writes a baseline exactly as immybot v0.1.x
// left one: its own snapshot table, no record of the kinds it tracked.
func writeUnversionedFleetSnapshot(t *testing.T, dbPath, takenAt string, rows []store.ImmyFleetRow) {
	t.Helper()
	fleetExec(t, dbPath, `CREATE TABLE IF NOT EXISTS immy_fleet_snapshots (
		taken_at      TEXT NOT NULL,
		resource_type TEXT NOT NULL,
		id            TEXT NOT NULL,
		fingerprint   TEXT NOT NULL,
		label         TEXT,
		PRIMARY KEY (taken_at, resource_type, id))`)
	for _, r := range rows {
		fleetExec(t, dbPath, `INSERT INTO immy_fleet_snapshots (taken_at, resource_type, id, fingerprint, label) VALUES (?, ?, ?, ?, ?)`,
			takenAt, r.ResourceType, r.ID, r.Fingerprint, r.Label)
	}
}

func runFleetDiff(t *testing.T, args ...string) fleetDiffOut {
	t.Helper()
	out, err := runCLI(t, append([]string{"fleet-diff", "--json"}, args...)...)
	if err != nil {
		t.Fatalf("fleet-diff %v: %v\n%s", args, err, out)
	}
	return decode[fleetDiffOut](t, out)
}

func hasFleetChange(in []fleetChange, kind, id string) bool {
	for _, c := range in {
		if c.ResourceType == kind && c.ID == id {
			return true
		}
	}
	return false
}

func deploymentChanges(v fleetDiffOut) []fleetChange {
	var out []fleetChange
	for _, c := range slices.Concat(v.Added, v.Removed, v.Changed) {
		if c.ResourceType == "deployment" || c.ResourceType == "global-deployment" {
			out = append(out, c)
		}
	}
	return out
}

// A baseline recorded today, when the instance happened to have no global
// deployment, must still report the first global deployment as added.
func TestFleetDiffNewBaselineWithoutGlobalsListsAddedGlobal(t *testing.T) {
	for _, since := range [][]string{nil, {"--since", "24h"}} {
		dbPath := filepath.Join(t.TempDir(), "immy.db")
		fleetUpsert(t, dbPath, map[string][]map[string]any{
			"target-assignments": {{"id": 5, "targetName": "Chrome (Contoso)", "maintenanceIdentifier": "chrome", "excluded": false}},
		})
		if out, err := runCLI(t, "fleet-diff", "--snapshot", "--json", "--db", dbPath); err != nil {
			t.Fatalf("snapshot: %v\n%s", err, out)
		}
		fleetUpsert(t, dbPath, map[string][]map[string]any{
			"target-assignments-global": {{"id": 7, "targetName": "Office (everyone)", "maintenanceIdentifier": "office", "excluded": false}},
		})
		v := runFleetDiff(t, append(since, "--db", dbPath)...)
		if !hasFleetChange(v.Added, "global-deployment", "7") {
			t.Fatalf("%v: added = %+v, want global-deployment 7 (note %q)", since, v.Added, v.Note)
		}
		if len(v.Added) != 1 || len(v.Removed)+len(v.Changed) != 0 {
			t.Fatalf("%v: added %+v removed %+v changed %+v, want only global-deployment 7 added", since, v.Added, v.Removed, v.Changed)
		}
		if len(v.NotCompared) != 0 || strings.Contains(v.Note, "predates") || strings.Contains(v.Note, "not compared") {
			t.Fatalf("%v: a baseline this version wrote was not fully compared: not_compared %v, note %q", since, v.NotCompared, v.Note)
		}
	}
}

// An unversioned baseline keyed local and global deployments as one kind.
// Even when the current mirror holds no global deployment it cannot be
// compared on deployments: its "deployment 5" may be Global 5 and its
// "deployment 7" a global that was since deleted. Its computer fingerprints
// were built another way too, so computers are not compared either.
func TestFleetDiffUnversionedBaselineNeverComparesDeployments(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "immy.db")
	fleetUpsert(t, dbPath, map[string][]map[string]any{
		"computers":          {{"id": "1", "name": "WS-01", "tenantId": "t1", "online": true}},
		"target-assignments": {{"id": 5, "targetName": "Chrome (Contoso)", "maintenanceIdentifier": "chrome", "excluded": false}},
	})
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	state, _, err := collectFleetState(t.Context(), db)
	_ = db.Close()
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	ws01, ok := state["computer\x001"]
	if !ok {
		t.Fatalf("no computer 1 in %+v", state)
	}
	writeUnversionedFleetSnapshot(t, dbPath, "2026-10-01T00:00:00Z", []store.ImmyFleetRow{
		ws01,
		{ResourceType: "deployment", ID: "5", Fingerprint: "global-5", Label: "Office (everyone)"},
		{ResourceType: "deployment", ID: "7", Fingerprint: "global-7", Label: "Edge (everyone)"},
	})
	fleetUpsert(t, dbPath, map[string][]map[string]any{
		"computers": {{"id": "2", "name": "WS-02", "tenantId": "t1", "online": true}},
	})

	v := runFleetDiff(t, "--db", dbPath)
	if c := deploymentChanges(v); len(c) != 0 {
		t.Fatalf("an unversioned baseline produced deployment changes %+v (note %q)", c, v.Note)
	}
	// v0.1.x fingerprinted computers from $.name/$.online on every roster
	// resource, which paged rows do not carry, so computers are not
	// compared either: a re-baseline, never a fabricated change.
	if !slices.Equal(v.NotCompared, []string{"computer", "deployment", "global-deployment"}) {
		t.Fatalf("not_compared = %v, want [computer deployment global-deployment]", v.NotCompared)
	}
	if len(v.Added) != 0 || len(v.Changed) != 0 || len(v.Removed) != 0 {
		t.Fatalf("added %+v changed %+v removed %+v, want nothing compared", v.Added, v.Changed, v.Removed)
	}
	if !strings.Contains(v.Note, "--snapshot") {
		t.Fatalf("note = %q, want a hint to record a new baseline", v.Note)
	}

	// The diff read the mirror read-only: it recorded nothing.
	db, err = store.Open(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer db.Close()
	var n int
	if err := db.DB().QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name = 'immy_fleet_snapshot_kinds'`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("a read created the snapshot kinds table (n=%d err=%v)", n, err)
	}
}

// A baseline whose recorded shape for a kind differs from this version's, or
// that did not record the kind at all, skips that kind and names it.
func TestFleetDiffBaselineRecordedInAnotherShapeIsNotCompared(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "immy.db")
	fleetUpsert(t, dbPath, map[string][]map[string]any{
		"computers":          {{"id": "1", "name": "WS-01", "tenantId": "t1", "online": true}},
		"target-assignments": {{"id": 5, "targetName": "Chrome (Contoso)", "maintenanceIdentifier": "chrome", "excluded": false}},
	})
	out, err := runCLI(t, "fleet-diff", "--snapshot", "--json", "--db", dbPath)
	if err != nil {
		t.Fatalf("snapshot: %v\n%s", err, out)
	}
	taken := decode[struct {
		Taken string `json:"snapshot_taken"`
	}](t, out).Taken
	fleetExec(t, dbPath, `UPDATE immy_fleet_snapshot_kinds SET shape = 'an older shape' WHERE taken_at = ? AND kind = 'computer'`, taken)
	fleetExec(t, dbPath, `DELETE FROM immy_fleet_snapshot_kinds WHERE taken_at = ? AND kind = 'global-deployment'`, taken)

	fleetUpsert(t, dbPath, map[string][]map[string]any{
		"computers":                 {{"id": "1", "name": "WS-01", "tenantId": "t1", "online": false}, {"id": "2", "name": "WS-02", "tenantId": "t1", "online": true}},
		"target-assignments":        {{"id": 5, "targetName": "Chrome (Contoso)", "maintenanceIdentifier": "chrome", "excluded": true}},
		"target-assignments-global": {{"id": 7, "targetName": "Office (everyone)", "maintenanceIdentifier": "office", "excluded": false}},
	})
	v := runFleetDiff(t, "--db", dbPath)
	if !slices.Equal(v.NotCompared, []string{"computer", "global-deployment"}) {
		t.Fatalf("not_compared = %v, want [computer global-deployment]", v.NotCompared)
	}
	if len(v.Added) != 0 || len(v.Removed) != 0 || len(v.Changed) != 1 || !hasFleetChange(v.Changed, "deployment", "5") {
		t.Fatalf("added %+v removed %+v changed %+v, want only deployment 5 changed", v.Added, v.Removed, v.Changed)
	}
	if !strings.Contains(v.Note, "computer") || !strings.Contains(v.Note, "--snapshot") {
		t.Fatalf("note = %q, want it to name the skipped kinds and how to re-baseline", v.Note)
	}
}

// A snapshot of a fleet with nothing tracked yet is still a baseline: the
// command said it recorded one, so the next diff must use it.
func TestFleetDiffEmptyFleetSnapshotIsABaseline(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "immy.db")
	fleetUpsert(t, dbPath, map[string][]map[string]any{"tenants": {{"id": 1, "name": "Contoso"}}})
	out, err := runCLI(t, "fleet-diff", "--snapshot", "--json", "--db", dbPath)
	if err != nil {
		t.Fatalf("snapshot: %v\n%s", err, out)
	}
	taken := decode[struct {
		Taken string `json:"snapshot_taken"`
	}](t, out).Taken
	fleetUpsert(t, dbPath, map[string][]map[string]any{
		"computers": {{"id": "1", "name": "WS-01", "tenantId": "t1", "online": true}},
	})
	v := runFleetDiff(t, "--db", dbPath)
	if v.Baseline != taken || len(v.Added) != 1 || !hasFleetChange(v.Added, "computer", "1") {
		t.Fatalf("baseline %q (recorded %q), added %+v, note %q: want the empty snapshot as baseline and WS-01 added", v.Baseline, taken, v.Added, v.Note)
	}
}

// Removing a tracked object from a mirror is reported against a versioned
// baseline that held it, so the shape check never hides a real removal.
func TestFleetDiffVersionedBaselineReportsRemovedGlobal(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "immy.db")
	fleetUpsert(t, dbPath, map[string][]map[string]any{
		"target-assignments":        {{"id": 5, "targetName": "Chrome (Contoso)", "maintenanceIdentifier": "chrome"}},
		"target-assignments-global": {{"id": 7, "targetName": "Office (everyone)", "maintenanceIdentifier": "office"}},
	})
	if out, err := runCLI(t, "fleet-diff", "--snapshot", "--json", "--db", dbPath); err != nil {
		t.Fatalf("snapshot: %v\n%s", err, out)
	}
	fleetDelete(t, dbPath, "target-assignments-global", "7")
	v := runFleetDiff(t, "--db", dbPath)
	if len(v.Removed) != 1 || !hasFleetChange(v.Removed, "global-deployment", "7") || len(v.Added)+len(v.Changed) != 0 || len(v.NotCompared) != 0 {
		t.Fatalf("added %+v removed %+v changed %+v not_compared %v, want only global-deployment 7 removed", v.Added, v.Removed, v.Changed, v.NotCompared)
	}
}
