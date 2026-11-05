// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): a tenant routed only by a [headers]
// CustomerSubDomain value is still a distinct tenant for the local store
// (superops-tenant-header-one-source).

package cli

import (
	"os"
	"path/filepath"
	"testing"
)

// The round-1 input: two private configs that name their tenant only in
// [headers] CustomerSubDomain. Tenant B can neither read nor sync into the
// --db tenant A synced.
func TestHeaderOnlyTenantsNeverShareAStore(t *testing.T) {
	a, b := twoTenantFakes(t)
	t.Setenv("SUPEROPS_SUBDOMAIN", "")
	dir := t.TempDir()
	cfgA, cfgB := filepath.Join(dir, "a.toml"), filepath.Join(dir, "b.toml")
	for path, sub := range map[string]string{cfgA: "acme-a", cfgB: "acme-b"} {
		if err := os.WriteFile(path, []byte("[headers]\nCustomerSubDomain = \""+sub+"\"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	shared := filepath.Join(t.TempDir(), "shared.db")
	if out, err := runSuperOpsCLI(t, "--config", cfgA, "sync", "--resources", "tickets", "--db", shared); err != nil {
		t.Fatalf("tenant A sync: %v\n%s", err, out)
	}
	if got := storeTicketRows(t, shared); got != 12 || fakeRequests(a) == 0 {
		t.Fatalf("tenant A synced %d tickets (%d requests), want 12", got, fakeRequests(a))
	}
	out, err := runSuperOpsCLI(t, "--config", cfgB, "stale-tickets", "--days", "1", "--db", shared, "--json")
	assertRefusal(t, "header-only tenant B reading tenant A's --db", out, err, `"T-1"`, `"T-12"`)
	out, err = runSuperOpsCLI(t, "--config", cfgB, "sync", "--resources", "tickets", "--db", shared)
	assertRefusal(t, "header-only tenant B syncing into tenant A's --db", out, err)
	if n := fakeRequests(b); n != 0 {
		t.Fatalf("tenant B reached the API %d times before it was refused", n)
	}
}
