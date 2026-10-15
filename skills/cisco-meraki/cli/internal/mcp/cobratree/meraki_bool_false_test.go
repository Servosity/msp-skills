// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/cisco-meraki/handfixes.json
// under "meraki-mcp-bool-false".

package cobratree

import (
	"reflect"
	"testing"
)

// TestMCPSendsExplicitFalse: dropping a false left a default-on flag
// (fleet renewals --include-expired) impossible to switch off from MCP.
func TestMCPSendsExplicitFalse(t *testing.T) {
	got := cliArgsFromMCP(map[string]any{"include-expired": false, "csv": true}, nil)
	want := []string{"--csv", "--include-expired=false"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}
