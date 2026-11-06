// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Behavioural acceptance for reading a mirrored computer's identity: every
// computer resource the sync stores is read with the field names of the
// response schema its GET returns (spec.json components.schemas), never with
// another resource's. ComputerSearch says name/tenant, ComputerListViewModel
// (computers-paged) says computerName/tenantName, and on a user-affinity row id
// is the affinity while computerId is the computer. The JSON fields are decoded
// into test-local shapes so these tests compile against, and fail on, code that
// does not emit them yet.

package cli

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"immybot-pp-cli/internal/store"
)

type keyedRow struct {
	key string
	row map[string]any
}

// seedKeyed stores rows under an explicit mirror key: the sync keys
// computers-inventory on computerId and computers-user-affinities on the
// affinity id, which seedStore's item["id"] cannot express.
func seedKeyed(t *testing.T, rows map[string][]keyedRow) string {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "immy.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer db.Close()
	for rt, items := range rows {
		for _, it := range items {
			raw, err := json.Marshal(it.row)
			if err != nil {
				t.Fatalf("marshal %s: %v", rt, err)
			}
			if err := db.Upsert(rt, it.key, raw); err != nil {
				t.Fatalf("upsert %s/%s: %v", rt, it.key, err)
			}
		}
	}
	return dbPath
}

type rowDossier struct {
	ComputerID    string `json:"computer_id"`
	ComputerName  string `json:"computer_name"`
	Tenant        string `json:"tenant"`
	TenantID      string `json:"tenant_id"`
	Online        bool   `json:"online"`
	ExcludedMaint bool   `json:"excluded_from_maintenance"`
	SoftwareCount int    `json:"software_count"`
	Note          string `json:"note"`
}

// computerShapeCases is computer 42 "WS-42" in tenant 7 "Contoso", in the
// vendor shape of each mirrored computer resource.
var computerShapeCases = []struct {
	rt, key, label     string
	row                map[string]any
	tenantID, tenant   string
	online, excluded   bool
	wrongIDMustNotFind string
}{
	{rt: "computers", key: "42", row: map[string]any{"id": 42, "name": "WS-42", "tenant": "Contoso", "tenantId": 7,
		"online": true, "excludeFromMaintenance": true, "updatedDate": "2026-10-26T00:00:00Z"},
		tenantID: "7", tenant: "Contoso", online: true, excluded: true},
	{rt: "computers-paged", key: "42", row: map[string]any{"id": 42, "computerName": "WS-42", "tenantName": "Contoso",
		"tenantId": 7, "isOnline": true, "excludeFromMaintenance": true, "deviceId": "6f1c0d1e-0000-4000-8000-000000000042"},
		tenantID: "7", tenant: "Contoso", online: true, excluded: true},
	{rt: "computers-dx", key: "42", row: map[string]any{"id": 42, "computerName": "WS-42", "tenantName": "Contoso",
		"tenantId": 7, "isOnline": true},
		tenantID: "7", tenant: "Contoso", online: true},
	// computers-dx is untyped in the spec: ComputerSearch's names read too.
	{rt: "computers-dx", label: "computers-dx-search-names", key: "42", row: map[string]any{"id": 42, "name": "WS-42", "tenant": "Contoso", "tenantId": 7,
		"online": true, "excludeFromMaintenance": true},
		tenantID: "7", tenant: "Contoso", online: true, excluded: true},
	{rt: "computers-agent-status", key: "42", row: map[string]any{"id": 42, "computerName": "WS-42", "tenantName": "Contoso",
		"tenantId": 7, "allAgentsConnected": true, "someConnected": true, "agents": []any{}},
		tenantID: "7", tenant: "Contoso"},
	{rt: "computers-onboarding", key: "42", row: map[string]any{"id": 42, "computerName": "WS-42", "tenantName": "Contoso",
		"isOnline": true, "onboardingStatus": "Onboarding", "onboardingFailed": false},
		tenant: "Contoso", online: true},
	{rt: "computers-inventory", key: "42", row: map[string]any{"computerId": 42, "computerName": "WS-42", "tenantName": "Contoso",
		"tenantId": 7, "inventoryKey": "WindowsSystemInfo", "success": true, "timestampUtc": "2026-10-26T00:00:00Z"},
		tenantID: "7", tenant: "Contoso"},
	{rt: "computers-user-affinities", key: "900", row: map[string]any{"id": 900, "personId": 5, "personName": "Pat Lee",
		"computerId": 42, "computerName": "WS-42", "tenantId": 7, "tenantName": "Contoso", "date": "2026-10-26T00:00:00Z"},
		tenantID: "7", tenant: "Contoso", wrongIDMustNotFind: "900"},
	{rt: "computers-my", key: "42", row: map[string]any{"id": 42, "name": "WS-42", "isOnline": true}, online: true},
}

