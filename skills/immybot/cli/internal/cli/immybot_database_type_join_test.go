// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Behavioural acceptance: local and global assignments number their ids
// separately (LocalTargetAssignmentResource / GlobalTargetAssignmentResource),
// and a maintenance action names its assignment by assignmentId plus
// assignmentType (DatabaseType). No reader may join them on a bare id.

package cli

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"immybot-pp-cli/internal/store"
)

func TestDeploymentHealthKeepsLocalAndGlobalAssignmentsApart(t *testing.T) {
	db := seedStore(t, map[string][]map[string]any{
		"target-assignments": {
			{"id": 5, "databaseType": "Local", "targetName": "Chrome (Contoso)", "maintenanceIdentifier": "chrome", "tenantId": 1},
		},
		"target-assignments-global": {
			{"id": 5, "databaseType": "Global", "targetName": "Office (everyone)", "maintenanceIdentifier": "office", "tenantId": nil},
		},
		"maintenance-actions": {
			{"id": 1, "assignmentId": 5, "assignmentType": "Local", "status": "Complete", "statusName": "Complete", "result": "Success", "resultName": "Success"},
			{"id": 2, "assignmentId": 5, "assignmentType": 0, "status": "Complete", "statusName": "Complete", "result": "Failed", "resultName": "Failed"},
			{"id": 3, "assignmentId": 5, "assignmentType": "Global", "status": "Complete", "statusName": "Complete", "result": 2, "resultName": nil},
		},
	})
	out, err := runCLI(t, "deployment-health", "--json", "--db", db)
	if err != nil {
		t.Fatalf("deployment-health: %v\n%s", err, out)
	}
	v := decode[deploymentHealthView](t, out)
	got := map[string]deploymentHealth{}
	for _, d := range v.Deployments {
		got[d.AssignmentType+":"+d.AssignmentID] = d
	}
	if len(v.Deployments) != 2 {
		t.Fatalf("deployments = %d, want 2 (Local 5 and Global 5 are different assignments):\n%s", len(v.Deployments), out)
	}
	if l := got["Local:5"]; l.Succeeded != 1 || l.Failed != 0 || l.TargetName != "Chrome (Contoso)" {
		t.Fatalf("Local 5 = %+v, want 1 success named Chrome (Contoso):\n%s", l, out)
	}
	if g := got["Global:5"]; g.Succeeded != 0 || g.Failed != 2 || g.TargetName != "Office (everyone)" || !g.NeverSucceeded {
		t.Fatalf("Global 5 = %+v, want 2 failures, never succeeded, named Office (everyone):\n%s", g, out)
	}
}

func TestFleetDiffKeepsLocalAndGlobalDeploymentsApart(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "immy.db")
	write := func(local, global map[string]any) {
		db, err := store.Open(dbPath)
		if err != nil {
			t.Fatalf("open store: %v", err)
		}
		defer db.Close()
		for rt, item := range map[string]map[string]any{"target-assignments": local, "target-assignments-global": global} {
			raw, _ := json.Marshal(item)
			if err := db.Upsert(rt, fmt.Sprintf("%v", item["id"]), raw); err != nil {
				t.Fatalf("upsert: %v", err)
			}
		}
	}
	write(map[string]any{"id": 5, "targetName": "Chrome (Contoso)", "maintenanceIdentifier": "chrome", "excluded": false},
		map[string]any{"id": 5, "targetName": "Office (everyone)", "maintenanceIdentifier": "office", "excluded": false})
	if out, err := runCLI(t, "fleet-diff", "--snapshot", "--json", "--db", dbPath); err != nil {
		t.Fatalf("snapshot: %v\n%s", err, out)
	}
	// Only the GLOBAL deployment 5 changes.
	write(map[string]any{"id": 5, "targetName": "Chrome (Contoso)", "maintenanceIdentifier": "chrome", "excluded": false},
		map[string]any{"id": 5, "targetName": "Office (everyone)", "maintenanceIdentifier": "office", "excluded": true})
	out, err := runCLI(t, "fleet-diff", "--json", "--db", dbPath)
	if err != nil {
		t.Fatalf("fleet-diff: %v\n%s", err, out)
	}
	v := decode[fleetDiffView](t, out)
	if v.BaselineCount != 2 || v.CurrentCount != 2 {
		t.Fatalf("baseline %d / current %d objects, want 2 / 2 (local and global deployment 5 are two objects):\n%s", v.BaselineCount, v.CurrentCount, out)
	}
	if len(v.Changed) != 1 || v.Changed[0].ResourceType != "global-deployment" || v.Changed[0].ID != "5" {
		t.Fatalf("changed = %+v, want only global-deployment 5:\n%s", v.Changed, out)
	}
}

func TestFleetDiffPreSplitBaselineFabricatesNoDeploymentChanges(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "immy.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	for rt, item := range map[string]map[string]any{
		"target-assignments":        {"id": 5, "targetName": "Chrome (Contoso)"},
		"target-assignments-global": {"id": 7, "targetName": "Office (everyone)"},
	} {
		raw, _ := json.Marshal(item)
		if err := db.Upsert(rt, fmt.Sprintf("%v", item["id"]), raw); err != nil {
			t.Fatalf("upsert: %v", err)
		}
	}
	_ = db.Close()
	// A baseline written before the split: both stores under kind "deployment".
	writeUnversionedFleetSnapshot(t, dbPath, "2026-10-01T00:00:00Z", []store.ImmyFleetRow{
		{ResourceType: "deployment", ID: "5", Fingerprint: "a", Label: "Chrome (Contoso)"},
		{ResourceType: "deployment", ID: "7", Fingerprint: "b", Label: "Office (everyone)"},
	})
	out, err := runCLI(t, "fleet-diff", "--json", "--db", dbPath)
	if err != nil {
		t.Fatalf("fleet-diff: %v\n%s", err, out)
	}
	v := decode[fleetDiffView](t, out)
	for _, c := range append(append(v.Added, v.Removed...), v.Changed...) {
		if c.ResourceType == "deployment" || c.ResourceType == "global-deployment" {
			t.Fatalf("a pre-split baseline produced a deployment change %+v:\n%s", c, out)
		}
	}
	if !strings.Contains(v.Note, "--snapshot") {
		t.Fatalf("note = %q, want a hint to record a new baseline", v.Note)
	}
}
