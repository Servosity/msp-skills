// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cliutil

import "testing"

// TestCredentialTierPathMatchesEverySpelling: a member path matches however
// it is spelled on the wire, and its neighbours under /api/v1/auth do not.
// internal/mcp's TestCredentialTierMembershipIsExactlyTheTwoEndpoints checks
// the same against every endpoint in the registry.
func TestCredentialTierPathMatchesEverySpelling(t *testing.T) {
	const get = "access.get-get-azure-tenant-auth-details-by-azure-tenant-principal-id"
	const update = "access.create-update-azure-tenant-auth-details"
	members := map[string]string{
		"/api/v1/auth/get-azure-tenant-auth-details/{azureTenantPrincipalId}":             get,
		"/api/v1/auth/get-azure-tenant-auth-details/11111111-1111-1111-1111-111111111111": get,
		"/API/V1/Auth/Get-Azure-Tenant-Auth-Details/abc/":                                 get,
		"/api/v1/auth/get-azure-tenant-auth-details/abc?x=1#frag":                         get,
		"api/v1/auth/get-azure-tenant-auth-details/abc":                                   get,
		"/api/v1//auth/./get-azure-tenant-auth-details/abc":                               get,
		"/api/v1/auth/x/../get-azure-tenant-auth-details/abc":                             get,
		"/api/v1/auth/get-azure-tenant-auth-details%2Fabc":                                get,
		"/api/v1/auth/get-azure-tenant-auth-details%252Fabc":                              get,
		"/api/v1/auth/get-azure-tenant-auth-details/a%2Fb":                                get,
		"/api/v1/auth/%67et-azure-tenant-auth-details/abc":                                get,
		"\\api\\v1\\auth\\get-azure-tenant-auth-details\\abc":                             get,
		"/api/v2/auth/get-azure-tenant-auth-details/abc":                                  get,
		"/base/api/v1/auth/get-azure-tenant-auth-details/abc":                             get,
		"https://acme.immy.bot/api/v1/auth/get-azure-tenant-auth-details/abc":             get,
		"/api/v1/auth/update-azure-tenant-auth-details":                                   update,
		"/api/v1/auth/UPDATE-azure-tenant-auth-details/":                                  update,
		"https://acme.immy.bot/api/v1/auth/update-azure-tenant-auth-details?x=1":          update,
	}
	for path, want := range members {
		ep, ok := CredentialTierByPath(path)
		if !ok || ep.ID != want {
			t.Errorf("CredentialTierByPath(%q) = %q, %v; want %q", path, ep.ID, ok, want)
		}
	}
	for _, path := range []string{
		"",
		"/api/v1/auth",
		"/api/v1/auth/get-ip-addresses",
		"/api/v1/auth/delete-azure-tenant-auth-details",
		"/api/v1/auth/request-access",
		"/api/v1/auth/get-azure-tenant-auth-details",
		"/api/v1/auth/get-azure-tenant-auth-details-extra/abc",
		"/api/v1/auth/update-azure-tenant-auth-details-extra",
		"/api/v1/auth/me/permissions/x/tenants",
		"/api/v1/azure/app-registration-options",
	} {
		if ep, ok := CredentialTierByPath(path); ok {
			t.Errorf("CredentialTierByPath(%q) = %q; want no member", path, ep.ID)
		}
	}
	if ep, ok := CredentialTierFor(" "+get+" ", ""); !ok || ep.ID != get {
		t.Errorf("CredentialTierFor(id) = %q, %v", ep.ID, ok)
	}
	if len(CredentialTier()) != 2 {
		t.Errorf("tier has %d members, want 2", len(CredentialTier()))
	}
}

// TestCredentialTierServedOnlyOutsideMCP: the CLI process serves a member;
// a process the MCP server spawned does not, and its transport refuses every
// spelling while ordinary paths pass.
func TestCredentialTierServedOnlyOutsideMCP(t *testing.T) {
	t.Setenv(MCPChildEnvVar, "")
	if !CredentialTierServedHere() {
		t.Fatal("a terminal CLI process may not serve the tier")
	}
	if err := CredentialTierTransportError("/api/v1/auth/get-azure-tenant-auth-details/abc"); err != nil {
		t.Fatalf("terminal CLI process refused: %v", err)
	}
	t.Setenv(MCPChildEnvVar, "1")
	if CredentialTierServedHere() {
		t.Fatal("an MCP-spawned process may serve the tier")
	}
	if err := CredentialTierTransportError("/api/v1/auth/get-ip-addresses", "https://acme.immy.bot/api/v1/auth/get-azure-tenant-auth-details/abc"); err == nil {
		t.Fatal("an MCP-spawned process was not refused on the resolved URL")
	}
	if err := CredentialTierTransportError("/api/v1/auth/get-ip-addresses"); err != nil {
		t.Fatalf("an ordinary path was refused: %v", err)
	}
}
