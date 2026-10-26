// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
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
//
// From press 4.34.0 the wiring lives here too, not in generated files: the
// --org flag is registered from a novel-command hook and the headers are
// applied from a client hook, which the generated CLI constructor
// (rootFlags.newClient) and the MCP constructor (newMCPClientFromConfig) both
// run. The response cache partitions on these headers natively (client.go
// canonicalTenantSelectingHeaders; TestCacheKeyPartitionsByTenant pins it).

package cli

import (
	"os"
	"strings"

	"github.com/spf13/cobra"

	"threatlocker-pp-cli/internal/client"
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

// tenantOrgFlag is the explicit --org value for this invocation. Only the flag
// filters the local cross-tenant commands (devices health, approvals triage,
// audit drift and the rest); THREATLOCKER_ORG_ID scopes live requests but never
// hides local rows, because an MSP that sets it to its parent organization
// would otherwise see none of its child tenants' rows. The MCP server never
// parses CLI flags, so for it this stays "" and the env var decides.
var tenantOrgFlag string

func init() {
	registerNovelCommand(func(root *cobra.Command, _ *rootFlags) {
		if root.PersistentFlags().Lookup("org") == nil {
			root.PersistentFlags().StringVar(&tenantOrgFlag, "org", "", "ThreatLocker managed organization (tenant) GUID, sent as the ManagedOrganizationId header (env: "+TenantOrgEnvVar+")")
		}
	})
	registerClientHook(func(c *client.Client) error {
		if c != nil {
			ApplyTenantScopeHeaders(c.Config, ResolveTenantOrg(tenantOrgFlag))
		}
		return nil
	})
}
