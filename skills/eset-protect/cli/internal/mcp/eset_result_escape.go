// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored (not generator output); pinned by skills/eset-protect/handfixes.json
// under "mcp-results-escape-terminal-controls".
//
// Every MCP tool result leaves the server process through this middleware. It
// escapes the terminal controls JSON leaves raw, the way the CLI's stdout writer
// does: U+0080-U+009F (C1: CSI, NEL, OSC) and the bidi embedding, override and
// isolate controls (U+202A-U+202E, U+2066-U+2069) become \u00XX / \u20XX.
// encoding/json already escapes C0, U+2028 and U+2029. In-process tools (execute,
// sql, search) answer with tenant strings as JSON text, where each escape decodes
// back to the same rune, so the result stays valid and lossless; the cobratree
// mirror's text comes from the escaped CLI already. Credentials are not this
// file's concern: credkit.MCPTools holds and withholds them.

package mcp

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func installResultEscaping(s *server.MCPServer) {
	s.Use(escapeToolResults)
}

// escapeToolResults escapes the terminal controls in every tool's text results.
func escapeToolResults(next server.ToolHandlerFunc) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		res, err := next(ctx, req)
		if res != nil {
			for i, c := range res.Content {
				res.Content[i] = escapeContent(c)
			}
		}
		return res, err
	}
}

func escapeContent(c mcplib.Content) mcplib.Content {
	switch v := c.(type) {
	case mcplib.TextContent:
		v.Text = escapeTerminalRunes(v.Text)
		return v
	case *mcplib.TextContent:
		if v != nil {
			v.Text = escapeTerminalRunes(v.Text)
		}
		return v
	case mcplib.EmbeddedResource:
		v.Resource = escapeResource(v.Resource)
		return v
	case *mcplib.EmbeddedResource:
		if v != nil {
			v.Resource = escapeResource(v.Resource)
		}
		return v
	}
	return c
}

func escapeResource(r mcplib.ResourceContents) mcplib.ResourceContents {
	switch v := r.(type) {
	case mcplib.TextResourceContents:
		v.Text = escapeTerminalRunes(v.Text)
		return v
	case *mcplib.TextResourceContents:
		if v != nil {
			v.Text = escapeTerminalRunes(v.Text)
		}
		return v
	}
	return r
}

// escapeTerminalRunes replaces every C1 control and bidi control rune with
// its \uXXXX escape.
func escapeTerminalRunes(s string) string {
	if !strings.ContainsFunc(s, terminalRune) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + 16)
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if terminalRune(r) {
			fmt.Fprintf(&b, `\u%04x`, r)
		} else {
			// Bytes that are not UTF-8 are copied as they are.
			b.WriteString(s[i : i+size])
		}
		i += size
	}
	return b.String()
}

func terminalRune(r rune) bool {
	return (r >= 0x80 && r <= 0x9f) || (r >= 0x202a && r <= 0x202e) || (r >= 0x2066 && r <= 0x2069)
}
