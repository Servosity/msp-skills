// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/dnsfilter/handfixes.json
// under the "mcp-tool-results-escape-controls",
// "mcp-read-only-withholds-credentials" and
// "mcp-read-only-hinted-tools-filter-credentials" entries.

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
	s.Use(func(next server.ToolHandlerFunc) server.ToolHandlerFunc {
		return resultSafetyMiddleware(s, next)
	})
}

func resultSafetyMiddleware(s *server.MCPServer, next server.ToolHandlerFunc) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		// Hand-wired (handfixes.json: mcp-read-only-hinted-tools-filter-credentials):
		// hosts auto-approve a tool advertised readOnlyHint=true, so its
		// results and errors are credential-filtered in every mode. The mark
		// on ctx reaches the shell-out layer (cobratree.RunCLICommand), which
		// redacts captured CLI output before it selects lines from it.
		readOnlyHinted := toolAdvertisedReadOnly(s, req.Params.Name)
		if readOnlyHinted {
			ctx = redact.WithForced(ctx)
		}
		res, err := next(ctx, req)
		if err != nil {
			// A handler error would otherwise reach the client verbatim as a
			// JSON-RPC error message, outside this funnel.
			return mcplib.NewToolResultError(safeResultText(err.Error(), readOnlyHinted)), nil
		}
		return safeToolResult(res, readOnlyHinted), nil
	}
}

// toolAdvertisedReadOnly reports whether the registered tool carries
// readOnlyHint=true, read from the registry at call time, so it is exactly
// what tools/list advertised (dnsfilter_execute and tail are hinted read-only
// only under the switch).
func toolAdvertisedReadOnly(s *server.MCPServer, name string) bool {
	if s == nil {
		return false
	}
	entry := s.GetTool(name)
	if entry == nil {
		return false
	}
	hint := entry.Tool.Annotations.ReadOnlyHint
	return hint != nil && *hint
}

// safeToolResult rewrites every text block of a result in place.
func safeToolResult(res *mcplib.CallToolResult, readOnlyHinted bool) *mcplib.CallToolResult {
	if res == nil {
		return nil
	}
	for i, c := range res.Content {
		switch tc := c.(type) {
		case mcplib.TextContent:
			tc.Text = safeResultText(tc.Text, readOnlyHinted)
			res.Content[i] = tc
		case *mcplib.TextContent:
			if tc != nil {
				tc.Text = safeResultText(tc.Text, readOnlyHinted)
			}
		}
	}
	if res.StructuredContent != nil && (redact.Enabled() || readOnlyHinted) {
		// Nothing in this server sets structured content today; a future
		// handler that does must not bypass the credential filter.
		if raw, err := json.Marshal(res.StructuredContent); err == nil {
			var v any
			if json.Unmarshal([]byte(filterCredentials(string(raw), readOnlyHinted)), &v) == nil {
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

// safeResultText applies the credential filter (filterCredentials), then the
// terminal control escape.
func safeResultText(s string, readOnlyHinted bool) string {
	return escapeTerminalControls(filterCredentials(s, readOnlyHinted))
}

// filterCredentials is the fail-closed credential pass on the text actually
// returned: redact.Final under the read-only switch (internal/mcp/bound
// already ran redact.Raw on the full body before budgeting it), and
// redact.FinalAlways, the same rules, for a tool advertised readOnlyHint=true
// while the switch is off. Any other tool's text passes unchanged off the
// switch: destructive-hinted tools (dnsfilter_execute, tail) return
// credentials in default mode by design.
func filterCredentials(s string, readOnlyHinted bool) string {
	if readOnlyHinted && !redact.Enabled() {
		return redact.FinalAlways(s)
	}
	return redact.Final(s)
}

// escapeTerminalControls rewrites C0 controls (except tab and newline), a CR
// not followed by LF, DEL, C1 controls (U+0080-U+009F) and the bidi
// override/isolate controls (U+202A-U+202E, U+2066-U+2069) as \uXXXX. Tenant
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
	const hex = "0123456789abcdef"
	for i, r := range s {
		if isTerminalControl(r) || (r == '\r' && (i+1 >= len(s) || s[i+1] != '\n')) {
			b.WriteString(`\u`)
			b.WriteByte(hex[(r>>12)&0xf])
			b.WriteByte(hex[(r>>8)&0xf])
			b.WriteByte(hex[(r>>4)&0xf])
			b.WriteByte(hex[r&0xf])
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func isTerminalControl(r rune) bool {
	return (r < 0x20 && r != '\t' && r != '\n' && r != '\r') || r == 0x7f || (r >= 0x80 && r <= 0x9f) || isBidiControl(r)
}

// isBidiControl reports the Unicode bidirectional embedding, override and
// isolate controls (U+202A-U+202E, U+2066-U+2069). In a tenant name they
// reorder how a host displays the surrounding text (Trojan Source), so
// why_blocked / allow previews could show a different domain or policy than
// the one acted on. Hand-wired (handfixes.json: mcp-tool-results-escape-controls).
func isBidiControl(r rune) bool {
	return (r >= 0x202a && r <= 0x202e) || (r >= 0x2066 && r <= 0x2069)
}
