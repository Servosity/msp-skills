// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated), cove only: tests for the MCP half of the
// short-credential backstop (cove_short_funnel_mcp.go). Hand-fix code:
// cove-short-credential-funnel.

package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"cove-pp-cli/internal/cliutil"
	"cove-pp-cli/internal/cliutil/testenv"
)

// coveToolChain is the tool-handler chain main.go registers: funnelToolErrors
// outermost, coveShortToolErrors inside it.
func coveToolChain(h server.ToolHandlerFunc) server.ToolHandlerFunc {
	return funnelToolErrors(coveShortToolErrors(h))
}

// isolatedHome points the session cache at an empty directory.
func isolatedHome(t *testing.T) string {
	t.Helper()
	return testenv.Isolate(t, cliutil.ConfigDir, cliutil.DataDir, cliutil.StateDir, cliutil.CacheDir)
}

// Codex review round 1: a credential shorter than the shared funnel's
// decoded-view minimum bypassed the MCP error backstop.
func TestFunnelToolErrorsWithholdsAShortHeldCredential(t *testing.T) {
	isolatedHome(t)
	cliutil.FunnelHold("Kp4$w")
	res, err := coveToolChain(func(context.Context, mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		return mcplib.NewToolResultError("upstream said Kp4$w"), errors.New("boom Kp4$w")
	})(context.Background(), mcplib.CallToolRequest{})
	if text := res.Content[0].(mcplib.TextContent).Text; strings.Contains(text, "Kp4$w") {
		t.Fatalf("short held credential in an error result: %q", text)
	}
	if err == nil || strings.Contains(err.Error(), "Kp4$w") {
		t.Fatalf("short held credential in a handler error: %v", err)
	}
}

// The short backstop covers structured content and every other content
// block of an error result, holds COVE_PASSWORD and the cached session visa,
// leaves success results alone and passes clean errors unchanged.
func TestCoveShortToolErrorsCoversEverySurface(t *testing.T) {
	home := isolatedHome(t)
	t.Setenv("COVE_PASSWORD", "Tq8%m")
	dir := filepath.Join(home, ".config", "cove-cli")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "session.json"), []byte(`{"visa":"cached-visa-0123456789abcdef"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	call := func(res *mcplib.CallToolResult, err error) (*mcplib.CallToolResult, error) {
		return coveToolChain(func(context.Context, mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
			return res, err
		})(context.Background(), mcplib.CallToolRequest{})
	}

	structured := mcplib.NewToolResultError("failed")
	structured.StructuredContent = map[string]any{"detail": "token Tq8%m"}
	if res, _ := call(structured, nil); res.StructuredContent != nil {
		t.Fatalf("structured content repeating COVE_PASSWORD kept: %v", res.StructuredContent)
	}
	embedded := mcplib.NewToolResultError("failed")
	embedded.Content = append(embedded.Content, mcplib.NewEmbeddedResource(mcplib.TextResourceContents{URI: "x://y", Text: "token Tq8%m"}))
	if res, _ := call(embedded, nil); len(res.Content) != 2 || !cliutil.FunnelWithheld(res.Content[1].(mcplib.TextContent).Text) {
		t.Fatalf("embedded content repeating COVE_PASSWORD kept: %#v", res.Content)
	}
	if _, err := call(nil, errors.New("visa cached-visa-0123456789abcdef refused")); err == nil || strings.Contains(err.Error(), "cached-visa-0123456789abcdef") {
		t.Fatalf("cached session visa in a handler error: %v", err)
	}

	if res, _ := call(mcplib.NewToolResultText("data Tq8%m"), nil); res.Content[0].(mcplib.TextContent).Text != "data Tq8%m" {
		t.Fatalf("a success result was changed: %#v", res.Content)
	}
	if res, _ := call(mcplib.NewToolResultError("not found"), nil); res.Content[0].(mcplib.TextContent).Text != "not found" {
		t.Fatalf("a clean error result was changed: %#v", res.Content)
	}
}
