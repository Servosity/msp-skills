// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package main

import (
	"context"
	"strings"
	"testing"

	mcplib "github.com/mark3labs/mcp-go/mcp"
)

// TestWithholdToolSecretsCoversEveryToolButAConfirmedExecute pins that a
// secret field in any tool result (a read, tail's NDJSON, sql rows) reaches
// the client withheld, and only servosity-msp_execute with confirm: true
// (a JSON boolean) returns it as sent.
func TestWithholdToolSecretsCoversEveryToolButAConfirmedExecute(t *testing.T) {
	body := `{"id":2,"name":"MSP","postmark_server_api_token":"pm-value","LanPassword":"lan-value"}`
	handler := withholdToolSecrets(func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		return mcplib.NewToolResultText(body), nil
	})
	call := func(name string, args map[string]any) string {
		var req mcplib.CallToolRequest
		req.Params.Name = name
		req.Params.Arguments = args
		res, err := handler(context.Background(), req)
		if err != nil {
			t.Fatal(err)
		}
		return res.Content[0].(mcplib.TextContent).Text
	}
	for _, tc := range []struct {
		name string
		args map[string]any
	}{
		{"servosity-msp_read", map[string]any{"endpoint_id": "resellers.read"}},
		{"tail", map[string]any{"resource": "credentials"}},
		{"sql", map[string]any{"query": "select 1"}},
		{"servosity-msp_execute", map[string]any{"confirm": "true"}},
		{"servosity-msp_execute", map[string]any{}},
	} {
		out := call(tc.name, tc.args)
		if strings.Contains(out, "pm-value") || strings.Contains(out, "lan-value") || !strings.Contains(out, `"name":"MSP"`) {
			t.Fatalf("%s %v: %s", tc.name, tc.args, out)
		}
	}
	if out := call("servosity-msp_execute", map[string]any{"confirm": true}); out != body {
		t.Fatalf("a confirmed execute must return the body as sent: %s", out)
	}
}
