// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/synology/handfixes.json
// under the "mcp-read-only-drops-endpoint-writes" entry.

package mcp

import (
	"sort"

	"github.com/mark3labs/mcp-go/server"
)

// dropNonReadOnlyTools removes every registered tool that is not annotated
// read-only. SYNOLOGY_MCP_READ_ONLY was only consulted by the Cobra-tree
// mirror (cobratree.RegisterAll), so the generated endpoint tools that start
// or cancel a backup (abb_backup, abb_cancel) and end the DSM session
// (session_logout) stayed registered and callable in read-only mode, and the
// mirror's own filter keeps local-store writers (teach, teach_playbook,
// learnings_confirm, playbook_amend) that the manifest promises the switch
// drops. It runs after every Register* call, so one rule covers every tool.
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