// TestComputerDossierReadsEveryComputerShape: a mirror holding only one
// computer resource still resolves the computer, by id and by name, with the
// tenant and flags that resource's schema carries.
func TestComputerDossierReadsEveryComputerShape(t *testing.T) {
	for _, tc := range computerShapeCases {
		// A distinct label per case: a subtest suffix like "#01" lands in
		// t.TempDir(), and '#' ends the path of the read-only file: URI.
		t.Run(firstNonEmpty(tc.label, tc.rt), func(t *testing.T) {
			db := seedKeyed(t, map[string][]keyedRow{tc.rt: {{tc.key, tc.row}}})
			for _, needle := range []string{"WS-42", "ws-42", "42"} {
				out, err := runCLI(t, "computer-dossier", needle, "--json", "--db", db)
				if err != nil {
					t.Fatalf("computer-dossier %s: %v\n%s", needle, err, out)
				}
				v := decode[rowDossier](t, out)
				if v.Note != "" || v.ComputerID != "42" || v.ComputerName != "WS-42" {
					t.Fatalf("%s row, computer-dossier %s = %+v, want computer 42 WS-42:\n%s", tc.rt, needle, v, out)
				}
				if v.TenantID != tc.tenantID || v.Tenant != tc.tenant {
					t.Fatalf("%s row: tenant = %q (id %q), want %q (id %q):\n%s", tc.rt, v.Tenant, v.TenantID, tc.tenant, tc.tenantID, out)
				}
				if v.Online != tc.online || v.ExcludedMaint != tc.excluded {
					t.Fatalf("%s row: online %t excluded %t, want %t %t:\n%s", tc.rt, v.Online, v.ExcludedMaint, tc.online, tc.excluded, out)
				}
			}
			if tc.wrongIDMustNotFind != "" {
				out, _ := runCLI(t, "computer-dossier", tc.wrongIDMustNotFind, "--json", "--db", db)
				if v := decode[rowDossier](t, out); v.Note == "" {
					t.Fatalf("%s: %s is the row's own id, not a computer id, yet it resolved: %+v", tc.rt, tc.wrongIDMustNotFind, v)
				}
			}
		})
	}
}

type rowExplainAssignment struct {
	ID               string `json:"id"`
	ScopeMatched     string `json:"scope_matched"`
	UnverifiedReason string `json:"unverified_reason"`
}

type rowExplain struct {
	ComputerID   string                 `json:"computer_id"`
	ComputerName string                 `json:"computer_name"`
	Tenant       string                 `json:"tenant"`
	TenantID     string                 `json:"tenant_id"`
	Effective    []rowExplainAssignment `json:"effective"`
	Unverified   []rowExplainAssignment `json:"unverified"`
	Note         string                 `json:"note"`
}

// TestAssignmentExplainResolvesPagedComputer is the round-1 input: a mirror
// whose only computer row is a computers-paged ComputerListViewModel.
func TestAssignmentExplainResolvesPagedComputer(t *testing.T) {
	db := seedStore(t, map[string][]map[string]any{
		"computers-paged": {{"id": 42, "computerName": "WS-42", "tenantName": "Contoso", "tenantId": 7}},
		"target-assignments": {{"id": 9001, "databaseType": "Local", "maintenanceType": "GlobalSoftware",
			"maintenanceIdentifier": "chrome", "targetName": "Chrome (tenant)", "targetType": "AllForTenant", "tenantId": 7}},
	})
	out, err := runCLI(t, "assignment-explain", "WS-42", "--data-source", "local", "--json", "--db", db)
	if err != nil {
		t.Fatalf("assignment-explain: %v\n%s", err, out)
	}
	v := decode[rowExplain](t, out)
	if v.ComputerID != "42" || v.ComputerName != "WS-42" || v.TenantID != "7" || v.Tenant != "Contoso" {
		t.Fatalf("identity = %+v, want computer 42 WS-42 in tenant 7 Contoso (ComputerListViewModel fields):\n%s", v, out)
	}
	if len(v.Effective) != 1 || v.Effective[0].ID != "9001" || v.Effective[0].ScopeMatched != "tenant" {
		t.Fatalf("effective = %+v, want the tenant rule 9001:\n%s", v.Effective, out)
	}
}

// TestComputerLookupMergesResourcesByComputerID: computers-my carries no
// tenant; the agent-status row of the same computer does, and fills it.
func TestComputerLookupMergesResourcesByComputerID(t *testing.T) {
	db := seedStore(t, map[string][]map[string]any{
		"computers-my":           {{"id": 42, "name": "WS-42", "isOnline": true}},
		"computers-agent-status": {{"id": 42, "computerName": "WS-42", "tenantName": "Contoso", "tenantId": 7}},
	})
	out, err := runCLI(t, "computer-dossier", "WS-42", "--json", "--db", db)
	if err != nil {
		t.Fatalf("computer-dossier: %v\n%s", err, out)
	}
	if v := decode[rowDossier](t, out); v.TenantID != "7" || v.Tenant != "Contoso" || !v.Online {
		t.Fatalf("dossier = %+v, want tenant 7 Contoso from agent-status and online from computers-my:\n%s", v, out)
	}
}

