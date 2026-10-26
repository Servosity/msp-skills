// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored (not generated). The MCP half of the credential tier
// (internal/cliutil/tl_credential_tier.go). A credential-returning read is
// never returned to an MCP client, with or without THREATLOCKER_MCP_READ_ONLY:
//
//   - threatlocker_read and threatlocker_execute refuse a member by id before
//     a client is built, naming the CLI command to run in a terminal;
//   - threatlocker_search lists one only as callable:false with the refusal
//     (no method, no path), and threatlocker_get refuses to describe one, so
//     discovery never offers it as callable;
//   - init marks this process as the MCP server, which makes the client
//     transport refuse every member path for any other tool or future code
//     path (internal/client/tl_credential_tier.go);
//   - the CLI commands carry mcp:hidden, so the cobratree mirror never lists
//     them (internal/cli/tl_credential_tier.go).
//
// Hooked from the generated code_orch.go at handleCodeOrchSearch (result
// shaping), handleCodeOrchGet and codeOrchInvoke (handfixes.json:
// credential-tier-never-cached-never-served-to-mcp).

package mcp

import (
	mcplib "github.com/mark3labs/mcp-go/mcp"

	"threatlocker-pp-cli/internal/cliutil"
)

func init() {
	cliutil.MarkMCPServerProcess()
}

// codeOrchCredentialTierMember reports the tier member an executor endpoint
// reads, by id or by path.
func codeOrchCredentialTierMember(ep *codeOrchEndpoint) (cliutil.CredentialTierEndpoint, bool) {
	if ep == nil {
		return cliutil.CredentialTierEndpoint{}, false
	}
	if member, ok := cliutil.CredentialTierByID(ep.ID); ok {
		return member, true
	}
	return cliutil.CredentialTierByPath(ep.Path)
}

// codeOrchSearchItem is the threatlocker_search result for ep. A member is
// still findable, so an agent asked for an install key can tell its operator
// where to get it, but it is not shaped as an endpoint to call: no method, no
// path, callable false, and the refusal naming the terminal command.
func codeOrchSearchItem(ep *codeOrchEndpoint) map[string]any {
	member, ok := codeOrchCredentialTierMember(ep)
	if !ok {
		return codeOrchEndpointMetadata(ep)
	}
	return map[string]any{
		"endpoint_id":   ep.ID,
		"summary":       ep.Summary,
		"callable":      false,
		"refusal":       cliutil.CredentialTierRefusal(member, "MCP"),
		"params_schema": requestContracts[ep.ID],
	}
}

// codeOrchCredentialTierRefusal is the error result for a member, or nil.
func codeOrchCredentialTierRefusal(ep *codeOrchEndpoint) *mcplib.CallToolResult {
	member, ok := codeOrchCredentialTierMember(ep)
	if !ok {
		return nil
	}
	return mcplib.NewToolResultError(cliutil.CredentialTierRefusal(member, "MCP"))
}
