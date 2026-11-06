// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/td-synnex/handfixes.json under the
// "mcp-http-token-ignores-mcpb-placeholder" entry.

package main

import (
	"strings"
	"testing"
)

// The .mcpb manifest maps TD_SYNNEX_MCP_HTTP_TOKEN to ${user_config.td_synnex_mcp_http_token}.
// A host that leaves the template unresolved passes that literal, which is 39
// characters and public, so it must never be accepted as the HTTP bearer secret.
func TestRequireHTTPCallerTokenRejectsUnresolvedPlaceholder(t *testing.T) {
	t.Setenv(httpTokenEnvVar, "${user_config.td_synnex_mcp_http_token}")
	if _, err := requireHTTPCallerToken(); err == nil {
		t.Fatal("an unresolved MCPB placeholder must not be accepted as the HTTP bearer token")
	}
	real := strings.Repeat("s", mcpHTTPTokenMinLength)
	t.Setenv(httpTokenEnvVar, real)
	if tok, err := requireHTTPCallerToken(); err != nil || tok != real {
		t.Fatalf("a real token must be accepted: %q %v", tok, err)
	}
}
