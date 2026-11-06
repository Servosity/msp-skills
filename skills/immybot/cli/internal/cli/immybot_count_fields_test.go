// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Behavioural acceptance for count fields: each count, in JSON and in the human
// output, counts what its name and the command's help say. A tenant listed as
// behind a floor reports its computers below the floor (distinct computerId,
// DetectedComputerSoftwareResponse), a tenant is its tenantId, and a total is
// never the length of a list --limit capped. The JSON fields are decoded into
// test-local shapes so these tests compile against, and fail on, code that does
// not emit them yet.

package cli

import (
	"strings"
	"testing"
)

type cfLag struct {
	Tenant             string `json:"tenant"`
	TenantID           string `json:"tenant_id"`
	LowestVersion      string `json:"lowest_version"`
	ComputerCount      int    `json:"computer_count"`
	ComputersWithTitle int    `json:"computers_with_title"`
}

type cfBucket struct {
	Version     string   `json:"version"`
	TenantCount int      `json:"tenant_count"`
	Tenants     []string `json:"tenants"`
}

type cfSpread struct {
	ScannedInstalls int        `json:"scanned_installs"`
	MatchedInstalls int        `json:"matched_installs"`
	BelowFloor      int        `json:"computers_below_floor"`
	Versions        []cfBucket `json:"versions"`
	TenantsBehind   []cfLag    `json:"tenants_behind"`
}

// TestVersionSpreadTenantBehindCountsComputersBelowFloor is the round-1 input:
// computers 1 and 2 in tenant 7 at 139 and 140, floor 140.
func TestVersionSpreadTenantBehindCountsComputersBelowFloor(t *testing.T) {
	db := seedStore(t, map[string][]map[string]any{
		"tenants-software-from-inventory-dx": {
			swRow(1, 1, "WS-01", "Contoso", 7, "Chrome", "139"),
			swRow(2, 2, "WS-02", "Contoso", 7, "Chrome", "140"),
		},
	})
	out, err := runCLI(t, "version-spread", "Chrome", "--min-version", "140", "--json", "--db", db)
	if err != nil {
		t.Fatalf("version-spread: %v\n%s", err, out)
	}
	v := decode[cfSpread](t, out)
	if v.BelowFloor != 1 || len(v.TenantsBehind) != 1 {
		t.Fatalf("below %d, tenants_behind %+v; want 1 computer in 1 tenant:\n%s", v.BelowFloor, v.TenantsBehind, out)
	}
	if lag := v.TenantsBehind[0]; lag.ComputerCount != 1 || lag.ComputersWithTitle != 2 || lag.TenantID != "7" {
		t.Fatalf("tenants_behind[0] = %+v, want computer_count 1 (below the floor) of computers_with_title 2, tenant_id 7:\n%s", lag, out)
	}
	human, err := runCLI(t, "version-spread", "Chrome", "--min-version", "140", "--human-friendly", "--db", db)
	if err != nil {
		t.Fatalf("version-spread human: %v\n%s", err, human)
	}
	if !strings.Contains(human, "1 computer(s) below 140 across 1 tenant(s)") || !strings.Contains(human, "(1 of 2 machines below 140)") {
		t.Fatalf("human output must say 1 machine below 140 of the tenant's 2:\n%s", human)
	}
}

