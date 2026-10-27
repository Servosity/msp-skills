// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Behavioural acceptance for tenant propagation on rows shaped like the vendor
// schemas (sources/swagger.json): a deployment
// (LocalTargetAssignmentResource / GlobalTargetAssignmentResource) bound to a
// tenant with propagateToChildTenants true also reaches every descendant
// tenant, and the hierarchy is GetTenantResponse.parentTenantId on the
// mirrored tenants. When the mirror cannot prove a tenant's ancestry the
// commands must say so instead of dropping the rule.

package cli

import (
	"sort"
	"strings"
	"testing"
)

// Decoded into local shapes so the assertions read only the JSON contract.
type propAssignment struct {
	ID               string `json:"id"`
	DatabaseType     string `json:"database_type"`
	ScopeMatched     string `json:"scope_matched"`
	ShadowedBy       string `json:"shadowed_by"`
	UnverifiedReason string `json:"unverified_reason"`
	PropagatedFrom   string `json:"propagated_from_tenant_id"`
}

type propExplainView struct {
	Effective  []propAssignment `json:"effective"`
	Shadowed   []propAssignment `json:"shadowed"`
	Unverified []propAssignment `json:"unverified"`
	Note       string           `json:"note"`
}

func propIndex(in []propAssignment) map[string]propAssignment {
	out := map[string]propAssignment{}
	for _, a := range in {
		out[a.DatabaseType+":"+a.ID] = a
	}
	return out
}

func propExplain(t *testing.T, db, computer string) (propExplainView, string) {
	t.Helper()
	out, err := runCLI(t, "assignment-explain", computer, "--json", "--data-source", "local", "--db", db)
	if err != nil {
		t.Fatalf("assignment-explain: %v\n%s", err, out)
	}
	return decode[propExplainView](t, out), out
}

func propagationAssignments() map[string][]map[string]any {
	return map[string][]map[string]any{
		"target-assignments": {
			{"id": 9101, "maintenanceType": "GlobalSoftware", "maintenanceIdentifier": "chrome",
				"targetName": "Chrome (Parent, propagating)", "targetType": "AllForTenant", "tenantId": 1, "propagateToChildTenants": true},
			{"id": 9102, "maintenanceType": "GlobalSoftware", "maintenanceIdentifier": "edge",
				"targetName": "Edge (Parent only)", "targetType": "AllForTenant", "tenantId": 1, "propagateToChildTenants": false},
			{"id": 9103, "maintenanceType": "GlobalSoftware", "maintenanceIdentifier": "zoom",
				"targetName": "Zoom (Parent, All, propagating)", "targetType": 1, "tenantId": 1, "propagateToChildTenants": true},
			{"id": 9105, "maintenanceType": "GlobalSoftware", "maintenanceIdentifier": "7zip",
				"targetName": "7-Zip (Parent tag, propagating)", "targetType": "TenantTag", "target": "12", "tenantId": 1, "propagateToChildTenants": true},
			{"id": 9106, "maintenanceType": "GlobalSoftware", "maintenanceIdentifier": "slack",
				"targetName": "Slack (Unrelated, propagating)", "targetType": "AllForTenant", "tenantId": 4, "propagateToChildTenants": true},
			// A tenant's own rule and one it inherits for the same item: its own wins.
			{"id": 9107, "maintenanceType": "GlobalSoftware", "maintenanceIdentifier": "chrome",
				"targetName": "Chrome (Child's own)", "targetType": "AllForTenant", "tenantId": 2},
		},
		"target-assignments-global": {
			{"id": 9104, "maintenanceType": "GlobalSoftware", "maintenanceIdentifier": "office",
				"targetName": "Office (SpecificTenant Parent, propagating)", "targetType": "SpecificTenant", "target": "1", "tenantId": nil,
				"propagateToChildTenants": true},
		},
	}
}

func propagationStore(t *testing.T, tenants []map[string]any) string {
	t.Helper()
	rows := propagationAssignments()
	rows["computers"] = []map[string]any{
		{"id": 201, "name": "CHILD-01", "tenant": "Child", "tenantId": 2},
		{"id": 301, "name": "GRAND-01", "tenant": "Grandchild", "tenantId": 3},
	}
	if tenants != nil {
		rows["tenants"] = tenants
	}
	return seedStore(t, rows)
}

func fullTenantTree() []map[string]any {
	return []map[string]any{
		{"id": 1, "name": "Parent", "parentTenantId": nil},
		{"id": 2, "name": "Child", "parentTenantId": 1},
		{"id": 3, "name": "Grandchild", "parentTenantId": 2},
		{"id": 4, "name": "Unrelated", "parentTenantId": nil},
	}
}

