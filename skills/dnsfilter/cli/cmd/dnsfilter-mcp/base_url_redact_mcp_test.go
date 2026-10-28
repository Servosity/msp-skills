// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/dnsfilter/handfixes.json under
// the "base-url-credentials-held-and-redacted" entry (class "secret-echo").

package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// TestToolErrorsRedactURLCredentials: an error result or handler error that
// names a URL with credentials reaches the client with them redacted, inside
// the kit's check (a held password yields the redacted text, not a withheld one);
// a success result is never rewritten.
func TestToolErrorsRedactURLCredentials(t *testing.T) {
	const pw = "McpWriterPw4242"
	chain := func(h server.ToolHandlerFunc) server.ToolHandlerFunc { return redactToolErrorURLs(h) }
	call := func(h server.ToolHandlerFunc) (*mcplib.CallToolResult, error) {
		return chain(h)(context.Background(), mcplib.CallToolRequest{})
	}
	bad := `creating request: parse "ht!tp://u:` + pw + `@[::1/v1/current_user": first path segment in URL cannot contain colon`
	want := `parse "ht!tp://****@[::1/v1/current_user"`

	res, _ := call(func(context.Context, mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		return mcplib.NewToolResultError(bad), nil
	})
	if text := res.Content[0].(mcplib.TextContent).Text; strings.Contains(text, pw) || !strings.Contains(text, want) {
		t.Fatalf("an error result shows %q (want the redacted URL)", text)
	}
	_, err := call(func(context.Context, mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		return nil, errors.New(bad)
	})
	if err == nil || strings.Contains(err.Error(), pw) || !strings.Contains(err.Error(), want) {
		t.Fatalf("a handler error shows %v (want the redacted URL)", err)
	}
	res, _ = call(func(context.Context, mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		return mcplib.NewToolResultText("see https://u:data@h.example"), nil
	})
	if text := res.Content[0].(mcplib.TextContent).Text; text != "see https://u:data@h.example" {
		t.Fatalf("a success result was rewritten: %q", text)
	}
}
