// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/dnsfilter/handfixes.json
// under the "mcp-tool-results-escape-controls" and
// "mcp-read-only-withholds-credentials" entries.

package mcp

import (
	"context"
	"encoding/json"
	"strings"

	"dnsfilter-pp-cli/internal/mcp/redact"
	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// installResultSafety registers the one result funnel every tool call passes
// through: typed tools, code orchestration, intents and the Cobra-tree mirror
// alike, because mcp-go applies server middleware to every registered tool at
// call time. RegisterTools installs it first so it is the outermost layer and
// also sees errors raised by the tenant gate.
func installResultSafety(s *server.MCPServer) {
	s.Use(resultSafetyMiddleware)
}

func resultSafetyMiddleware(next server.ToolHandlerFunc) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		res, err := next(ctx, req)
		if err != nil {
			// A handler error would otherwise reach the client verbatim as a
			// JSON-RPC error message, outside this funnel.
			return mcplib.NewToolResultError(safeResultText(err.Error())), nil
		}
		return safeToolResult(res), nil
	}
}

// safeToolResult rewrites every text block of a result in place.
func safeToolResult(res *mcplib.CallToolResult) *mcplib.CallToolResult {
	if res == nil {
		return nil
	}
	for i, c := range res.Content {
		switch tc := c.(type) {
		case mcplib.TextContent:
			tc.Text = safeResultText(tc.Text)
			res.Content[i] = tc
		case *mcplib.TextContent:
			if tc != nil {
				tc.Text = safeResultText(tc.Text)
			}
		}
	}
	if res.StructuredContent != nil && redact.Enabled() {
		// Nothing in this server sets structured content today; a future
		// handler that does must not bypass the read-only filter.
		if raw, err := json.Marshal(res.StructuredContent); err == nil {
			var v any
			if json.Unmarshal([]byte(redact.Final(string(raw))), &v) == nil {
				res.StructuredContent = v
			} else {
				res.StructuredContent = nil
			}
		} else {
			res.StructuredContent = nil
		}
	}
	return res
}

// safeResultText applies the read-only credential filter (redact.Final, the
// fail-closed pass on the text actually returned; internal/mcp/bound already
// ran redact.Raw on the full body before budgeting it), then the terminal
// control escape.
func safeResultText(s string) string {
	return escapeTerminalControls(redact.Final(s))
}

// escapeTerminalControls rewrites C0 controls (except tab and newline), a CR
// not followed by LF, DEL and C1 controls (U+0080-U+009F) as \u00XX. Tenant
// strings (organization, site, policy and agent names, notes, block-page
// text) reach tool results from API JSON, the local store and the companion
// CLI's stdout; a host that prints a result to a terminal would otherwise
// execute an OSC 52 clipboard write or a CSI sequence carried in a name.
// Inside JSON string literals the rewrite is a valid escape, so the text stays
// valid JSON and decodes to the original value.
func escapeTerminalControls(s string) string {
	if !strings.ContainsFunc(s, isTerminalControl) && !strings.Contains(s, "\r") {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + 16)
	for i, r := range s {
		if isTerminalControl(r) || (r == '\r' && (i+1 >= len(s) || s[i+1] != '\n')) {
			b.WriteString(`\u00`)
			b.WriteByte("0123456789abcdef"[r>>4])
			b.WriteByte("0123456789abcdef"[r&0xf])
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func isTerminalControl(r rune) bool {
	return (r < 0x20 && r != '\t' && r != '\n' && r != '\r') || r == 0x7f || (r >= 0x80 && r <= 0x9f)
}
