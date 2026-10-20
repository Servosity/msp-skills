// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/backup-radar/handfixes.json
// under the "mcp-tool-results-escape-controls" entry.

package cobratree

import (
	"backup-radar-pp-cli/internal/termio"

	mcplib "github.com/mark3labs/mcp-go/mcp"
)

// escapedToolResultText, escapedToolResultError and escapedTextContent are
// the only result constructors the mirror tools use (a test in internal/mcp
// enforces it): the companion CLI's own writer passes its SGR colors through,
// and an MCP result has no colors of its own, so every control character in a
// mirrored result or error, ESC included, is escaped at this boundary.
func escapedToolResultText(s string) *mcplib.CallToolResult {
	return mcplib.NewToolResultText(termio.EscapeString(s))
}

func escapedToolResultError(s string) *mcplib.CallToolResult {
	return mcplib.NewToolResultError(termio.EscapeString(s))
}

func escapedTextContent(s string) mcplib.TextContent {
	return mcplib.NewTextContent(termio.EscapeString(s))
}
