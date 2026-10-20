// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// HAND-AUTHORED.

package mcp

import (
	"sort"
	"strings"
	"testing"

	"synology-pp-cli/internal/mcp/cobratree"
)

// TestEndpointReadToolsRefuseCobraBlockedNames: the typed endpoint read
// tools (abb_tasks and the other spec-derived tools) accepted deliver,
// config, home, output, db and the other CLI root and destination flag names
// the Cobra-mirror tools refuse, and forwarded them to the NAS as DSM query
// parameters. They now refuse every name in cobratree.BlockedArgumentNames
// (the one list both layers read), before any request.
func TestEndpointReadToolsRefuseCobraBlockedNames(t *testing.T) {
	fake := newFakeDSM(t, "Office PCs")
	s := sweepMCPServer(t, fake, "1")
	names := make([]string, 0)
	for n := range cobratree.BlockedArgumentNames() {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, must := range []string{"deliver", "output", "db", "config", "home", "profile", "base-url", "o", "yes"} {
		if !cobratree.BlockedArgumentNames()[must] {
			t.Fatalf("the shared block list lacks %q", must)
		}
	}
	for _, tool := range []string{"abb_tasks", "abb_settings", "system_get"} {
		for _, name := range append(names, "Deliver", "OUTPUT") {
			text, isErr := callTool(t, s, tool, map[string]any{name: "file:/tmp/x"})
			if !isErr || !strings.Contains(text, "refused over MCP") {
				t.Fatalf("%s {%q} was not refused: %s", tool, name, text)
			}
		}
	}
	if calls := fake.Calls(); len(calls) != 0 {
		t.Fatalf("a refused call reached the NAS: %+v", calls)
	}
	// A harmless extra DSM filter still passes through.
	if text, isErr := callTool(t, s, "abb_tasks", map[string]any{"sort_by": "task_name"}); isErr {
		t.Fatalf("a harmless extra filter was refused: %s", text)
	}
}
