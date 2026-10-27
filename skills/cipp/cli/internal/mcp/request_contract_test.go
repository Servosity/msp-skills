// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package mcp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	mcplib "github.com/mark3labs/mcp-go/mcp"
)

func TestEveryEndpointHasARequestContract(t *testing.T) {
	for _, ep := range codeOrchEndpoints {
		c, ok := requestContracts[ep.ID]
		if !ok {
			t.Fatalf("%s has no request contract; regenerate request_contracts.json", ep.ID)
		}
		if _, ok := c["properties"].(map[string]any); !ok {
			t.Fatalf("%s contract has no properties object", ep.ID)
		}
	}
	if len(requestContracts) != len(codeOrchEndpoints) {
		t.Fatalf("%d contracts for %d endpoints: stale request_contracts.json", len(requestContracts), len(codeOrchEndpoints))
	}
}

func TestDeleteWithBodyPreviewKeepsTheBody(t *testing.T) {
	ep := findCodeOrchEndpoint("exec-api-client.delete")
	if ep == nil || !codeOrchHasBody(ep) {
		t.Fatal("exec-api-client.delete should be a DELETE that carries a body")
	}
	result, err := handleCodeOrchExecute(context.Background(), mcplib.CallToolRequest{Params: mcplib.CallToolParams{
		Arguments: map[string]any{"endpoint_id": ep.ID, "params": map[string]any{"ClientId": "abc", "RemoveAppReg": true}},
	}})
	if err != nil || result == nil || result.IsError {
		t.Fatalf("preview failed: %v %+v", err, result)
	}
	var preview struct {
		Preview bool              `json:"preview"`
		Query   map[string]string `json:"query"`
		Body    map[string]any    `json:"body"`
	}
	if err := json.Unmarshal([]byte(result.Content[0].(mcplib.TextContent).Text), &preview); err != nil {
		t.Fatal(err)
	}
	if !preview.Preview || preview.Body["ClientId"] != "abc" || len(preview.Query) != 0 {
		t.Fatalf("a body-carrying DELETE must send its inputs as the body, not the query: %+v", preview)
	}
}

func TestSearchReturnsParamsSchemaAndFindsBodyFields(t *testing.T) {
	result, err := handleCodeOrchSearch(context.Background(), mcplib.CallToolRequest{Params: mcplib.CallToolParams{
		Arguments: map[string]any{"query": "ConvertToShared offboard"},
	}})
	if err != nil || result == nil || result.IsError {
		t.Fatalf("search failed: %v %+v", err, result)
	}
	text := result.Content[0].(mcplib.TextContent).Text
	if !strings.Contains(text, `"params_schema"`) || !strings.Contains(text, "exec-offboard-user") {
		t.Fatalf("search must rank the endpoint whose body carries the field and show its params_schema:\n%.600s", text)
	}
}
