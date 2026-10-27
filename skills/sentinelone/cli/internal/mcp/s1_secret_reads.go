// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated). A GET whose answer is a credential
// (cliutil.IsSecretBearingPath: agents.get-passphrase,
// accounts.uninstall-password.get, users.login-by-token,
// ranger.get-cred-group-details) needs confirm over MCP: sentinelone_read
// refuses it, sentinelone_execute previews it until confirm is boolean true,
// and SENTINELONE_MCP_READ_ONLY=1 refuses it. The flag is set here, at
// package init, instead of in the generated codeOrchEndpoints table, so a
// reprint that regenerates code_orch.go keeps it; the rule is the same one
// sync, the store and the response cache use.

package mcp

import (
	"sort"

	"sentinelone-pp-cli/internal/cliutil"
)

func init() {
	for i := range codeOrchEndpoints {
		ep := &codeOrchEndpoints[i]
		if codeOrchNormalizeMethod(ep.Method) == "GET" && cliutil.IsSecretBearingPath(ep.Path) {
			ep.NeedsConfirm = true
		}
	}
}

// CredentialTierEndpointIDs returns the sentinelone_execute endpoint ids that
// GET a credential-tier member's path (cliutil.CredentialTiers): the one way
// an MCP agent reads its value, with confirm set to boolean true. A refusal
// names them so the agent is pointed at the confirmed path instead of guessing.
func CredentialTierEndpointIDs(tierPath string) []string {
	if tierPath == "" {
		return nil
	}
	var ids []string
	for i := range codeOrchEndpoints {
		ep := &codeOrchEndpoints[i]
		if codeOrchNormalizeMethod(ep.Method) != "GET" {
			continue
		}
		if m, ok := cliutil.CredentialTierPath(ep.Path); ok && m.Path == tierPath && cliutil.IsSecretBearingPath(ep.Path) {
			ids = append(ids, ep.ID)
		}
	}
	sort.Strings(ids)
	return ids
}
