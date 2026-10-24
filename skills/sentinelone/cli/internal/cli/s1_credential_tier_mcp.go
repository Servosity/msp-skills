// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated). The CLI-side backstop of the MCP server's
// credential-tier choke point (cmd/sentinelone-mcp/s1_credential_tier_mcp.go):
// the mirrored tail and export tools run this CLI as a child process, and
// their resource resolver (resource_paths.go, marked lines, handfixes.json
// secret-tier-never-exposed-to-mcp) refuses a credential-tier resource or
// path here, before any request, so the refusal holds even if the middleware
// is bypassed.
//
// The MCP marker is SENTINELONE_MCP_CHILD=1 (cliutil.IsMCPChildEnv): the
// server sets it on every child it spawns (cobratree mcpChildEnviron, which
// replaces any inherited value, used by the mirrored tools and the recipe
// intents alike). An operator's own terminal run of `tail ranger` has no
// marker and keeps working: that is the operator reading their own tenant.

package cli

import (
	"fmt"

	"sentinelone-pp-cli/internal/cliutil"
)

// mcpCredentialTierPath returns apiPath, or a refusal when this process runs
// under the MCP server and the resource or the resolved path (raw or
// percent-decoded, so an escaped id cannot smuggle a credential path into an
// ordinary detail read) is in the credential tier.
func mcpCredentialTierPath(resource, apiPath string) (string, error) {
	if !cliutil.IsMCPChildEnv() {
		return apiPath, nil
	}
	m, ok := cliutil.CredentialTierResource(resource)
	if !ok {
		m, ok = cliutil.CredentialTierPath(apiPath)
	}
	if !ok {
		return apiPath, nil
	}
	where := m.Resource
	if m.Path != "" {
		where += " (GET " + m.Path + ")"
	}
	return "", fmt.Errorf("refused: credential-tier resource %s is not read under the MCP server (%s=1), and nothing was requested; an MCP agent reads it only through sentinelone_execute with confirm set to boolean true", where, cliutil.MCPChildEnvVar)
}
