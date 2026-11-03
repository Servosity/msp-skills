// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/proxmox/handfixes.json under
// "code-orch-get-only".

package mcp

import (
	"context"
	"strings"
	"testing"

	mcplib "github.com/mark3labs/mcp-go/mcp"
)

// Read-only toward Proxmox must not rest on the generated endpoint table alone:
// every entry is a GET today, and execute refuses anything else regardless.
func TestCodeOrchIsGetOnly(t *testing.T) {
	for _, ep := range codeOrchEndpoints {
		if ep.Method != "GET" {
			t.Errorf("code-orchestration endpoint %s is %s; this connector must not write to Proxmox", ep.ID, ep.Method)
		}
	}
	codeOrchEndpoints = append(codeOrchEndpoints, codeOrchEndpoint{ID: "zz.write", Method: "POST", Path: "/nodes"})
	defer func() { codeOrchEndpoints = codeOrchEndpoints[:len(codeOrchEndpoints)-1] }()
	res, err := handleCodeOrchExecute(context.Background(), mcplib.CallToolRequest{Params: mcplib.CallToolParams{
		Arguments: map[string]any{"endpoint_id": "zz.write"},
	}})
	if err != nil || res == nil || !res.IsError {
		t.Fatalf("a POST endpoint was not refused: %v %+v", err, res)
	}
	if text, _ := res.Content[0].(mcplib.TextContent); !strings.Contains(text.Text, "never writes to Proxmox") {
		t.Fatalf("unexpected refusal: %+v", res.Content)
	}
}
