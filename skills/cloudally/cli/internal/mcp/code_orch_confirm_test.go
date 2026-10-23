// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package mcp

import (
	"context"
	"strings"
	"testing"

	mcplib "github.com/mark3labs/mcp-go/mcp"
)

// TestCodeOrchExecuteWriteNeedsConfirm proves a non-GET endpoint returns a
// preview, and never reaches the API client, unless confirm=true is passed.
func TestCodeOrchExecuteWriteNeedsConfirm(t *testing.T) {
	t.Setenv("CLOUDALLY_MCP_READ_ONLY", "")
	for _, ep := range codeOrchEndpoints {
		if ep.Method == "GET" {
			continue
		}
		for _, confirm := range []any{nil, false, "true"} {
			args := map[string]any{"endpoint_id": ep.ID}
			if confirm != nil {
				args["confirm"] = confirm
			}
			result, err := handleCodeOrchExecute(context.Background(), mcplib.CallToolRequest{Params: mcplib.CallToolParams{Arguments: args}})
			if err != nil {
				t.Fatalf("%s %s: transport error: %v", ep.Method, ep.ID, err)
			}
			if result == nil || result.IsError || len(result.Content) == 0 {
				t.Fatalf("%s %s confirm=%v: want a preview, got %#v", ep.Method, ep.ID, confirm, result)
			}
			text, _ := result.Content[0].(mcplib.TextContent)
			if !strings.Contains(text.Text, `"preview":true`) {
				t.Fatalf("%s %s confirm=%v: want a preview, got %q", ep.Method, ep.ID, confirm, text.Text)
			}
		}
		return
	}
	t.Fatal("no non-GET endpoint to exercise")
}

func TestCodeOrchConfirmAccepted(t *testing.T) {
	if !codeOrchConfirmAccepted(true) {
		t.Fatal("true was not accepted")
	}
	for _, v := range []any{nil, false, "true", 1} {
		if codeOrchConfirmAccepted(v) {
			t.Fatalf("%#v was accepted", v)
		}
	}
}
