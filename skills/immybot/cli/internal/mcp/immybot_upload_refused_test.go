// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package mcp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	mcplib "github.com/mark3labs/mcp-go/mcp"
)

// TestUploadEndpointsAreRefusedOverMCP: the MCP server never takes file
// content or a path for an upload from a tool call (0.1.10 behavior). Every
// multipart endpoint is refused before any request, confirmed or not, and the
// endpoint table names no file parameter.
func TestUploadEndpointsAreRefusedOverMCP(t *testing.T) {
	var hits atomic.Int32
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer fixture.Close()
	requestContractConfig(t, fixture.URL)
	t.Setenv(mcpUploadRootEnvVar, t.TempDir())

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
			args := map[string]any{"endpoint_id": ep.ID, "params": map[string]any{"file": "aGVsbG8=", "file_filename": "x.bin"}, "confirm": confirm}
			var req mcplib.CallToolRequest
			req.Params.Arguments = args
			res, err := handleCodeOrchExecute(context.Background(), req)
			if err != nil || res == nil || !res.IsError {
				t.Fatalf("%s confirm=%v: want a refusal, got %#v (%v)", ep.ID, confirm, res, err)
			}
			text := res.Content[0].(mcplib.TextContent).Text
			if !strings.Contains(text, "uploads a file") || !strings.Contains(text, "immybot-cli") {
				t.Errorf("%s: refusal does not name the CLI: %s", ep.ID, text)
			}
		}
	}
	if uploads != 6 {
		t.Errorf("multipart endpoints = %d, want the 6 ImmyBot upload endpoints", uploads)
	}
	if n := hits.Load(); n != 0 {
		t.Errorf("an upload reached the vendor %d time(s)", n)
	}
}
