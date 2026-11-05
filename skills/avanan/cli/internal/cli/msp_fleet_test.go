// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// cli-printing-press: novel-scaffold-test
// Novel command scaffold tests. Keep the wiring smoke test and add behavior cases as needed.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"avanan-pp-cli/internal/store"
)

// TestNovelMspFleetHelpWires smoke-tests that the msp fleet command
// resolves at runtime and renders useful --help output. Catches wiring
// regressions (missing AddCommand, panicking RunE on --help, etc.) before
// review. Keep this smoke test when adding behavior-specific cases.
func TestNovelMspFleetHelpWires(t *testing.T) {
	cmd := RootCmd()
	cmd.SetArgs([]string{"msp", "fleet", "--help"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("msp fleet --help error = %v (novel command not wired correctly?)", err)
	}
	help := out.String()
	for _, want := range []string{"Usage:", "fleet"} {
		if !strings.Contains(help, want) {
			t.Fatalf("msp fleet --help missing %q in output:\n%s", want, help)
		}
	}
}

// TestNovelMspFleetReadsDocumentedTenantShape pins msp fleet to the tenant
// records `sync` actually writes: the msp-tenants partition (GET
// /v1.0/msp/tenants) in the Tenant shape of the vendor's API reference
// (maxLicensedUsers, users, package{}, status{}, addons[]). The shipped command
// read the "msp" partition (usage rows, then MSP partners) and seat keys the API
// never sends, so every tenant showed 0 seats and --over-seats-only was always
// empty.
func TestNovelMspFleetReadsDocumentedTenantShape(t *testing.T) {
	home := withTempLearnHome(t)
	dbPath := filepath.Join(home, "fleet.db")
	db, err := store.OpenWithContext(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	tenant := `{"id":4421,"domain":"contoso.example.com","deploymentMode":"api","maxLicensedUsers":50,"users":57,` +
		`"package":{"id":1,"codeName":"email_pro","displayName":"Email & Collaboration Pro"},` +
		`"status":{"statusCode":"active","description":"Active"},"addons":[{"id":3,"name":"dlp"}]}`
	if err := db.Upsert(mspTenantsResource, "4421", json.RawMessage(tenant)); err != nil {
		t.Fatal(err)
	}
	// A usage row in the old partition must not become a fleet row.
	if err := db.Upsert("msp", "2026-10-01", json.RawMessage(`{"day":"2026-10-01","tenantDomain":"other.example.com","users":9}`)); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()

	stdout, stderr, err := runRootArgs(t, "msp", "fleet", "--db", dbPath, "--json")
	if err != nil {
		t.Fatalf("msp fleet: %v\nstderr: %s", err, stderr)
	}
	var out struct {
		Results fleetReport `json:"results"`
		fleetReport
	}
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("decode %s: %v", stdout, err)
	}
	rep := out.fleetReport
	if len(out.Results.Rows) > 0 {
		rep = out.Results
	}
	if len(rep.Rows) != 1 {
		t.Fatalf("rows = %d, want 1 (the msp-tenants record only): %s", len(rep.Rows), stdout)
	}
	r := rep.Rows[0]
	if r.SeatsLicensed != 50 || r.SeatsUsed != 57 || r.SeatOverage != 7 {
		t.Errorf("seats licensed/used/overage = %d/%d/%d, want 50/57/7", r.SeatsLicensed, r.SeatsUsed, r.SeatOverage)
	}
	if r.Package != "Email & Collaboration Pro" || r.Status != "active" || r.Addons != 1 {
		t.Errorf("package/status/addons = %q/%q/%d", r.Package, r.Status, r.Addons)
	}
	if r.Tenant != "contoso.example.com" || r.TenantID != "4421" {
		t.Errorf("tenant/id = %q/%q", r.Tenant, r.TenantID)
	}
	if rep.OverSeatCount != 1 {
		t.Errorf("tenants_over_seat_count = %d, want 1", rep.OverSeatCount)
	}
}
