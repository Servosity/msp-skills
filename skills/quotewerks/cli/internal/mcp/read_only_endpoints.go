// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored (handfixes.json: mcp-read-only-drops-endpoint-writes).

package mcp

import (
	"sort"

	"github.com/mark3labs/mcp-go/server"
)

// dropNonReadOnlyTools removes every registered tool that is not annotated
// read-only. The generated endpoint-mirror tools (tables_insert, tables_update,
// tables_delete, agent_ask, ...) never consulted QUOTEWERKS_MCP_READ_ONLY, so
// the kill switch still offered raw table writes; only the Cobra-tree mirror
// applied it. Run it before cobratree.RegisterAll, which filters its own tools.
func dropNonReadOnlyTools(s *server.MCPServer) []string {
	var drop []string
	for name, st := range s.ListTools() {
		if hint := st.Tool.Annotations.ReadOnlyHint; hint == nil || !*hint {
			drop = append(drop, name)
		}
	}
	sort.Strings(drop)
	if len(drop) > 0 {
		s.DeleteTools(drop...)
	}
	return drop
}
