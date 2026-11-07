// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Behavioural acceptance for the version-lag readers (version-spread, drift)
// on rows shaped like the vendor's DetectedComputerSoftwareResponse
// (sources/swagger.json): version is nullable, softwareName is the Windows
// display name, so "Microsoft Edge Update" and "Microsoft Edge WebView2
// Runtime" sit beside "Microsoft Edge" on every machine that has Edge.

package cli

import (
	"strings"
	"testing"
)

func inv(id, computerID int, computer, tenant, title string, version any) map[string]any {
	return map[string]any{"id": id, "computerId": computerID, "computerName": computer, "tenantId": 7,
		"tenantName": tenant, "softwareName": title, "version": version, "detectedAt": "2026-10-26T00:00:00Z"}
}

func TestVersionSpreadUnknownVersionNeverHidesALaggard(t *testing.T) {
	db := seedStore(t, map[string][]map[string]any{
		"tenants-software-from-inventory-dx": {
			// Contoso's first row has no detected version; WS-02 then runs 139.
			inv(1, 1, "WS-01", "Contoso", "Google Chrome", nil),
			inv(2, 2, "WS-02", "Contoso", "Google Chrome", "139.0.7258.66"),
			inv(3, 3, "WS-03", "Fabrikam", "Google Chrome", "141.0.7390.55"),
		},
	})
	out, err := runCLI(t, "version-spread", "Google Chrome", "--min-version", "140", "--json", "--db", db)
	if err != nil {
		t.Fatalf("version-spread: %v\n%s", err, out)
	}
	v := decode[versionSpreadView](t, out)
	if len(v.TenantsBehind) != 1 || v.TenantsBehind[0].Tenant != "Contoso" || v.TenantsBehind[0].LowestVersion != "139.0.7258.66" {
		t.Fatalf("tenants_behind = %+v, want Contoso at 139.0.7258.66 (an unknown version must not mask a known laggard):\n%s", v.TenantsBehind, out)
	}
	if v.BelowFloor != 1 {
		t.Fatalf("computers_below_floor = %d, want 1", v.BelowFloor)
	}
}

func TestVersionSpreadDefaultMatchIsTheTitleNotASubstring(t *testing.T) {
	db := seedStore(t, map[string][]map[string]any{
		"tenants-software-from-inventory-dx": {
			inv(1, 1, "WS-01", "Contoso", "Microsoft Edge", "141.0.3537.57"),
			inv(2, 1, "WS-01", "Contoso", "Microsoft Edge Update", "1.3.195.65"),
			inv(3, 1, "WS-01", "Contoso", "Microsoft Edge WebView2 Runtime", "141.0.3537.57"),
			inv(4, 2, "WS-02", "Contoso", "Microsoft Edge", "141.0.3537.57"),
		},
	})
	out, err := runCLI(t, "version-spread", "Microsoft Edge", "--min-version", "140", "--json", "--db", db)
	if err != nil {
		t.Fatalf("version-spread: %v\n%s", err, out)
	}
	v := decode[versionSpreadView](t, out)
	if v.BelowFloor != 0 || len(v.TenantsBehind) != 0 {
		t.Fatalf("below_floor = %d, tenants_behind = %+v; the Edge updater (1.3.195.65) is not an Edge build:\n%s", v.BelowFloor, v.TenantsBehind, out)
	}
	if v.MatchedInstalls != 2 {
		t.Fatalf("matched_installs = %d, want 2 (only titles named exactly Microsoft Edge):\n%s", v.MatchedInstalls, out)
	}
	if len(v.MatchedTitles) != 1 || v.MatchedTitles[0] != "Microsoft Edge" {
		t.Fatalf("matched_titles = %v, want [Microsoft Edge]:\n%s", v.MatchedTitles, out)
	}

	// Substring matching stays available behind a flag, and says what it matched.
	out, err = runCLI(t, "version-spread", "Microsoft Edge", "--contains", "--json", "--db", db)
	if err != nil {
		t.Fatalf("version-spread --contains: %v\n%s", err, out)
	}
	v = decode[versionSpreadView](t, out)
	if v.MatchedInstalls != 4 || len(v.MatchedTitles) != 3 {
		t.Fatalf("--contains matched %d installs over titles %v, want 4 over 3 titles:\n%s", v.MatchedInstalls, v.MatchedTitles, out)
	}
}

func TestVersionSpreadNoExactTitleNamesTheCandidates(t *testing.T) {
	db := seedStore(t, map[string][]map[string]any{
		"tenants-software-from-inventory-dx": {
			inv(1, 1, "WS-01", "Contoso", "7-Zip 23.01 (x64)", "23.01"),
			inv(2, 2, "WS-02", "Contoso", "7-Zip 19.00 (x64)", "19.00"),
		},
	})
	out, err := runCLI(t, "version-spread", "7-Zip", "--json", "--db", db)
	if err != nil {
		t.Fatalf("version-spread: %v\n%s", err, out)
	}
	v := decode[versionSpreadView](t, out)
	if v.MatchedInstalls != 0 || !strings.Contains(v.Note, "7-Zip 23.01 (x64)") || !strings.Contains(v.Note, "--contains") {
		t.Fatalf("matched_installs = %d, note = %q; want no match and a note naming the candidate titles and --contains", v.MatchedInstalls, v.Note)
	}
}

