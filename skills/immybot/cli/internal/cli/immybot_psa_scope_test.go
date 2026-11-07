// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Behavioural acceptance for psa-reconcile scope on rows shaped like the
// vendor's GetProviderLinkResponse (computers: GetComputerResponse[],
// providerClients: GetProviderClientResponse[]): discrepancies are reported
// only against provider links that matched and carry the roster compared.

package cli

import (
	"strings"
	"testing"
)

func psaFixture(t *testing.T) string {
	t.Helper()
	return seedStore(t, map[string][]map[string]any{
		"provider-links": {
			{"id": 7, "name": "ConnectWise", "providerTypeId": "cw", "computers": []map[string]any{
				{"id": 1, "computerName": "WS-01", "tenantId": 1, "tenantName": "Contoso"},
				{"id": 404, "computerName": "GHOST-01", "tenantId": 1, "tenantName": "Contoso"},
			}, "providerClients": []map[string]any{
				{"providerLinkId": 7, "externalClientId": "cw-1", "externalClientName": "Contoso", "linkedToTenantId": 1},
			}},
			// A link synced without its rosters (computers and clients null).
			{"id": 9, "name": "NinjaOne", "providerTypeId": "ninja", "computers": nil, "providerClients": nil},
		},
		"computers": {
			{"id": 1, "name": "WS-01", "tenantId": 1, "tenant": "Contoso"},
			{"id": 2, "name": "WS-02", "tenantId": 1, "tenant": "Contoso"},
			{"id": 3, "name": "WS-03", "tenantId": 2, "tenant": "Fabrikam"},
		},
		"tenants": {
			{"id": 1, "name": "Contoso"},
			{"id": 2, "name": "Fabrikam"},
		},
	})
}

func TestPsaReconcileUnknownProviderReportsNothing(t *testing.T) {
	db := psaFixture(t)
	out, err := runCLI(t, "psa-reconcile", "--provider", "77", "--json", "--db", db)
	if err != nil {
		t.Fatalf("psa-reconcile: %v\n%s", err, out)
	}
	v := decode[psaReconcileView](t, out)
	if v.ProviderLinksSeen != 0 || len(v.UnlinkedComputers) != 0 || len(v.TenantsWithNoLink) != 0 || len(v.OrphanedAssets) != 0 {
		t.Fatalf("--provider 77 matched nothing, yet reported discrepancies: %+v\n%s", v, out)
	}
	if v.Note == "" {
		t.Fatal("expected a note saying no provider link matched")
	}
	if v.DataSource != "local" {
		t.Fatalf("data_source = %q, want local (the command has no live path)", v.DataSource)
	}
}

func TestPsaReconcileCountsOnlyLocalLinkedComputers(t *testing.T) {
	db := psaFixture(t)
	out, err := runCLI(t, "psa-reconcile", "--provider", "7", "--json", "--db", db)
	if err != nil {
		t.Fatalf("psa-reconcile: %v\n%s", err, out)
	}
	v := decode[psaReconcileView](t, out)
	if v.LinkedComputers != 1 {
		t.Fatalf("linked_computers = %d, want 1 (GHOST-01 is not a local computer)", v.LinkedComputers)
	}
	if v.LinkedComputers+len(v.UnlinkedComputers) != v.LocalComputers {
		t.Fatalf("linked %d + unlinked %d != local %d", v.LinkedComputers, len(v.UnlinkedComputers), v.LocalComputers)
	}
	if len(v.OrphanedAssets) != 1 || v.OrphanedAssets[0].ComputerID != "404" {
		t.Fatalf("orphaned_provider_assets = %+v, want GHOST-01", v.OrphanedAssets)
	}
}

func TestPsaReconcileLinkWithoutRosterClaimsNothing(t *testing.T) {
	db := psaFixture(t)
	out, err := runCLI(t, "psa-reconcile", "--provider", "9", "--json", "--db", db)
	if err != nil {
		t.Fatalf("psa-reconcile: %v\n%s", err, out)
	}
	v := decode[psaReconcileView](t, out)
	if len(v.UnlinkedComputers) != 0 || len(v.TenantsWithNoLink) != 0 {
		t.Fatalf("a provider link with no computer or client roster cannot call computers unlinked or tenants unmapped: %+v\n%s", v, out)
	}
	if v.Note == "" {
		t.Fatal("expected a note saying the matched link carries no roster")
	}
}

func TestPsaReconcileAnyRosterlessLinkStopsTheComputerClaim(t *testing.T) {
	db := psaFixture(t)
	// No --provider: ConnectWise carries a roster, NinjaOne does not. A computer
	// NinjaOne links cannot be told apart from an unlinked one.
	out, err := runCLI(t, "psa-reconcile", "--json", "--db", db)
	if err != nil {
		t.Fatalf("psa-reconcile: %v\n%s", err, out)
	}
	v := decode[psaReconcileView](t, out)
	if len(v.UnlinkedComputers) != 0 || len(v.TenantsWithNoLink) != 0 {
		t.Fatalf("with a matched link lacking its roster, unlinked/unmapped claims are unsound: %+v\n%s", v, out)
	}
	if !strings.Contains(v.Note, "NinjaOne") {
		t.Fatalf("note = %q, want it to name the link that carries no roster", v.Note)
	}
	if len(v.OrphanedAssets) != 1 {
		t.Fatalf("orphaned_provider_assets = %+v, want GHOST-01 (a roster that was read still reports its orphans)", v.OrphanedAssets)
	}
}
