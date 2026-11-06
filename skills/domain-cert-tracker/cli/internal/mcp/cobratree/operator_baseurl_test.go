// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cobratree

import "testing"

// domain-cert-tracker's guard reaches a non-public address only at the
// client's configured base URL, so no tool argument may choose the base URL
// or the config, profile or home it is read from.
func TestMCPCannotChooseTheBaseURL(t *testing.T) {
	for _, f := range []string{"base-url", "config", "profile", "home"} {
		if !blockedRootFlags[f] {
			t.Errorf("root flag --%s is not blocked from MCP tool arguments", f)
		}
		if got := cliArgsFromMCP(map[string]any{f: "http://127.0.0.1:1"}, blockedRootFlags); len(got) != 0 {
			t.Errorf("--%s passed through MCP arguments: %v", f, got)
		}
	}
}
