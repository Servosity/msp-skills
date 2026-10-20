// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/dnsfilter/handfixes.json
// under the "mcp-http-token-placeholder" entry.

package main

import "testing"

// TestHTTPTokenPlaceholderRefused: the published manifest's unresolved
// placeholder is public text, so it must read as no token at all.
func TestHTTPTokenPlaceholderRefused(t *testing.T) {
	t.Setenv(httpTokenEnvVar, "${user_config.dnsfilter_mcp_http_token}")
	if tok, err := requireHTTPCallerToken(); err == nil {
		t.Fatalf("placeholder accepted as bearer token %q", tok)
	}
	t.Setenv(httpTokenEnvVar, "real-token")
	if tok, err := requireHTTPCallerToken(); err != nil || tok != "real-token" {
		t.Fatalf("real token refused: %q %v", tok, err)
	}
}
