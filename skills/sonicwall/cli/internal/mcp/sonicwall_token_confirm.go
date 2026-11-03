// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package mcp

import "strings"

// Hand-wired (handfixes.json: token-exchange-needs-confirm). Every endpoint
// that mints or refreshes a SonicWall credential is a state change: MCP may run
// it only through sonicwall_execute with boolean confirm: true, and never under
// SONICWALL_MCP_READ_ONLY=1. The spec marks POST /auth/sso as a read
// (x-pp-mutation: false keeps the CLI command from writing the minted token
// into the local store), which left it callable through sonicwall_read.
var tokenExchangePaths = []string{"/auth/sso", "/auth/refreshSessionToken", "/api/generate-cscAccessCode"}

func init() {
	for i := range codeOrchEndpoints {
		if isTokenExchangePath(codeOrchEndpoints[i].Path) {
			codeOrchEndpoints[i].NeedsConfirm = true
		}
	}
}

func isTokenExchangePath(p string) bool {
	if i := strings.IndexAny(p, "?#"); i >= 0 {
		p = p[:i]
	}
	p = strings.TrimRight(p, "/")
	for _, s := range tokenExchangePaths {
		if strings.HasSuffix(strings.ToLower(p), strings.ToLower(s)) {
			return true
		}
	}
	return false
}