func TestDriftCountsEveryLaggingComputer(t *testing.T) {
	db := seedStore(t, map[string][]map[string]any{
		"tenants-software-from-inventory-dx": {
			// Contoso holds the fleet's newest Chrome on WS-01, but WS-02 lags.
			inv(1, 1, "WS-01", "Contoso", "Google Chrome", "141.0.7390.55"),
			inv(2, 2, "WS-02", "Contoso", "Google Chrome", "139.0.7258.66"),
			// Fabrikam: two titles a patch behind.
			inv(3, 3, "WS-03", "Fabrikam", "Google Chrome", "141.0.7390.54"),
			inv(4, 3, "WS-03", "Fabrikam", "Mozilla Firefox", "143.0.1"),
			inv(5, 4, "WS-04", "Initech", "Mozilla Firefox", "143.0.2"),
			// Northwind: one title thirteen majors behind.
			inv(6, 5, "WS-05", "Northwind", "Google Chrome", "128.0.6613.120"),
			// A version the inventory could not read is never a lag.
			inv(7, 6, "WS-06", "Initech", "Google Chrome", nil),
		},
	})
	out, err := runCLI(t, "drift", "--json", "--db", db)
	if err != nil {
		t.Fatalf("drift: %v\n%s", err, out)
	}
	v := decode[driftView](t, out)
	byTenant := map[string]driftTenant{}
	order := make([]string, 0, len(v.Tenants))
	for _, tn := range v.Tenants {
		byTenant[tn.Tenant] = tn
		order = append(order, tn.Tenant)
	}
	if c := byTenant["Contoso"]; c.TitlesBehind != 1 || len(c.Behind) != 1 || c.Behind[0].TenantVersion != "139.0.7258.66" || c.Behind[0].ComputersBehind != 1 {
		t.Fatalf("Contoso = %+v, want Chrome behind on 1 computer at its lowest version 139.0.7258.66:\n%s", c, out)
	}
	if byTenant["Initech"].TitlesBehind != 0 {
		t.Fatalf("Initech = %+v, want 0 titles behind (it holds the newest Firefox; its unknown Chrome is not a lag)", byTenant["Initech"])
	}
	// Lag size ranks first: Northwind's major-version lag outranks Fabrikam's two patch lags.
	if len(order) < 2 || order[0] != "Northwind" {
		t.Fatalf("tenant order = %v, want Northwind first (largest lag):\n%s", order, out)
	}
	if n := byTenant["Northwind"]; len(n.Behind) != 1 || n.Behind[0].LagLevel != "major" {
		t.Fatalf("Northwind = %+v, want one title with lag_level major", n)
	}
	if f := byTenant["Fabrikam"]; f.TitlesBehind != 2 || f.Behind[0].LagLevel != "patch" {
		t.Fatalf("Fabrikam = %+v, want two titles with lag_level patch", f)
	}
}

func TestVersionLagKeepsRowsWithNoTenantName(t *testing.T) {
	row := inv(1, 9, "WS-09", "", "Google Chrome", "128.0.6613.120")
	row["tenantName"] = nil // nullable in DetectedComputerSoftwareResponse
	row["tenantId"] = 9
	db := seedStore(t, map[string][]map[string]any{
		"tenants-software-from-inventory-dx": {
			row,
			inv(2, 1, "WS-01", "Contoso", "Google Chrome", "141.0.7390.55"),
		},
	})
	out, err := runCLI(t, "drift", "--json", "--db", db)
	if err != nil {
		t.Fatalf("drift: %v\n%s", err, out)
	}
	found := false
	for _, tn := range decode[driftView](t, out).Tenants {
		if tn.Tenant == "tenant 9" && tn.TitlesBehind == 1 {
			found = true
		}
	}
	if !found {
		t.Fatalf("drift dropped a lagging computer whose tenantName is null:\n%s", out)
	}
	out, err = runCLI(t, "version-spread", "Google Chrome", "--min-version", "140", "--json", "--db", db)
	if err != nil {
		t.Fatalf("version-spread: %v\n%s", err, out)
	}
	v := decode[versionSpreadView](t, out)
	if len(v.TenantsBehind) != 1 || v.TenantsBehind[0].Tenant != "tenant 9" {
		t.Fatalf("tenants_behind = %+v, want tenant 9 (tenantName null, tenantId 9):\n%s", v.TenantsBehind, out)
	}
}
