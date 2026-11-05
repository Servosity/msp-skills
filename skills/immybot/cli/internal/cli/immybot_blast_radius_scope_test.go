// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Behavioural acceptance for script-blast-radius on rows shaped like the vendor
// schemas (sources/swagger.json): Get{Global,Local}ScriptResponse carry a
// databaseType, every script reference is an <role>ScriptId with its
// <role>ScriptType (maintenance tasks: get/set/test; software: detection,
// install, upgrade, ... and per version), and a deployment names its item by
// maintenanceType + maintenanceIdentifier with a nullable tenantId. Global and
// local scripts, tasks and assignments number their ids separately.

package cli

import (
	"strings"
	"testing"
)

func blastFixture(t *testing.T) string {
	t.Helper()
	return seedStore(t, map[string][]map[string]any{
		"scripts-global": {
			{"id": 312, "name": "Global Chrome Detection", "databaseType": "Global"},
		},
		"scripts-local": {
			{"id": 312, "name": "Local Chrome Fix", "databaseType": "Local", "ownerTenantId": 1},
			{"id": 500, "name": "Local Only", "databaseType": 1, "ownerTenantId": 1},
		},
		"maintenance-tasks": {
			{"id": 10, "name": "Chrome Task", "databaseType": "Global", "getScriptId": 312, "getScriptType": "Global",
				"setScriptId": 312, "setScriptType": "Global", "testScriptId": nil, "testScriptType": "Global"},
		},
		"maintenance-tasks-local": {
			{"id": 77, "name": "Local Chrome Task", "databaseType": "Local", "getScriptId": 312, "getScriptType": "Local",
				"setScriptId": nil, "setScriptType": "Local", "ownerTenantId": 1},
		},
		"software": {
			{"id": 55, "name": "Google Chrome", "identifier": "chrome", "databaseType": "Global",
				"detectionScriptId": 312, "detectionScriptType": "Global", "installScriptId": 900, "installScriptType": "Global",
				"softwareVersions": []map[string]any{{"semanticVersionString": "141.0", "upgradeScriptId": 312, "upgradeScriptType": 0}}},
		},
		"software-local": {
			{"id": 56, "name": "Local App", "databaseType": "Local", "installScriptId": 312, "installScriptType": "Local", "ownerTenantId": 1},
		},
		"target-assignments": {
			{"id": 9001, "databaseType": "Local", "maintenanceType": "GlobalMaintenanceTask", "maintenanceIdentifier": "10",
				"targetType": "AllForTenant", "targetName": "Chrome task (Contoso)", "tenantId": 1},
			{"id": 9002, "databaseType": "Local", "maintenanceType": 5, "maintenanceIdentifier": "77",
				"targetType": "AllForTenant", "targetName": "Local task (Contoso)", "tenantId": 1},
			{"id": 9003, "databaseType": "Local", "maintenanceType": "GlobalSoftware", "maintenanceIdentifier": "55",
				"targetType": "AllForTenant", "targetName": "Chrome (Fabrikam)", "tenantId": 2},
			{"id": 9004, "databaseType": "Local", "maintenanceType": "LocalSoftware", "maintenanceIdentifier": "56",
				"targetType": "AllForTenant", "targetName": "Local App (Contoso)", "tenantId": 1},
		},
		"target-assignments-global": {
			// Same id as local 9001, a different (fleet-wide) deployment.
			{"id": 9001, "databaseType": "Global", "maintenanceType": "GlobalSoftware", "maintenanceIdentifier": "55",
				"targetType": "All", "targetName": "Chrome everywhere", "tenantId": nil},
		},
		"tenants": {
			{"id": 1, "name": "Contoso"}, {"id": 2, "name": "Fabrikam"}, {"id": 3, "name": "Northwind"},
		},
		"computers": {
			{"id": 1, "name": "WS-01", "tenantId": 1}, {"id": 2, "name": "WS-02", "tenantId": 1},
			{"id": 3, "name": "WS-03", "tenantId": 2}, {"id": 4, "name": "WS-04", "tenantId": 3},
		},
	})
}

func TestScriptBlastRadiusJoinsOnDatabaseType(t *testing.T) {
	db := blastFixture(t)
	out, err := runCLI(t, "script-blast-radius", "312", "--scope", "global", "--json", "--db", db)
	if err != nil {
		t.Fatalf("script-blast-radius: %v\n%s", err, out)
	}
	v := decode[blastRadiusView](t, out)
	if v.ScriptName != "Global Chrome Detection" || v.ScriptScope != "Global" {
		t.Fatalf("script = %q (%s), want Global Chrome Detection (Global)", v.ScriptName, v.ScriptScope)
	}
	if len(v.Tasks) != 1 || v.Tasks[0].TaskID != "10" {
		t.Fatalf("consuming_tasks = %+v, want only global task 10 (local task 77 uses LOCAL script 312):\n%s", v.Tasks, out)
	}
	if len(v.Software) != 1 || v.Software[0].SoftwareID != "55" || !strings.Contains(strings.Join(v.Software[0].Roles, ","), "detection") ||
		!strings.Contains(strings.Join(v.Software[0].Roles, ","), "upgrade") {
		t.Fatalf("consuming_software = %+v, want global software 55 with detection and version upgrade roles:\n%s", v.Software, out)
	}
	got := map[string]bool{}
	for _, a := range v.Assignments {
		got[a.DatabaseType+":"+a.ID] = true
	}
	if len(v.Assignments) != 3 || !got["Local:9001"] || !got["Local:9003"] || !got["Global:9001"] {
		t.Fatalf("deployments = %v, want Local:9001 (task 10), Local:9003 and Global:9001 (software 55):\n%s", got, out)
	}
	if !v.FleetWide || v.ComputerCount != 4 || len(v.TenantsAffected) != 3 {
		t.Fatalf("fleet_wide = %t, computers = %d, tenants = %v; a tenant-less All deployment reaches every tenant:\n%s",
			v.FleetWide, v.ComputerCount, v.TenantsAffected, out)
	}
}

