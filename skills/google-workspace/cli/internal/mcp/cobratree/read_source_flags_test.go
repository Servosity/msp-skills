// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: covers handfixes.json mcp-blocks-local-file-reads.

package cobratree

import "testing"

func TestLocalFileReadFlagsBlockedOverMCP(t *testing.T) {
	for _, name := range []string{"input", "notes-file", "playbook-file", "playbook-notes-file"} {
		if !DestinationFlagBlocked(nil, name) {
			t.Errorf("--%s reads a local file and must not be settable over MCP", name)
		}
		args := cliArgsFromMCP(map[string]any{name: "/home/someone/keys/acme-service-account.json"}, blockedDestinationFlags)
		if len(args) != 0 {
			t.Errorf("--%s must be dropped from MCP argv, got %v", name, args)
		}
	}
}
