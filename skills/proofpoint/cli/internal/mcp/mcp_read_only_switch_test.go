// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package mcp

// Hand-fix (handfixes.json mcp-read-only-switch): tools/maintainer/
// replay_handfixes.py writes this file from
// handfix_replay/mcp_read_only_switch_test.go.tmpl.

import (
	"sort"
	"testing"

	"github.com/mark3labs/mcp-go/server"
)

// mcpReadOnlySwitchEnvVar is spelled out here, not read from cliutil, so this
// test builds, and fails, on a server that never reads the switch.
const mcpReadOnlySwitchEnvVar = "PROOFPOINT_MCP_READ_ONLY"

// mcpReadOnlySwitchListing registers every tool this server offers with the
// switch set to value, and returns all tool names and the names of the tools
// that do not carry readOnlyHint=true.
func mcpReadOnlySwitchListing(t *testing.T, value string) (all, notReadOnly []string) {
	t.Helper()
	t.Setenv(mcpReadOnlySwitchEnvVar, value)
	s := server.NewMCPServer("mcp-read-only-switch-test", "0")
	RegisterTools(s)
	for name, entry := range s.ListTools() {
		all = append(all, name)
		if hint := entry.Tool.Annotations.ReadOnlyHint; hint == nil || !*hint {
			notReadOnly = append(notReadOnly, name)
		}
	}
	sort.Strings(all)
	sort.Strings(notReadOnly)
	return all, notReadOnly
}

// With the switch set to exactly "1" the server lists only tools annotated
// readOnlyHint=true: every write tool, the API execute tool and the
// local-store writers are gone. Any other value changes nothing.
func TestMCPReadOnlySwitchListsOnlyReadOnlyTools(t *testing.T) {
	off, writes := mcpReadOnlySwitchListing(t, "")
	if len(writes) == 0 {
		t.Fatal("control: with the switch off every tool already carries readOnlyHint=true, so this test cannot tell the switch from no switch")
	}
	on, leaked := mcpReadOnlySwitchListing(t, "1")
	if len(leaked) > 0 {
		t.Errorf("%s=1 still lists %d tool(s) without readOnlyHint=true: %v", mcpReadOnlySwitchEnvVar, len(leaked), leaked)
	}
	// Every tool that is read-only with the switch off is still offered with it on.
	listed, isWrite := map[string]bool{}, map[string]bool{}
	for _, name := range on {
		listed[name] = true
	}
	for _, name := range writes {
		isWrite[name] = true
	}
	for _, name := range off {
		if !isWrite[name] && !listed[name] {
			t.Errorf("%s=1 dropped the read-only tool %s", mcpReadOnlySwitchEnvVar, name)
		}
	}
	if len(on) == 0 {
		t.Errorf("%s=1 lists no tool at all", mcpReadOnlySwitchEnvVar)
	}
	for _, value := range []string{"true", "yes", "0"} {
		if got, _ := mcpReadOnlySwitchListing(t, value); len(got) != len(off) {
			t.Errorf("%s=%s lists %d tool(s), want all %d: only \"1\" turns the switch on", mcpReadOnlySwitchEnvVar, value, len(got), len(off))
		}
	}
}