func TestScriptBlastRadiusLocalScriptStaysLocal(t *testing.T) {
	db := blastFixture(t)
	out, err := runCLI(t, "script-blast-radius", "Local Chrome Fix", "--json", "--db", db)
	if err != nil {
		t.Fatalf("script-blast-radius: %v\n%s", err, out)
	}
	v := decode[blastRadiusView](t, out)
	if v.ScriptScope != "Local" || len(v.Tasks) != 1 || v.Tasks[0].TaskID != "77" {
		t.Fatalf("scope %q tasks %+v, want Local and task 77 only:\n%s", v.ScriptScope, v.Tasks, out)
	}
	if len(v.Software) != 1 || v.Software[0].SoftwareID != "56" {
		t.Fatalf("consuming_software = %+v, want local software 56 only:\n%s", v.Software, out)
	}
	ids := map[string]bool{}
	for _, a := range v.Assignments {
		ids[a.DatabaseType+":"+a.ID] = true
	}
	if len(v.Assignments) != 2 || !ids["Local:9002"] || !ids["Local:9004"] {
		t.Fatalf("deployments = %v, want Local:9002 and Local:9004:\n%s", ids, out)
	}
	if v.FleetWide || v.ComputerCount != 2 || len(v.TenantsAffected) != 1 || v.TenantsAffected[0] != "Contoso" {
		t.Fatalf("reach = fleet_wide %t, %d computers, tenants %v; want Contoso's 2 computers:\n%s", v.FleetWide, v.ComputerCount, v.TenantsAffected, out)
	}
}

func TestScriptBlastRadiusAmbiguousIDNeedsScope(t *testing.T) {
	db := blastFixture(t)
	out, err := runCLI(t, "script-blast-radius", "312", "--json", "--db", db)
	if err != nil {
		t.Fatalf("script-blast-radius: %v\n%s", err, out)
	}
	v := decode[blastRadiusView](t, out)
	if len(v.Tasks)+len(v.Software)+len(v.Assignments) != 0 || !strings.Contains(v.Note, "--scope") {
		t.Fatalf("an id both a global and a local script carry must not merge their reach; got tasks %d software %d deployments %d, note %q",
			len(v.Tasks), len(v.Software), len(v.Assignments), v.Note)
	}
}

func TestScriptBlastRadiusUnusedNamesTheRealResources(t *testing.T) {
	db := blastFixture(t)
	out, err := runCLI(t, "script-blast-radius", "Local Only", "--json", "--db", db)
	if err != nil {
		t.Fatalf("script-blast-radius: %v\n%s", err, out)
	}
	v := decode[blastRadiusView](t, out)
	for _, want := range []string{"scripts-global", "scripts-local", "maintenance-tasks-local", "software-local", "target-assignments-global"} {
		if !strings.Contains(v.Note, want) {
			t.Fatalf("note = %q, want a sync hint naming %s", v.Note, want)
		}
	}
}

func TestScriptBlastRadiusTenantlessTargetsReachOnlyTheirTarget(t *testing.T) {
	db := seedStore(t, map[string][]map[string]any{
		"scripts-global": {{"id": 312, "name": "Global Chrome Detection", "databaseType": "Global"}},
		"software":       {{"id": 55, "name": "Google Chrome", "databaseType": "Global", "detectionScriptId": 312, "detectionScriptType": "Global"}},
		"target-assignments-global": {
			{"id": 1, "maintenanceType": "GlobalSoftware", "maintenanceIdentifier": "55", "targetType": "SpecificTenant", "target": "2", "tenantId": nil},
			{"id": 2, "maintenanceType": "GlobalSoftware", "maintenanceIdentifier": "55", "targetType": "Computer", "target": "4", "tenantId": nil},
		},
		"tenants":   {{"id": 1, "name": "Contoso"}, {"id": 2, "name": "Fabrikam"}, {"id": 3, "name": "Northwind"}},
		"computers": {{"id": 1, "name": "WS-01", "tenantId": 1}, {"id": 3, "name": "WS-03", "tenantId": 2}, {"id": 4, "name": "WS-04", "tenantId": 3}},
	})
	out, err := runCLI(t, "script-blast-radius", "312", "--json", "--db", db)
	if err != nil {
		t.Fatalf("script-blast-radius: %v\n%s", err, out)
	}
	v := decode[blastRadiusView](t, out)
	if v.FleetWide || strings.Join(v.TenantsAffected, ",") != "Fabrikam,Northwind" || v.ComputerCount != 2 {
		t.Fatalf("fleet_wide %t, tenants %v, computers %d; want Fabrikam (SpecificTenant 2) and Northwind (Computer 4's tenant), 2 computers:\n%s",
			v.FleetWide, v.TenantsAffected, v.ComputerCount, out)
	}
}
