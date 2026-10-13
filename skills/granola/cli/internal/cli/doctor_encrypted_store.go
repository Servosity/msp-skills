// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"strconv"
	"time"
)

// collectMCPReport reports the official-MCP connection status passively (no
// network, no token rotation) — Keychain presence + locally-stored expiry.
// Optional surface: absent MCP only means private/human notes are unavailable.
//
// On Granola v7.4x+ the encrypted desktop store is sealed behind Granola's own
// Keychain access group, so a third-party binary can never decrypt it. This
// connector therefore sources ENTIRELY from the public REST API + the official
// MCP; there is no desktop-store status to report here.
func collectMCPReport(report map[string]any) {
	access, err := kcGet(kcAcctAccess)
	if err != nil || access == "" {
		report["granola_mcp"] = "INFO not connected (optional — run 'granola-cli mcp-auth login' for private/human notes)"
		return
	}
	if expStr, e := kcGet(kcAcctExpiry); e == nil {
		if unix, e2 := strconv.ParseInt(expStr, 10, 64); e2 == nil && !time.Now().Before(time.Unix(unix, 0)) {
			report["granola_mcp"] = "INFO token expired — run 'granola-cli mcp-auth login' to reconnect"
			return
		}
	}
	report["granola_mcp"] = "connected (private/human notes available)"
}