func TestAssignmentExplainFollowsTenantPropagation(t *testing.T) {
	db := propagationStore(t, fullTenantTree())
	for _, computer := range []string{"201", "301"} {
		v, out := propExplain(t, db, computer)
		eff, sh, unv := propIndex(v.Effective), propIndex(v.Shadowed), propIndex(v.Unverified)
		// The brief's failing input: AllForTenant on parent 1, propagating, must
		// reach a computer in child 2 (and, multi-level, grandchild 3).
		_, inEff := eff["Local:9101"]
		_, inSh := sh["Local:9101"]
		if !inEff && !inSh {
			t.Fatalf("computer %s: propagating AllForTenant 9101 on parent tenant 1 must reach it:\n%s", computer, out)
		}
		for _, id := range []string{"Local:9103", "Global:9104"} {
			a, ok := eff[id]
			if !ok || a.ScopeMatched != "tenant" || a.PropagatedFrom != "1" {
				t.Fatalf("computer %s: %s must be effective at tenant scope, propagated from tenant 1; got %+v:\n%s", computer, id, a, out)
			}
		}
		if _, ok := unv["Local:9105"]; !ok {
			t.Fatalf("computer %s: propagating TenantTag 9105 must be listed as unverified (tag membership), not dropped:\n%s", computer, out)
		}
		for _, id := range []string{"Local:9102", "Local:9106"} {
			if _, ok := eff[id]; ok {
				t.Fatalf("computer %s: %s must not reach it (non-propagating, or an unrelated tenant):\n%s", computer, id, out)
			}
			if _, ok := unv[id]; ok {
				t.Fatalf("computer %s: %s is decidable from the hierarchy and must not be unverified:\n%s", computer, id, out)
			}
		}
	}

	// In child tenant 2, its own Chrome rule outranks the one inherited from 1.
	v, out := propExplain(t, db, "201")
	if a, ok := propIndex(v.Effective)["Local:9107"]; !ok || a.PropagatedFrom != "" {
		t.Fatalf("child tenant's own rule 9107 must win for chrome; effective = %+v:\n%s", v.Effective, out)
	}
	if s, ok := propIndex(v.Shadowed)["Local:9101"]; !ok || s.ShadowedBy != "9107" {
		t.Fatalf("inherited rule 9101 must be shadowed by the child's own 9107; shadowed = %+v:\n%s", v.Shadowed, out)
	}
	// In grandchild 3 the child's own non-propagating rule does not apply, so
	// the propagating parent rule is the effective one.
	v, out = propExplain(t, db, "301")
	if a, ok := propIndex(v.Effective)["Local:9101"]; !ok || a.PropagatedFrom != "1" {
		t.Fatalf("grandchild: 9101 must be effective, propagated from tenant 1; effective = %+v:\n%s", v.Effective, out)
	}
	if _, ok := propIndex(v.Shadowed)["Local:9107"]; ok {
		t.Fatalf("grandchild: child tenant 2's non-propagating 9107 must not reach tenant 3:\n%s", out)
	}
}

func TestAssignmentExplainUnknownHierarchyIsUnverifiedNotDropped(t *testing.T) {
	cases := map[string][]map[string]any{
		"tenants not synced": nil,
		// Tenant 3's parent row (2) is missing: its ancestry is unknown.
		"parent row missing": {{"id": 1, "name": "Parent"}, {"id": 3, "name": "Grandchild", "parentTenantId": 2}},
		// A parent chain that loops must terminate and stay undecided.
		"parent chain loops": {{"id": 1, "name": "Parent"}, {"id": 3, "name": "Grandchild", "parentTenantId": 5},
			{"id": 5, "name": "Loop A", "parentTenantId": 6}, {"id": 6, "name": "Loop B", "parentTenantId": 5}},
	}
	for name, tenants := range cases {
		t.Run(name, func(t *testing.T) {
			db := propagationStore(t, tenants)
			v, out := propExplain(t, db, "301")
			unv, eff := propIndex(v.Unverified), propIndex(v.Effective)
			for _, id := range []string{"Local:9101", "Local:9103", "Global:9104", "Local:9106"} {
				a, ok := unv[id]
				if !ok {
					t.Fatalf("%s: a propagating rule whose reach the mirror cannot decide must be unverified, not dropped:\n%s", id, out)
				}
				if !strings.Contains(a.UnverifiedReason, "sync --resources tenants") {
					t.Fatalf("%s: unverified_reason = %q, want it to name 'sync --resources tenants'", id, a.UnverifiedReason)
				}
				if _, ok := eff[id]; ok {
					t.Fatalf("%s: an undecided rule must never be effective:\n%s", id, out)
				}
			}
			if _, ok := unv["Local:9102"]; ok {
				t.Fatalf("non-propagating 9102 keeps the exact-tenant rule and must not appear:\n%s", out)
			}
			if !strings.Contains(v.Note, "sync --resources tenants") {
				t.Fatalf("note = %q, want a tenants sync hint", v.Note)
			}
		})
	}
}

type propBlastView struct {
	FleetWide       bool     `json:"fleet_wide"`
	TenantsAffected []string `json:"tenants_affected"`
	ComputerCount   int      `json:"computers_in_affected_tenants"`
	ReachLowerBound bool     `json:"reach_is_lower_bound"`
	Note            string   `json:"note"`
}