// TestVersionSpreadReadsOnlyInstalledSoftwareRows: computers-inventory rows are
// DxComputerInventoryScriptResult, with no softwareName or version; they are
// not installs and must not be scanned as installs.
func TestVersionSpreadReadsOnlyInstalledSoftwareRows(t *testing.T) {
	db := seedKeyed(t, map[string][]keyedRow{
		"tenants-software-from-inventory-dx": {{"1", swRow(1, 1, "WS-01", "Contoso", 7, "Chrome", "139")}},
		"computers-inventory": {
			{"1", map[string]any{"computerId": 1, "computerName": "WS-01", "tenantId": 7, "tenantName": "Contoso",
				"inventoryKey": "WindowsSystemInfo", "success": true, "timestampUtc": "2026-10-26T00:00:00Z"}},
			{"2", map[string]any{"computerId": 2, "computerName": "WS-02", "tenantId": 7, "tenantName": "Contoso",
				"inventoryKey": "WindowsSystemInfo", "success": true, "timestampUtc": "2026-10-26T00:00:00Z"}},
		},
	})
	out, err := runCLI(t, "version-spread", "Chrome", "--json", "--db", db)
	if err != nil {
		t.Fatalf("version-spread: %v\n%s", err, out)
	}
	if v := decode[cfSpread](t, out); v.ScannedInstalls != 1 || v.MatchedInstalls != 1 {
		t.Fatalf("scanned %d matched %d; want 1 and 1 (inventory script results are not installs):\n%s",
			v.ScannedInstalls, v.MatchedInstalls, out)
	}
}

// TestVersionSpreadTenantIsItsTenantID: tenantName is nullable, so one tenant's
// rows can carry its name on some and null on others; two tenants can share a
// name. Neither may change how many tenants there are.
func TestVersionSpreadTenantIsItsTenantID(t *testing.T) {
	noName := swRow(2, 2, "WS-02", "", 7, "Chrome", "139")
	noName["tenantName"] = nil
	db := seedStore(t, map[string][]map[string]any{
		"tenants-software-from-inventory-dx": {
			swRow(1, 1, "WS-01", "Contoso", 7, "Chrome", "139"),
			noName,
			swRow(3, 3, "WS-03", "Contoso", 8, "Chrome", "139"),
		},
	})
	out, err := runCLI(t, "version-spread", "Chrome", "--min-version", "140", "--json", "--db", db)
	if err != nil {
		t.Fatalf("version-spread: %v\n%s", err, out)
	}
	v := decode[cfSpread](t, out)
	if len(v.Versions) != 1 || v.Versions[0].TenantCount != 2 {
		t.Fatalf("versions = %+v, want one version in 2 tenants (7 and 8):\n%s", v.Versions, out)
	}
	byID := map[string]cfLag{}
	for _, l := range v.TenantsBehind {
		byID[l.TenantID] = l
	}
	if len(v.TenantsBehind) != 2 || byID["7"].ComputerCount != 2 || byID["8"].ComputerCount != 1 ||
		byID["7"].Tenant != "Contoso" || byID["8"].Tenant != "Contoso" {
		t.Fatalf("tenants_behind = %+v, want tenant 7 Contoso with 2 computers and tenant 8 Contoso with 1:\n%s", v.TenantsBehind, out)
	}
}

type cfDriftTenant struct {
	Tenant          string `json:"tenant"`
	TenantID        string `json:"tenant_id"`
	ComputersBehind int    `json:"computers_behind"`
}

// TestDriftTenantIsItsTenantID: drift groups by tenant the same way.
func TestDriftTenantIsItsTenantID(t *testing.T) {
	noName := swRow(2, 2, "WS-02", "", 7, "Chrome", "139")
	noName["tenantName"] = nil
	db := seedStore(t, map[string][]map[string]any{
		"tenants-software-from-inventory-dx": {
			swRow(1, 1, "WS-01", "Contoso", 7, "Chrome", "139"),
			noName,
			swRow(3, 3, "WS-03", "Contoso", 8, "Chrome", "139"),
			swRow(4, 4, "WS-04", "Fabrikam", 9, "Chrome", "141"),
		},
	})
	out, err := runCLI(t, "drift", "--json", "--db", db)
	if err != nil {
		t.Fatalf("drift: %v\n%s", err, out)
	}
	v := decode[struct {
		Tenants []cfDriftTenant `json:"tenants"`
	}](t, out)
	byID := map[string]cfDriftTenant{}
	for _, tn := range v.Tenants {
		byID[tn.TenantID] = tn
	}
	if len(v.Tenants) != 3 || byID["7"].ComputersBehind != 2 || byID["7"].Tenant != "Contoso" ||
		byID["8"].ComputersBehind != 1 || byID["8"].Tenant != "Contoso" {
		t.Fatalf("tenants = %+v, want tenant 7 Contoso with 2 behind, tenant 8 Contoso with 1, and Fabrikam:\n%s", v.Tenants, out)
	}
}