// TestComputerLookupRefusesAnAmbiguousName: two computers in two tenants share
// a name; the lookup names both ids rather than picking one.
func TestComputerLookupRefusesAnAmbiguousName(t *testing.T) {
	db := seedStore(t, map[string][]map[string]any{
		"computers": {
			{"id": 42, "name": "WS-42", "tenant": "Contoso", "tenantId": 7},
			{"id": 43, "name": "ws-42", "tenant": "Fabrikam", "tenantId": 8},
		},
		"tenants-software-from-inventory-dx": {
			swRow(1, 42, "WS-42", "Contoso", 7, "Google Chrome", "141.0"),
			swRow(2, 43, "ws-42", "Fabrikam", 8, "Google Chrome", "139.0"),
		},
		"target-assignments": {{"id": 9001, "databaseType": "Local", "maintenanceType": "GlobalSoftware",
			"maintenanceIdentifier": "chrome", "targetName": "Chrome (tenant)", "targetType": "AllForTenant", "tenantId": 7}},
	})
	out, err := runCLI(t, "computer-dossier", "WS-42", "--json", "--db", db)
	if err != nil {
		t.Fatalf("computer-dossier: %v\n%s", err, out)
	}
	d := decode[rowDossier](t, out)
	if d.SoftwareCount != 0 || !strings.Contains(d.Note, "42") || !strings.Contains(d.Note, "43") {
		t.Fatalf("dossier = %+v, want no attribution and a note naming ids 42 and 43:\n%s", d, out)
	}
	out, err = runCLI(t, "assignment-explain", "WS-42", "--data-source", "local", "--json", "--db", db)
	if err != nil {
		t.Fatalf("assignment-explain: %v\n%s", err, out)
	}
	e := decode[rowExplain](t, out)
	if len(e.Effective) != 0 || !strings.Contains(e.Note, "42") || !strings.Contains(e.Note, "43") {
		t.Fatalf("explain = %+v, want no resolution and a note naming ids 42 and 43:\n%s", e, out)
	}
	// The id still resolves exactly.
	out, _ = runCLI(t, "computer-dossier", "43", "--json", "--db", db)
	if d := decode[rowDossier](t, out); d.ComputerID != "43" || d.Tenant != "Fabrikam" || d.SoftwareCount != 1 {
		t.Fatalf("computer-dossier 43 = %+v, want Fabrikam's computer with its one install:\n%s", d, out)
	}
}

// TestComputerLookupNeverReadsAnObjectAsAName: computers-dx rows are untyped
// in the spec; a nested tenant object (GetComputerResponse.tenant) is not a
// tenant name.
func TestComputerLookupNeverReadsAnObjectAsAName(t *testing.T) {
	db := seedStore(t, map[string][]map[string]any{
		"computers-dx": {{"id": 42, "computerName": "WS-42", "tenantId": 7, "tenant": map[string]any{"id": 7, "name": "Contoso"}}},
	})
	out, err := runCLI(t, "computer-dossier", "WS-42", "--json", "--db", db)
	if err != nil {
		t.Fatalf("computer-dossier: %v\n%s", err, out)
	}
	if v := decode[rowDossier](t, out); v.ComputerID != "42" || v.TenantID != "7" || strings.Contains(v.Tenant, "{") {
		t.Fatalf("dossier = %+v, want computer 42 in tenant 7 and no JSON object as its tenant name:\n%s", v, out)
	}
}

