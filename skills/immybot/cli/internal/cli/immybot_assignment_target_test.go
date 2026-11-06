// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Behavioural acceptance for assignment-explain on rows shaped like the vendor's
// LocalTargetAssignmentResource / GlobalTargetAssignmentResource
// (sources/swagger.json): targetType is the TargetType enum (string or
// integer), target names the computer for a Computer rule, tenantId is
// nullable, and local and global assignments number their ids independently.

package cli

import (
	"testing"
)

func TestAssignmentExplainEvaluatesEachTargetType(t *testing.T) {
	db := seedStore(t, map[string][]map[string]any{
		"computers": {
			{"id": 4821, "name": "WS-21", "tenant": "Contoso", "tenantId": 7, "online": true},
			{"id": 4822, "name": "WS-22", "tenant": "Contoso", "tenantId": 7, "online": true},
		},
		"target-assignments": {
			{"id": 9001, "databaseType": "Local", "maintenanceType": "GlobalSoftware", "maintenanceIdentifier": "chrome",
				"targetName": "Chrome (tenant)", "targetType": "AllForTenant", "tenantId": 7},
			{"id": 9002, "databaseType": "Local", "maintenanceType": "GlobalSoftware", "maintenanceIdentifier": "chrome",
				"targetName": "Chrome (this machine)", "targetType": "Computer", "target": "4821", "tenantId": 7},
			// A Computer rule for ANOTHER machine in the same tenant never applies here.
			{"id": 9003, "databaseType": "Local", "maintenanceType": 0, "maintenanceIdentifier": "chrome",
				"targetName": "Chrome (WS-22)", "targetType": 2, "target": "4822", "tenantId": 7},
			// Tag membership is not in the mirror: unverified, never a winner.
			{"id": 9004, "databaseType": "Local", "maintenanceType": "GlobalSoftware", "maintenanceIdentifier": "7zip",
				"targetName": "7-Zip (tagged)", "targetType": "TenantTag", "target": "12", "tenantId": 7},
			{"id": 9005, "databaseType": "Local", "maintenanceType": "GlobalSoftware", "maintenanceIdentifier": "7zip",
				"targetName": "7-Zip (tenant)", "targetType": "AllForTenant", "tenantId": 7},
			// Another tenant's tenant-wide rule never applies.
			{"id": 9007, "databaseType": "Local", "maintenanceType": "GlobalSoftware", "maintenanceIdentifier": "office",
				"targetName": "Office (Fabrikam)", "targetType": "AllForTenant", "tenantId": 8},
			{"id": 9010, "databaseType": "Local", "maintenanceType": "GlobalSoftware", "maintenanceIdentifier": "zoom",
				"targetName": "Zoom (this machine)", "targetType": "Computer", "target": "4821", "tenantId": 7,
				"softwareSemanticVersion": map[string]any{"major": 6, "minor": 2, "patch": 1}, "softwareSemanticVersionString": "6.2.1"},
		},
		"target-assignments-global": {
			{"id": 9006, "databaseType": "Global", "maintenanceType": "GlobalSoftware", "maintenanceIdentifier": "office",
				"targetName": "Office (everyone)", "targetType": "All", "tenantId": nil},
			// Same id as local 9010: a different assignment.
			{"id": 9010, "databaseType": "Global", "maintenanceType": "GlobalSoftware", "maintenanceIdentifier": "zoom",
				"targetName": "Zoom (all tenants)", "targetType": "AllTenants", "tenantId": nil},
		},
	})
	out, err := runCLI(t, "assignment-explain", "4821", "--json", "--data-source", "local", "--db", db)
	if err != nil {
		t.Fatalf("assignment-explain: %v\n%s", err, out)
	}
	v := decode[assignmentExplainView](t, out)

	type key struct{ db, id string }
	effective := map[key]resolvedAssignment{}
	for _, a := range v.Effective {
		effective[key{a.DatabaseType, a.ID}] = a
	}
	shadowed := map[key]resolvedAssignment{}
	for _, a := range v.Shadowed {
		shadowed[key{a.DatabaseType, a.ID}] = a
	}
	unverified := map[key]resolvedAssignment{}
	for _, a := range v.Unverified {
		unverified[key{a.DatabaseType, a.ID}] = a
	}
	all := func(k key) bool {
		_, e := effective[k]
		_, s := shadowed[k]
		_, u := unverified[k]
		return e || s || u
	}

	if a, ok := effective[key{"Local", "9002"}]; !ok || a.ScopeMatched != "computer" {
		t.Fatalf("effective = %+v, want Local 9002 (the Computer rule naming 4821) to win:\n%s", v.Effective, out)
	}
	if all(key{"Local", "9003"}) {
		t.Fatalf("Local 9003 targets computer 4822 and must not resolve onto 4821:\n%s", out)
	}
	if s, ok := shadowed[key{"Local", "9001"}]; !ok || s.ShadowedBy != "9002" {
		t.Fatalf("shadowed = %+v, want Local 9001 shadowed by 9002:\n%s", v.Shadowed, out)
	}
	if a, ok := effective[key{"Local", "9001"}]; ok {
		t.Fatalf("AllForTenant 9001 must be shadowed, got effective %+v", a)
	}
	if a, ok := effective[key{"Local", "9005"}]; !ok || a.ScopeMatched != "tenant" {
		t.Fatalf("effective = %+v, want Local 9005 (AllForTenant, scope tenant) for 7zip:\n%s", v.Effective, out)
	}
	if _, ok := unverified[key{"Local", "9004"}]; !ok {
		t.Fatalf("unverified = %+v, want the TenantTag rule 9004 listed as unverified:\n%s", v.Unverified, out)
	}
	if effective[key{"Local", "9005"}].MayBeOverriddenBy == "" {
		t.Fatalf("Local 9005 must say an unverified, more specific rule may override it:\n%s", out)
	}
	if a, ok := effective[key{"Global", "9006"}]; !ok || a.ScopeMatched != "global" {
		t.Fatalf("effective = %+v, want Global 9006 (All, no tenant) as a global rule:\n%s", v.Effective, out)
	}
	if all(key{"Local", "9007"}) {
		t.Fatalf("Local 9007 belongs to tenant 8 and must not resolve onto 4821:\n%s", out)
	}
	if a, ok := effective[key{"Local", "9010"}]; !ok || a.Version != "6.2.1" {
		t.Fatalf("effective = %+v, want Local 9010 with desired_version 6.2.1:\n%s", v.Effective, out)
	}
	if s, ok := shadowed[key{"Global", "9010"}]; !ok || s.ShadowedBy != "9010" || s.ShadowedByType != "Local" {
		t.Fatalf("shadowed = %+v, want Global 9010 shadowed by Local 9010 (ids are per database type):\n%s", v.Shadowed, out)
	}
}
