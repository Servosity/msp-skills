// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): tests for the MCP server's error backstop
// (secret_funnel_mcp.go). Pinned by this connector's handfixes.json under the
// "secret-echo-decoded-funnel" entry (class "secret-echo").

package main

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"testing"

	mcplib "github.com/mark3labs/mcp-go/mcp"

	"liongard-pp-cli/internal/cliutil"
)

func TestFunnelToolErrorsWithholdsAnEchoedCredential(t *testing.T) {
	held := "mcp-backstop-held-0a1+2b3/4c5="
	cliutil.FunnelHold(held)
	echo := base64.StdEncoding.EncodeToString([]byte(held))
	call := func(h func(context.Context, mcplib.CallToolRequest) (*mcplib.CallToolResult, error)) (*mcplib.CallToolResult, error) {
		return funnelToolErrors(h)(context.Background(), mcplib.CallToolRequest{})
	}

	res, _ := call(func(context.Context, mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		return mcplib.NewToolResultError("upstream said " + echo), nil
	})
	if text := res.Content[0].(mcplib.TextContent).Text; strings.Contains(text, echo) || !cliutil.FunnelWithheld(text) {
		t.Fatalf("an error result carried the echo: %q", text)
	}

	_, err := call(func(context.Context, mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		return nil, errors.New("boom " + echo)
	})
	if err == nil || strings.Contains(err.Error(), echo) {
		t.Fatalf("a handler error carried the echo: %v", err)
	}

	// Success results are never funnelled, and clean errors pass unchanged.
	res, _ = call(func(context.Context, mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		return mcplib.NewToolResultText("data " + echo), nil
	})
	if text := res.Content[0].(mcplib.TextContent).Text; text != "data "+echo {
		t.Fatalf("a success result was changed: %q", text)
	}
	res, _ = call(func(context.Context, mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		return mcplib.NewToolResultError("not found"), nil
	})
	if text := res.Content[0].(mcplib.TextContent).Text; text != "not found" {
		t.Fatalf("a clean error result was changed: %q", text)
	}
}