// TestAssignmentExplainUnknownTenantIsUnverifiedNotDropped: no mirrored row
// gives the computer's tenant id, so a tenant-bound rule cannot be placed. It
// is listed as unverified; a Computer rule naming the machine still applies,
// and one naming another machine still does not.
func TestAssignmentExplainUnknownTenantIsUnverifiedNotDropped(t *testing.T) {
	db := seedStore(t, map[string][]map[string]any{
		"computers-my": {{"id": 42, "name": "WS-42", "isOnline": true}},
		"target-assignments": {
			{"id": 9001, "databaseType": "Local", "maintenanceType": "GlobalSoftware", "maintenanceIdentifier": "chrome",
				"targetName": "Chrome (tenant)", "targetType": "AllForTenant", "tenantId": 7},
			{"id": 9002, "databaseType": "Local", "maintenanceType": "GlobalSoftware", "maintenanceIdentifier": "zoom",
				"targetName": "Zoom (this machine)", "targetType": "Computer", "target": "42", "tenantId": 7},
			{"id": 9003, "databaseType": "Local", "maintenanceType": "GlobalSoftware", "maintenanceIdentifier": "zoom",
				"targetName": "Zoom (another machine)", "targetType": "Computer", "target": "43", "tenantId": 7},
		},
	})
	out, err := runCLI(t, "assignment-explain", "42", "--data-source", "local", "--json", "--db", db)
	if err != nil {
		t.Fatalf("assignment-explain: %v\n%s", err, out)
	}
	v := decode[rowExplain](t, out)
	if len(v.Unverified) != 1 || v.Unverified[0].ID != "9001" || !strings.Contains(v.Unverified[0].UnverifiedReason, "tenant id") {
		t.Fatalf("unverified = %+v, want tenant rule 9001 with a reason naming the missing tenant id:\n%s", v.Unverified, out)
	}
	if len(v.Effective) != 1 || v.Effective[0].ID != "9002" {
		t.Fatalf("effective = %+v, want only the Computer rule naming 42:\n%s", v.Effective, out)
	}
	if !strings.Contains(v.Note, "tenant id") {
		t.Fatalf("note = %q, want it to say the computer's tenant id is not mirrored", v.Note)
	}
}

type rowPsaComputer struct {
	ComputerID   string `json:"computer_id"`
	ComputerName string `json:"computer_name"`
	TenantID     string `json:"tenant_id"`
	TenantName   string `json:"tenant_name"`
}

type rowPsa struct {
	LocalComputers int              `json:"local_computers"`
	Unlinked       []rowPsaComputer `json:"unlinked_computers"`
	Orphaned       []rowPsaComputer `json:"orphaned_provider_assets"`
}

// TestPsaReconcileReadsPagedComputerRows: the local roster read from
// computers-paged carries each computer's name and tenant.
func TestPsaReconcileReadsPagedComputerRows(t *testing.T) {
	db := seedStore(t, map[string][]map[string]any{
		"provider-links": {{"id": 7, "name": "ConnectWise", "providerTypeId": "cw",
			"computers":       []map[string]any{{"id": 42, "computerName": "WS-42", "tenantId": 7, "tenantName": "Contoso"}},
			"providerClients": []map[string]any{{"externalClientId": "cw-1", "externalClientName": "Contoso", "linkedToTenantId": 7}}}},
		"computers-paged": {
			{"id": 42, "computerName": "WS-42", "tenantName": "Contoso", "tenantId": 7},
			{"id": 43, "computerName": "WS-43", "tenantName": "Contoso", "tenantId": 7},
		},
	})
	out, err := runCLI(t, "psa-reconcile", "--json", "--db", db)
	if err != nil {
		t.Fatalf("psa-reconcile: %v\n%s", err, out)
	}
	v := decode[rowPsa](t, out)
	want := rowPsaComputer{ComputerID: "43", ComputerName: "WS-43", TenantID: "7", TenantName: "Contoso"}
	if v.LocalComputers != 2 || len(v.Unlinked) != 1 || v.Unlinked[0] != want {
		t.Fatalf("local %d, unlinked %+v; want 2 local and %+v (ComputerListViewModel fields):\n%s",
			v.LocalComputers, v.Unlinked, want, out)
	}
}

// TestPsaReconcileRosterMergesResourcesByComputerID: the same computer in
// computers and computers-paged is one local computer, and a paged-only
// computer keeps its name and tenant.
func TestPsaReconcileRosterMergesResourcesByComputerID(t *testing.T) {
	db := seedStore(t, map[string][]map[string]any{
		"provider-links": {{"id": 7, "name": "ConnectWise", "computers": []map[string]any{}, "providerClients": []map[string]any{}}},
		"computers":      {{"id": 42, "name": "WS-42", "tenant": "Contoso", "tenantId": 7}},
		"computers-paged": {
			{"id": 42, "computerName": "WS-42", "tenantName": "Contoso", "tenantId": 7},
			{"id": 44, "computerName": "WS-44", "tenantName": "Contoso", "tenantId": 7},
		},
	})
	out, err := runCLI(t, "psa-reconcile", "--json", "--db", db)
	if err != nil {
		t.Fatalf("psa-reconcile: %v\n%s", err, out)
	}
	v := decode[rowPsa](t, out)
	if v.LocalComputers != 2 || len(v.Unlinked) != 2 {
		t.Fatalf("local %d, unlinked %d; want 2 and 2 (computer 42 is in both resources once):\n%s", v.LocalComputers, len(v.Unlinked), out)
	}
	for _, c := range v.Unlinked {
		if c.ComputerName == "" || c.TenantName != "Contoso" {
			t.Fatalf("unlinked %+v lost its name or tenant:\n%s", c, out)
		}
	}
}
