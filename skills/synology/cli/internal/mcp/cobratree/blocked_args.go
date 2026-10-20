// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// HAND-AUTHORED (not emitted by CLI Printing Press): read-only view of the
// generated block tables in shellout.go, shared with the typed endpoint tools.

package cobratree

// BlockedArgumentNames lists every argument name the Cobra-mirror MCP tools
// refuse because it would steer the CLI rather than the command: the root
// flags an MCP caller must not override (blockedRootFlags: --deliver,
// --config, --home, --profile, --base-url, ...), the filesystem destination
// flags (blockedDestinationFlags: --output, -o, --db, ...), the confirmation
// bypass (--yes) and the MCP-only structured names (args). It is built from
// those same tables on every call, so the typed endpoint tools (abb_tasks
// and the other spec-derived tools in package mcp) refuse exactly the names
// the Cobra-mirror tools refuse: one list, never a copy.
func BlockedArgumentNames() map[string]bool {
	out := map[string]bool{}
	for _, table := range []map[string]bool{blockedRootFlags, blockedDestinationFlags, confirmationBypassRootFlags, reservedStructuredArgs} {
		for name, blocked := range table {
			if blocked {
				out[name] = true
			}
		}
	}
	return out
}
