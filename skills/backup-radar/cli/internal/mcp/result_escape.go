// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/backup-radar/handfixes.json
// under the "mcp-tool-results-escape-controls" entry.

package mcp

import (
	"backup-radar-pp-cli/internal/termio"

	mcplib "github.com/mark3labs/mcp-go/mcp"
)

// escapeTerminalControls rewrites C0 controls (except tab, newline and CR),
// DEL and C1 controls (U+0080-U+009F) as \u00XX (termio.EscapeString). Tenant
// strings (client, device and job names, notes) reach MCP tool results
// straight from API JSON and the local store, and a host that prints a result
// to a terminal would otherwise execute an OSC 52 clipboard write or a CSI
// sequence carried in a name. The escape keeps JSON text valid and lossless.
func escapeTerminalControls(s string) string { return termio.EscapeString(s) }

// mcpResultText and mcpResultError are the only constructors tool handlers
// use (a test enforces it), so every result text is escaped at one boundary.
func mcpResultText(s string) *mcplib.CallToolResult {
	return mcplib.NewToolResultText(escapeTerminalControls(s))
}

func mcpResultError(s string) *mcplib.CallToolResult {
	return mcplib.NewToolResultError(escapeTerminalControls(s))
}
