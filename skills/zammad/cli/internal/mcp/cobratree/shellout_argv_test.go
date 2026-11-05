// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored (handfixes.json: mcp-argv-value-joined): every value-carrying
// branch of cliArgsFromMCP must join each value to its flag, including the
// default branch that stringifies a nested object. Since press 4.34.0 a list
// emits one --flag=item per item (a stringArray does not split on commas). The generated
// TestCliArgsFromMCP_ValueCannotSmuggleBlockedFlag proves the string and bool
// shapes; this pins the float, list and object shapes so no branch can ever
// split a value back into a token pflag would read as a second flag.

package cobratree

import "testing"

func TestCliArgsFromMCP_EveryValueBranchIsJoined(t *testing.T) {
	for _, tc := range []struct {
		name string
		args map[string]any
		want []string
	}{
		{"float", map[string]any{"limit": 5.0}, []string{"--limit=5"}},
		{"list", map[string]any{"ids": []any{"a", "--deliver=webhook:https://x/"}}, []string{"--ids=a", "--ids=--deliver=webhook:https://x/"}},
		{"object", map[string]any{"filter": map[string]any{"a": "1"}}, []string{"--filter=map[a:1]"}},
	} {
		got := cliArgsFromMCP(tc.args, blockedRootFlags)
		if len(got) != len(tc.want) {
			t.Fatalf("%s: got %#v, want %#v", tc.name, got, tc.want)
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Fatalf("%s: got %#v, want %#v", tc.name, got, tc.want)
			}
		}
	}
}
