// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"

	"cipp-pp-cli/internal/cliutil"
	"cipp-pp-cli/internal/store"
)

func TestResourceTypeForEndpoint(t *testing.T) {
	cases := map[string]string{
		"/ListUsers":                     "users",
		"ListUsers":                      "users",
		"/ListConditionalAccessPolicies": "conditionalaccesspolicies",
		"/ListStandards":                 "standards",
		"/ListBpa":                       "bpa",
		"/api/ListLicenses":              "licenses",
		"/ExecSomething":                 "execsomething",
	}
	for in, want := range cases {
		if got := resourceTypeForEndpoint(in); got != want {
			t.Errorf("resourceTypeForEndpoint(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSplitCSVTenantParsing(t *testing.T) {
	got := cliutil.SplitCSV("contoso.onmicrosoft.com, fabrikam.onmicrosoft.com ,,")
	want := []string{"contoso.onmicrosoft.com", "fabrikam.onmicrosoft.com"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("SplitCSV tenant parse = %#v, want %#v", got, want)
	}
}

func TestFanoutCheckpointRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "fanout-listusers.json")

	done := map[string]bool{
		"contoso.onmicrosoft.com":  true,
		"fabrikam.onmicrosoft.com": true,
	}
	if err := writeFanoutCheckpoint(path, "/ListUsers", done); err != nil {
		t.Fatalf("writeFanoutCheckpoint: %v", err)
	}
	got, err := readFanoutCheckpoint(path)
	if err != nil {
		t.Fatalf("readFanoutCheckpoint: %v", err)
	}
	if !got["contoso.onmicrosoft.com"] || !got["fabrikam.onmicrosoft.com"] {
		t.Fatalf("checkpoint round-trip lost tenants: %#v", got)
	}
	if len(got) != 2 {
		t.Fatalf("checkpoint round-trip size = %d, want 2", len(got))
	}

	// Missing file reads as empty, not error.
	missing, err := readFanoutCheckpoint(filepath.Join(dir, "does-not-exist.json"))
	if err != nil {
		t.Fatalf("readFanoutCheckpoint(missing): %v", err)
	}
	if len(missing) != 0 {
		t.Fatalf("missing checkpoint should be empty, got %#v", missing)
	}
}

func TestStampTenant(t *testing.T) {
	items := []json.RawMessage{
		json.RawMessage(`{"userPrincipalName":"a@contoso.com"}`),
		json.RawMessage(`{"userPrincipalName":"b@contoso.com"}`),
	}
	stamped := stampTenant(items, "contoso.onmicrosoft.com")
	if len(stamped) != 2 {
		t.Fatalf("stampTenant len = %d, want 2", len(stamped))
	}
	for _, s := range stamped {
		var obj map[string]any
		if err := json.Unmarshal(s, &obj); err != nil {
			t.Fatalf("unmarshal stamped: %v", err)
		}
		if obj["_tenant"] != "contoso.onmicrosoft.com" {
			t.Fatalf("stamped _tenant = %v, want contoso.onmicrosoft.com", obj["_tenant"])
		}
	}
}

func TestLoadTenantsFromStore(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "data.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.Upsert("tenants", "t1", json.RawMessage(`{"defaultDomainName":"contoso.onmicrosoft.com","displayName":"Contoso","customerId":"cid-1"}`)); err != nil {
		t.Fatalf("upsert t1: %v", err)
	}
	if err := db.Upsert("tenants", "t2", json.RawMessage(`{"defaultDomainName":"fabrikam.onmicrosoft.com","displayName":"Fabrikam","customerId":"cid-2"}`)); err != nil {
		t.Fatalf("upsert t2: %v", err)
	}
	// A tenant with no defaultDomainName must be dropped.
	if err := db.Upsert("tenants", "t3", json.RawMessage(`{"displayName":"Broken"}`)); err != nil {
		t.Fatalf("upsert t3: %v", err)
	}
	db.Close()

	tenants, err := loadTenantsFromStore(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("loadTenantsFromStore: %v", err)
	}
	if len(tenants) != 2 {
		t.Fatalf("loaded %d tenants, want 2: %#v", len(tenants), tenants)
	}
	byDomain := map[string]tenantRef{}
	for _, tr := range tenants {
		byDomain[tr.DefaultDomainName] = tr
	}
	c, ok := byDomain["contoso.onmicrosoft.com"]
	if !ok || c.DisplayName != "Contoso" || c.CustomerID != "cid-1" {
		t.Fatalf("contoso tenant wrong: %#v", c)
	}
}

// TestResolveFanoutEndpoints asserts preset expansion, CSV endpoints, leading-
// slash normalization, de-duplication, and the unknown-preset error.
func TestResolveFanoutEndpoints(t *testing.T) {
	got, err := resolveFanoutEndpoints("/ListUsers", "")
	if err != nil || len(got) != 1 || got[0] != "/ListUsers" {
		t.Fatalf("single endpoint: got %v err %v", got, err)
	}

	got, err = resolveFanoutEndpoints("ListUsers,ListLicenses", "")
	if err != nil || len(got) != 2 || got[0] != "/ListUsers" || got[1] != "/ListLicenses" {
		t.Fatalf("CSV endpoints should normalize a leading slash: got %v err %v", got, err)
	}

	got, err = resolveFanoutEndpoints("", "core")
	if err != nil {
		t.Fatalf("preset core: %v", err)
	}
	if len(got) != len(fanoutPresets["core"]) {
		t.Errorf("preset core expanded to %d endpoints, want %d", len(got), len(fanoutPresets["core"]))
	}

	// Explicit endpoints come first and duplicates collapse.
	got, err = resolveFanoutEndpoints("/ListUsers", "core")
	if err != nil {
		t.Fatalf("endpoint+preset: %v", err)
	}
	if got[0] != "/ListUsers" {
		t.Errorf("explicit --endpoint should lead, got %v", got)
	}
	seen := map[string]int{}
	for _, e := range got {
		seen[e]++
		if seen[e] > 1 {
			t.Errorf("endpoint %q appears twice in %v", e, got)
		}
	}

	if _, err := resolveFanoutEndpoints("", "nope"); err == nil {
		t.Error("an unknown preset must error rather than silently fanning out nothing")
	}

	if got, err := resolveFanoutEndpoints("", ""); err != nil || len(got) != 0 {
		t.Errorf("empty input should yield no endpoints and no error, got %v %v", got, err)
	}
}

// TestFanoutPresetEndpointsAreNormalized asserts every declared preset endpoint
// carries a leading slash, so preset entries and --endpoint values agree on the
// resource_type they derive.
func TestFanoutPresetEndpointsAreNormalized(t *testing.T) {
	for name, endpoints := range fanoutPresets {
		if len(endpoints) == 0 {
			t.Errorf("preset %q is empty", name)
		}
		for _, e := range endpoints {
			if e == "" || e[0] != '/' {
				t.Errorf("preset %q endpoint %q must start with /", name, e)
			}
		}
	}
}
