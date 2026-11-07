// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"strings"
	"testing"
)

// Class unknown-telemetry-reported-as-healthy, partial facts (Codex r2): a
// PBX whose licence expiry is far but whose maintenance expiry is null
// (Pbx.LicenseStatus and Pbx.SystemStatus, both nullable), or whose licence
// activation is null, is not known to be outside the renewal window: kept,
// with what was not reported named.
func TestFleetLicencesWithinKeepsPartialFacts(t *testing.T) {
	dir := setupFleet(t)
	partial := `{"LicenseActive":true,"ExpirationDate":"2099-01-01T00:00:00Z","MaintenanceExpiresAt":null}`
	writeFixture(t, dir, "acme_LicenseStatus.json", partial)
	writeFixture(t, dir, "acme_SystemStatus.json", partial)
	noActive := `{"LicenseActive":null,"ExpirationDate":"2099-01-01T00:00:00Z","MaintenanceExpiresAt":"2099-01-01T00:00:00Z"}`
	writeFixture(t, dir, "globex_LicenseStatus.json", noActive)
	writeFixture(t, dir, "globex_SystemStatus.json", noActive)
	out, err := runCLI(t, "fleet", "licences", "--only", "acme,globex", "--within", "30d", "--json")
	if err != nil {
		t.Fatalf("fleet licences: %v\n%s", err, out)
	}
	var rows []map[string]any
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out)
	}
	got := map[string]bool{}
	for _, r := range rows {
		got[r["pbx"].(string)] = true
	}
	if !got["acme"] || !got["globex"] {
		t.Fatalf("want acme (maintenance expiry null) and globex (activation null) kept, got %s", out)
	}
	if !strings.Contains(out, "maintenance_expires") || !strings.Contains(out, "license_active") {
		t.Fatalf("want the unreported inputs named, got %s", out)
	}
}

// Maintenance reported current by GetUpdates but with no expiry date in
// SystemStatus, or a PBX that reported no update list at all, is not known
// to be current: --behind keeps it.
func TestFleetVersionsBehindKeepsPartialFacts(t *testing.T) {
	dir := setupFleet(t)
	writeFixture(t, dir, "acme_SystemStatus.json", `{"Version":"20.0.10.1479","MaintenanceExpiresAt":null}`)
	writeFixture(t, dir, "acme_GetUpdates__.json", `{"Entries":[],"IsMaintananceExpired":false,"UpdatesRunning":false}`)
	writeFixture(t, dir, "globex_SystemStatus.json", `{"Version":"20.0.10.1479","MaintenanceExpiresAt":"2099-01-01T00:00:00Z"}`)
	writeFixture(t, dir, "globex_GetUpdates__.json", `{"Entries":null,"IsMaintananceExpired":false,"UpdatesRunning":false}`)
	out, err := runCLI(t, "fleet", "versions", "--only", "acme,globex", "--behind", "--json")
	if err != nil {
		t.Fatalf("fleet versions: %v\n%s", err, out)
	}
	var rows []map[string]any
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out)
	}
	got := map[string]bool{}
	for _, r := range rows {
		got[r["pbx"].(string)] = true
	}
	if !got["acme"] || !got["globex"] {
		t.Fatalf("want acme (maintenance expiry null) and globex (no update list) kept, got %s", out)
	}
}
