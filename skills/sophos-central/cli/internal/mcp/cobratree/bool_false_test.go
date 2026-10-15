// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/sophos-central/handfixes.json
// under the "mcp-bool-false-forwarded" entry.

package cobratree

import "testing"

// TestExplicitFalseReachesTheCLI: an omitted false left default-true flags on,
// so MCP `tail` with follow:false polled forever.
func TestExplicitFalseReachesTheCLI(t *testing.T) {
	got := cliArgsFromMCP(map[string]any{"follow": false, "json": true}, map[string]bool{})
	want := map[string]bool{"--follow=false": true, "--json": true}
	if len(got) != len(want) || !want[got[0]] || !want[got[1]] {
		t.Fatalf("argv = %q, want --follow=false and --json", got)
	}
}
