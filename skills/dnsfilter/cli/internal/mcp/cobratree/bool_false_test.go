// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/dnsfilter/handfixes.json
// under the "mcp-bool-false-reaches-cli" entry.

package cobratree

import (
	"reflect"
	"testing"
)

// TestCliArgsFromMCP_ExplicitFalseReachesCLI: a bool flag that defaults to
// true (tail --follow) can only be turned off by sending --follow=false;
// dropping an explicit false left tail polling the API until the MCP host
// timed the call out.
func TestCliArgsFromMCP_ExplicitFalseReachesCLI(t *testing.T) {
	got := cliArgsFromMCP(map[string]any{"follow": false, "resource": "networks", "agent": true}, map[string]bool{"args": true})
	want := []string{"--agent", "--follow=false", "--resource=networks"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("cliArgsFromMCP = %v, want %v", got, want)
	}
}
