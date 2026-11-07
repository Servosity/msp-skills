// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package mcp

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	mcplib "github.com/mark3labs/mcp-go/mcp"
)

// TestUploadEndpointsAreRefusedOverMCP: the MCP server never takes file
// content or a path for an upload from a tool call (0.1.8 behavior), even with
// an upload root set. Every multipart endpoint is refused by execute before any
// request, confirmed or not; the endpoint table names no file parameter; and
// the typed-tool upload resolver refuses base64 content and a path alike.
func TestUploadEndpointsAreRefusedOverMCP(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "users.csv"), []byte("email\na@example.com\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(mcpUploadRootEnvVar, root)

	uploads := 0
	for i := range codeOrchEndpoints {
		ep := &codeOrchEndpoints[i]
		if len(ep.FileParams) > 0 {
			t.Errorf("%s names file params %v; an MCP upload must not be offered", ep.ID, ep.FileParams)
		}
		if !ep.Multipart {
			continue
		}
		uploads++
		for _, confirm := range []bool{false, true} {
			var req mcplib.CallToolRequest
			req.Params.Arguments = map[string]any{"endpoint_id": ep.ID, "params": map[string]any{"file": "aGVsbG8=", "invitations": "users.csv"}, "confirm": confirm}
			res, err := codeOrchInvoke(context.Background(), req, true)
			if err != nil || res == nil || !res.IsError {
				t.Fatalf("%s confirm=%v: want a refusal, got %#v (%v)", ep.ID, confirm, res, err)
			}
			text := res.Content[0].(mcplib.TextContent).Text
			if !strings.Contains(text, "uploads a file") || !strings.Contains(text, "appdirect-cli ") {
				t.Errorf("%s: refusal does not name the CLI: %s", ep.ID, text)
			}
		}
	}
	if uploads != 6 {
		t.Errorf("multipart endpoints = %d, want the 6 AppDirect upload endpoints", uploads)
	}
	for _, v := range []any{"aGVsbG8=", "users.csv", filepath.Join(root, "users.csv")} {
		if path, cleanup, err := resolveMCPMultipartFile("file", "", v); err == nil || path != "" {
			if cleanup != nil {
				cleanup()
			}
			t.Errorf("typed upload of %q was staged at %q; want a refusal", v, path)
		}
	}
}