func propBlastStore(t *testing.T, propagate bool, tenants []map[string]any) string {
	t.Helper()
	rows := map[string][]map[string]any{
		"scripts-global": {{"id": 312, "name": "Global Chrome Detection", "databaseType": "Global"}},
		"software":       {{"id": 55, "name": "Google Chrome", "databaseType": "Global", "detectionScriptId": 312, "detectionScriptType": "Global"}},
		"target-assignments": {
			{"id": 9001, "maintenanceType": "GlobalSoftware", "maintenanceIdentifier": "55",
				"targetType": "AllForTenant", "targetName": "Chrome (Parent)", "tenantId": 1, "propagateToChildTenants": propagate},
		},
		"computers": {
			{"id": 1, "name": "WS-01", "tenantId": 1}, {"id": 2, "name": "WS-02", "tenantId": 2},
			{"id": 3, "name": "WS-03", "tenantId": 3}, {"id": 4, "name": "WS-04", "tenantId": 4},
		},
	}
	if tenants != nil {
		rows["tenants"] = tenants
	}
	return seedStore(t, rows)
}

func propBlast(t *testing.T, db string) (propBlastView, string) {
	t.Helper()
	out, err := runCLI(t, "script-blast-radius", "312", "--json", "--db", db)
	if err != nil {
		t.Fatalf("script-blast-radius: %v\n%s", err, out)
	}
	v := decode[propBlastView](t, out)
	sort.Strings(v.TenantsAffected)
	return v, out
}

func TestScriptBlastRadiusFollowsTenantPropagation(t *testing.T) {
	v, out := propBlast(t, propBlastStore(t, true, fullTenantTree()))
	if v.FleetWide || strings.Join(v.TenantsAffected, ",") != "Child,Grandchild,Parent" || v.ComputerCount != 3 || v.ReachLowerBound {
		t.Fatalf("fleet_wide %t, tenants %v, computers %d, lower_bound %t; a propagating AllForTenant on Parent reaches "+
			"Parent, Child and Grandchild (3 computers), exactly:\n%s", v.FleetWide, v.TenantsAffected, v.ComputerCount, v.ReachLowerBound, out)
	}

	// Non-propagating: today's exact-tenant reach, and exact.
	v, out = propBlast(t, propBlastStore(t, false, fullTenantTree()))
	if strings.Join(v.TenantsAffected, ",") != "Parent" || v.ComputerCount != 1 || v.ReachLowerBound {
		t.Fatalf("tenants %v, computers %d, lower_bound %t; a non-propagating rule reaches Parent only:\n%s",
			v.TenantsAffected, v.ComputerCount, v.ReachLowerBound, out)
	}
}

func TestScriptBlastRadiusUnknownHierarchyIsALowerBound(t *testing.T) {
	cases := map[string][]map[string]any{
		"tenants not synced": nil,
		// Tenant 3's parent row (2) is missing, so 3 may or may not descend from 1.
		"parent row missing": {{"id": 1, "name": "Parent"}, {"id": 3, "name": "Grandchild", "parentTenantId": 2},
			{"id": 4, "name": "Unrelated"}},
	}
	for name, tenants := range cases {
		t.Run(name, func(t *testing.T) {
			v, out := propBlast(t, propBlastStore(t, true, tenants))
			if !v.ReachLowerBound || !strings.Contains(v.Note, "sync --resources tenants") {
				t.Fatalf("reach_is_lower_bound %t, note %q; a propagating deployment over an incomplete tenant hierarchy "+
					"must say its reach is a lower bound and name the tenants sync:\n%s", v.ReachLowerBound, v.Note, out)
			}
			if v.ComputerCount < 1 {
				t.Fatalf("computers = %d, want at least the bound tenant's own computer:\n%s", v.ComputerCount, out)
			}
		})
	}
	// Non-propagating over the same unsynced tenants: exact, not a lower bound.
	v, out := propBlast(t, propBlastStore(t, false, nil))
	if v.ReachLowerBound || v.ComputerCount != 1 {
		t.Fatalf("lower_bound %t, computers %d; a non-propagating rule needs no hierarchy:\n%s", v.ReachLowerBound, v.ComputerCount, out)
	}
}

func TestScriptBlastRadiusUnresolvedComputerIsALowerBound(t *testing.T) {
	db := seedStore(t, map[string][]map[string]any{
		"scripts-global": {{"id": 312, "name": "Global Chrome Detection", "databaseType": "Global"}},
		"software":       {{"id": 55, "name": "Google Chrome", "databaseType": "Global", "detectionScriptId": 312, "detectionScriptType": "Global"}},
		"target-assignments-global": {
			{"id": 1, "maintenanceType": "GlobalSoftware", "maintenanceIdentifier": "55", "targetType": "Computer", "target": "999", "tenantId": nil},
		},
		"tenants":   {{"id": 1, "name": "Contoso"}},
		"computers": {{"id": 1, "name": "WS-01", "tenantId": 1}},
	})
	v, out := propBlast(t, db)
	if !v.ReachLowerBound || !strings.Contains(v.Note, "999") {
		t.Fatalf("reach_is_lower_bound %t, note %q; a deployment on a computer the mirror does not hold leaves the reach a lower bound:\n%s",
			v.ReachLowerBound, v.Note, out)
	}
}