type cfDossierAction struct {
	Action string `json:"action"`
	When   string `json:"when"`
}

type cfDossier struct {
	ActionCount   int               `json:"action_count"`
	FailedActions int               `json:"failed_actions"`
	RecentActions []cfDossierAction `json:"recent_actions"`
}

func cfDossierActions() []map[string]any {
	return []map[string]any{
		// 07:00Z written with an offset: as text it sorts after 08:00Z.
		{"id": 1, "computerId": 42, "computerName": "WS-42", "maintenanceDisplayName": "Offset", "status": "Complete",
			"result": "Failed", "resultName": "Failed", "endTime": "2026-08-20T09:00:00+02:00"},
		{"id": 2, "computerId": 42, "computerName": "WS-42", "maintenanceDisplayName": "Newest", "status": "Complete",
			"result": "Success", "resultName": "Success", "endTime": "2026-08-20T08:00:00Z"},
		// No time that parses: as text "pending" sorts above every date.
		{"id": 3, "computerId": 42, "computerName": "WS-42", "maintenanceDisplayName": "Undated", "status": "Pending",
			"startTime": "pending"},
		// The offset-less .NET form (UTC): 07:30Z, between the two.
		{"id": 4, "computerId": 42, "computerName": "WS-42", "maintenanceDisplayName": "Offsetless", "status": "Complete",
			"result": "Success", "resultName": "Success", "endTime": "2026-08-20T07:30:00"},
	}
}

// TestComputerDossierOrdersActionsByInstant: recent_actions is newest first by
// instant (immyActionTimeFields read by the shared time helper), with an
// action whose time does not parse last.
func TestComputerDossierOrdersActionsByInstant(t *testing.T) {
	db := seedStore(t, map[string][]map[string]any{
		"computers":           {{"id": 42, "name": "WS-42", "tenant": "Contoso", "tenantId": 7}},
		"maintenance-actions": cfDossierActions(),
	})
	out, err := runCLI(t, "computer-dossier", "42", "--json", "--limit", "0", "--db", db)
	if err != nil {
		t.Fatalf("computer-dossier: %v\n%s", err, out)
	}
	v := decode[cfDossier](t, out)
	got := make([]string, 0, len(v.RecentActions))
	for _, a := range v.RecentActions {
		got = append(got, a.Action)
	}
	if strings.Join(got, ",") != "Newest,Offsetless,Offset,Undated" {
		t.Fatalf("recent_actions order = %v, want Newest,Offsetless,Offset,Undated (instants, undated last):\n%s", got, out)
	}
}

// TestComputerDossierCountsAreTotals: --limit caps the listed actions, never
// the action total the failures are counted against.
func TestComputerDossierCountsAreTotals(t *testing.T) {
	db := seedStore(t, map[string][]map[string]any{
		"computers":           {{"id": 42, "name": "WS-42", "tenant": "Contoso", "tenantId": 7}},
		"maintenance-actions": cfDossierActions(),
	})
	out, err := runCLI(t, "computer-dossier", "42", "--json", "--limit", "1", "--db", db)
	if err != nil {
		t.Fatalf("computer-dossier: %v\n%s", err, out)
	}
	v := decode[cfDossier](t, out)
	if v.ActionCount != 4 || v.FailedActions != 1 || len(v.RecentActions) != 1 {
		t.Fatalf("action_count %d failed %d listed %d; want 4, 1 and 1 (--limit 1):\n%s",
			v.ActionCount, v.FailedActions, len(v.RecentActions), out)
	}
	human, err := runCLI(t, "computer-dossier", "42", "--limit", "1", "--human-friendly", "--db", db)
	if err != nil {
		t.Fatalf("computer-dossier human: %v\n%s", err, human)
	}
	if !strings.Contains(human, "actions  4 (1 failed)") {
		t.Fatalf("human output must count all 4 actions, not the 1 listed:\n%s", human)
	}
}

