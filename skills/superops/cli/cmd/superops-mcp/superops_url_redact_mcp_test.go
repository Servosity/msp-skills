// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	mcplib "github.com/mark3labs/mcp-go/mcp"
)

// Every tool result and handler error, success results included, reaches the
// MCP client without a URL's userinfo; structured content that carried it is
// dropped rather than shown.
func TestToolResultsNeverShowURLUserinfo(t *testing.T) {
	const shown = "GET http://svc:pw@127.0.0.1:9/graphql"
	h := redactToolURLUserinfo(func(context.Context, mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		return &mcplib.CallToolResult{
			Content:           []mcplib.Content{mcplib.TextContent{Type: "text", Text: shown}, &mcplib.TextContent{Type: "text", Text: shown}},
			StructuredContent: map[string]any{"url": "http://svc:pw@127.0.0.1:9"},
		}, errors.New("transport: " + shown)
	})
	res, err := h(context.Background(), mcplib.CallToolRequest{})
	if err == nil || strings.Contains(err.Error(), "svc") {
		t.Fatalf("handler error shows URL userinfo: %v", err)
	}
	for i, c := range res.Content {
		var text string
		switch tc := c.(type) {
		case mcplib.TextContent:
			text = tc.Text
		case *mcplib.TextContent:
			text = tc.Text
		}
		if strings.Contains(text, "svc") || strings.Contains(text, "pw@") || !strings.Contains(text, "127.0.0.1:9") {
			t.Fatalf("content %d: %q", i, text)
		}
	}
	if res.StructuredContent != nil {
		t.Fatalf("structured content with URL userinfo was kept: %v", res.StructuredContent)
	}
}
