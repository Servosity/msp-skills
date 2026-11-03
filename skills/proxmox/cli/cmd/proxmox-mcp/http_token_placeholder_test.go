// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/proxmox/handfixes.json under the
// "mcp-http-token-ignores-mcpb-placeholder" entry.

package main

import (
	"strings"
	"testing"
)

// The .mcpb manifest maps PROXMOX_MCP_HTTP_TOKEN to ${user_config.proxmox_mcp_http_token}.
// A host that leaves the template unresolved passes that literal, which is 37
// characters and public, so it must never be accepted as the HTTP bearer secret.
func TestRequireHTTPCallerTokenRejectsUnresolvedPlaceholder(t *testing.T) {
	for _, v := range []string{"${user_config.proxmox_mcp_http_token}", " ${user_config.proxmox_mcp_http_token} "} {
		t.Setenv(httpTokenEnvVar, v)
		if _, err := requireHTTPCallerToken(); err == nil {
			t.Fatalf("an unresolved MCPB placeholder (%q) must not be accepted as the HTTP bearer token", v)
		}
	}
	real := strings.Repeat("s", mcpHTTPTokenMinLength)
	t.Setenv(httpTokenEnvVar, real)
	if tok, err := requireHTTPCallerToken(); err != nil || tok != real {
		t.Fatalf("a real token must be accepted: %q %v", tok, err)
	}
}
