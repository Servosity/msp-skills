// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored (novel file, not generated).
//
// Single source of truth for ThreatLocker's per-tenant request scoping.
//
// Most Portal API data calls are scoped by the ManagedOrganizationId header.
// Before this file the injection lived only in rootFlags.newClient, so the MCP
// server -- which builds its client through newMCPClientFromConfig and never
// touches rootFlags -- issued every live call with no tenant scoping at all.
// An MSP driving the MCP surface got whatever the token's default organization
// was, silently, on a connector whose entire premise is many customer tenants.
//
// Both entry points now call ApplyTenantScopeHeaders, so the CLI and the MCP
// server cannot drift apart again. See skills/threatlocker/handfixes.json
// (multi-tenant-org-header).

package cli

import (
	"os"
	"strings"

	"threatlocker-pp-cli/internal/config"
)

// TenantOrgHeader is the ThreatLocker header that scopes a request to one
// managed organization. OverrideTenantOrgHeader mirrors it; the portal expects
// both on the New API surface.
const (
	TenantOrgHeader         = "ManagedOrganizationId"
	OverrideTenantOrgHeader = "OverrideManagedOrganizationId"
	TenantOrgEnvVar         = "THREATLOCKER_ORG_ID"
)

// ResolveTenantOrg returns the managed-organization GUID to scope requests to:
// the explicit value (from --org) when set, else THREATLOCKER_ORG_ID. Returns
// "" when neither is set, in which case the API uses the token's own
// organization.
func ResolveTenantOrg(explicit string) string {
	if org := strings.TrimSpace(explicit); org != "" {
		return org
	}
	return strings.TrimSpace(os.Getenv(TenantOrgEnvVar))
}

// ApplyTenantScopeHeaders sets the ManagedOrganizationId /
// OverrideManagedOrganizationId headers on cfg so the client sends them on
// every call. A blank org is a no-op, leaving the token's default organization
// in force. Safe to call more than once.
func ApplyTenantScopeHeaders(cfg *config.Config, org string) {
	if cfg == nil || org == "" {
		return
	}
	if cfg.Headers == nil {
		cfg.Headers = map[string]string{}
	}
	cfg.Headers[TenantOrgHeader] = org
	cfg.Headers[OverrideTenantOrgHeader] = org
}