type cfPsa struct {
	UnlinkedComputerCount *int             `json:"unlinked_computer_count"`
	Unlinked              []rowPsaComputer `json:"unlinked_computers"`
	OrphanedAssetCount    int              `json:"orphaned_provider_asset_count"`
	Orphaned              []rowPsaComputer `json:"orphaned_provider_assets"`
}

// TestPsaReconcileTotalsAreUncapped: --limit caps the discrepancy lists, never
// the totals, and a split the rosters cannot support is "not computed", not 0.
func TestPsaReconcileTotalsAreUncapped(t *testing.T) {
	db := seedStore(t, map[string][]map[string]any{
		"provider-links": {
			{"id": 7, "name": "ConnectWise", "computers": []map[string]any{
				{"id": 501, "computerName": "GHOST-1"}, {"id": 502, "computerName": "GHOST-2"},
			}, "providerClients": []map[string]any{}},
			{"id": 9, "name": "NinjaOne", "computers": nil, "providerClients": nil},
		},
		"computers": {
			{"id": 1, "name": "WS-01", "tenant": "Contoso", "tenantId": 7},
			{"id": 2, "name": "WS-02", "tenant": "Contoso", "tenantId": 7},
			{"id": 3, "name": "WS-03", "tenant": "Contoso", "tenantId": 7},
		},
	})
	out, err := runCLI(t, "psa-reconcile", "--provider", "7", "--limit", "1", "--json", "--db", db)
	if err != nil {
		t.Fatalf("psa-reconcile: %v\n%s", err, out)
	}
	v := decode[cfPsa](t, out)
	if v.UnlinkedComputerCount == nil || *v.UnlinkedComputerCount != 3 || len(v.Unlinked) != 1 ||
		v.OrphanedAssetCount != 2 || len(v.Orphaned) != 1 {
		t.Fatalf("unlinked count %v (listed %d), orphaned count %d (listed %d); want 3 (1) and 2 (1):\n%s",
			v.UnlinkedComputerCount, len(v.Unlinked), v.OrphanedAssetCount, len(v.Orphaned), out)
	}
	human, err := runCLI(t, "psa-reconcile", "--provider", "7", "--limit", "1", "--human-friendly", "--db", db)
	if err != nil {
		t.Fatalf("psa-reconcile human: %v\n%s", err, human)
	}
	if !strings.Contains(human, "unlinked computers       3") || !strings.Contains(human, "orphaned provider assets 2") {
		t.Fatalf("human output must print the totals, not the capped list lengths:\n%s", human)
	}
	out, err = runCLI(t, "psa-reconcile", "--provider", "9", "--json", "--db", db)
	if err != nil {
		t.Fatalf("psa-reconcile: %v\n%s", err, out)
	}
	if v := decode[cfPsa](t, out); v.UnlinkedComputerCount != nil {
		t.Fatalf("unlinked_computer_count = %d, want null: link 9 carries no computer roster:\n%s", *v.UnlinkedComputerCount, out)
	}
	human, err = runCLI(t, "psa-reconcile", "--provider", "9", "--human-friendly", "--db", db)
	if err != nil {
		t.Fatalf("psa-reconcile human: %v\n%s", err, human)
	}
	if !strings.Contains(human, "unlinked computers       not computed") || strings.Contains(human, "unlinked computers       0") {
		t.Fatalf("human output must say the split was not computed rather than print 0:\n%s", human)
	}
}
