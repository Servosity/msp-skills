// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored (not generator output); pinned by skills/eset-protect/handfixes.json
// under "secret-registry-masks-every-boundary".
//
// Every MCP tool result leaves the server process through this middleware:
// the registry tools (search, get, execute), the intents, the cobratree
// command mirror (the companion CLI's stdout and stderr), sql, search and
// context. It runs the result's text, structured content and error through
// secrets.Mask, so a credential this process holds (ESET_PROTECT_PASSWORD,
// ESET_PROTECT_TOKEN, ESET_PROTECT_MCP_HTTP_TOKEN, the cached tokens, every
// token minted or refreshed in-process) never reaches the MCP host, whichever
// path produced the text. Registered first, so it is the outermost wrapper.
//
// The same pass escapes the terminal controls JSON leaves raw, the way the
// CLI's stdout writer does (handfixes.json: mcp-results-escape-terminal-controls):
// U+0080-U+009F (C1: CSI, NEL, OSC) and the bidi embedding, override and
// isolate controls (U+202A-U+202E, U+2066-U+2069) become \u00XX / \u20XX.
// encoding/json already escapes C0, U+2028 and U+2029. In-process tools
// (execute, sql, search) answer with tenant strings as JSON text, where each
// escape decodes back to the same rune, so the result stays valid and
// lossless; the cobratree mirror's text comes from the escaped CLI already.

package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"eset-protect-pp-cli/internal/secrets"
	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func installSecretMasking(s *server.MCPServer) {
	s.Use(maskToolResults)
}

// maskToolResults masks every tool's result and error.
func maskToolResults(next server.ToolHandlerFunc) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		res, err := next(ctx, req)
		if err != nil {
			if msg := secrets.Mask(err.Error()); msg != err.Error() {
				err = errors.New(msg)
			}
		}
		maskToolResult(res)
		return res, err
	}
}

// maskToolResult masks res in place: every text content, every embedded text
// resource, a resource link's names, and the structured content.
func maskToolResult(res *mcplib.CallToolResult) {
	if res == nil {
		return
	}
	for i, c := range res.Content {
		res.Content[i] = maskContent(c)
	}
	if res.StructuredContent != nil {
		res.StructuredContent = maskStructured(res.StructuredContent)
	}
	if len(res.RawStructuredContent) > 0 {
		if out, ok := secrets.MaskJSON(res.RawStructuredContent); ok {
			res.RawStructuredContent = out
		} else {
			res.RawStructuredContent = nil
		}
	}
}

func maskContent(c mcplib.Content) mcplib.Content {
	switch v := c.(type) {
	case mcplib.TextContent:
		v.Text = escapeTerminalRunes(secrets.Mask(v.Text))
		return v
	case *mcplib.TextContent:
		if v != nil {
			v.Text = escapeTerminalRunes(secrets.Mask(v.Text))
		}
		return v
	case mcplib.EmbeddedResource:
		v.Resource = maskResource(v.Resource)
		return v
	case *mcplib.EmbeddedResource:
		if v != nil {
			v.Resource = maskResource(v.Resource)
		}
		return v
	case mcplib.ResourceLink:
		v.URI, v.Name, v.Title, v.Description = secrets.Mask(v.URI), secrets.Mask(v.Name), secrets.Mask(v.Title), secrets.Mask(v.Description)
		return v
	case *mcplib.ResourceLink:
		if v != nil {
			v.URI, v.Name, v.Title, v.Description = secrets.Mask(v.URI), secrets.Mask(v.Name), secrets.Mask(v.Title), secrets.Mask(v.Description)
		}
		return v
	}
	// Image and audio content carry base64 media, which the registry never
	// matches (see package secrets).
	return c
}

func maskResource(r mcplib.ResourceContents) mcplib.ResourceContents {
	switch v := r.(type) {
	case mcplib.TextResourceContents:
		v.Text, v.URI = escapeTerminalRunes(secrets.Mask(v.Text)), secrets.Mask(v.URI)
		return v
	case *mcplib.TextResourceContents:
		if v != nil {
			v.Text, v.URI = escapeTerminalRunes(secrets.Mask(v.Text)), secrets.Mask(v.URI)
		}
		return v
	}
	return r
}

// maskStructured masks structured content through its JSON form. Content
// that cannot be masked in place is replaced by the withheld notice.
func maskStructured(v any) any {
	raw, err := json.Marshal(v)
	if err != nil {
		return v
	}
	out, ok := secrets.MaskJSON(raw)
	if !ok {
		return map[string]any{"withheld": secrets.Withheld}
	}
	if string(out) == string(raw) {
		return v
	}
	var masked any
	if json.Unmarshal(out, &masked) != nil {
		return map[string]any{"withheld": secrets.Withheld}
	}
	return masked
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
