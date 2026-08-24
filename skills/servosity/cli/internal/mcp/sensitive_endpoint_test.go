// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package mcp

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	"servosity-msp-pp-cli/internal/store"
)

const sensitiveTokenEndpointID = "current-user.api-token-list"

func TestSensitiveTokenEndpointIsAbsentFromCodeOrchestration(t *testing.T) {
	if ep := findCodeOrchEndpoint(sensitiveTokenEndpointID); ep != nil {
		t.Fatalf("sensitive endpoint remains registered: %#v", ep)
	}

	searchResult, err := handleCodeOrchSearch(context.Background(), mcplib.CallToolRequest{Params: mcplib.CallToolParams{
		Arguments: map[string]any{"query": "current user api token", "limit": float64(codeOrchSearchMaxLimit)},
	}})
	if err != nil {
		t.Fatalf("search transport error: %v", err)
	}
	if text := mcpTextContent(t, searchResult); strings.Contains(text, sensitiveTokenEndpointID) {
		t.Fatalf("search advertised sensitive endpoint: %s", text)
	}

	for name, call := range map[string]func(context.Context, mcplib.CallToolRequest) (*mcplib.CallToolResult, error){
		"get":     handleCodeOrchGet,
		"execute": handleCodeOrchExecute,
	} {
		result, err := call(context.Background(), mcplib.CallToolRequest{Params: mcplib.CallToolParams{
			Arguments: map[string]any{"endpoint_id": sensitiveTokenEndpointID},
		}})
		if err != nil {
			t.Fatalf("%s transport error: %v", name, err)
		}
		if result == nil || !result.IsError || !strings.Contains(mcpTextContent(t, result), "unknown endpoint_id") {
			t.Fatalf("%s result = %#v, want unknown endpoint error", name, result)
		}
	}
}

func TestMCPContextOmitsSensitiveTokenEndpoint(t *testing.T) {
	result, err := handleContext(context.Background(), mcplib.CallToolRequest{})
	if err != nil {
		t.Fatalf("handleContext transport error: %v", err)
	}
	var payload struct {
		Resources []struct {
			Name      string   `json:"name"`
			Endpoints []string `json:"endpoints"`
		} `json:"resources"`
	}
	if err := json.Unmarshal([]byte(mcpTextContent(t, result)), &payload); err != nil {
		t.Fatalf("decode context: %v", err)
	}
	for _, resource := range payload.Resources {
		if resource.Name != "current-user" {
			continue
		}
		for _, endpoint := range resource.Endpoints {
			if endpoint == "api-token-list" {
				t.Fatal("MCP context advertised sensitive token endpoint")
			}
		}
		return
	}
	t.Fatal("MCP context omitted current-user resource")
}

func TestMCPReadOnlyStoreRefusesPrePurgeSchema(t *testing.T) {
	resetMCPPathEnv(t)
	dbPath, err := mcpDBPath()
	if err != nil {
		t.Fatalf("resolve MCP store path: %v", err)
	}
	writable, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	if err := writable.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}

	raw, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open raw store: %v", err)
	}
	const fixtureSecret = "fixture-pre-migration-token"
	if _, err := raw.Exec(`INSERT INTO resources (id, resource_type, data) VALUES ('token-row', 'current-user-api-token', ?)`, fmt.Sprintf(`{"token":%q}`, fixtureSecret)); err != nil {
		raw.Close()
		t.Fatalf("seed sensitive row: %v", err)
	}
	if _, err := raw.Exec(`PRAGMA user_version = 9`); err != nil {
		raw.Close()
		t.Fatalf("stamp old schema: %v", err)
	}
	if err := raw.Close(); err != nil {
		t.Fatalf("close raw store: %v", err)
	}

	db, toolErr := openMCPReadOnlyStore(context.Background(), dbPath)
	if db != nil {
		db.Close()
		t.Fatal("MCP opened a pre-purge store")
	}
	if toolErr == nil || !toolErr.IsError {
		t.Fatalf("pre-purge store result = %#v, want MCP error", toolErr)
	}
	text := mcpTextContent(t, toolErr)
	if !strings.Contains(text, "security migration") || strings.Contains(text, fixtureSecret) {
		t.Fatalf("pre-purge refusal = %q", text)
	}

	for name, call := range map[string]func() (*mcplib.CallToolResult, error){
		"search": func() (*mcplib.CallToolResult, error) {
			return handleSearch(context.Background(), mcplib.CallToolRequest{Params: mcplib.CallToolParams{
				Arguments: map[string]any{"query": fixtureSecret},
			}})
		},
		"sql": func() (*mcplib.CallToolResult, error) {
			return handleSQL(context.Background(), mcplib.CallToolRequest{Params: mcplib.CallToolParams{
				Arguments: map[string]any{"query": "SELECT data FROM resources"},
			}})
		},
	} {
		result, err := call()
		if err != nil {
			t.Fatalf("%s transport error: %v", name, err)
		}
		resultText := mcpTextContent(t, result)
		if result == nil || !result.IsError || !strings.Contains(resultText, "security migration") || strings.Contains(resultText, fixtureSecret) {
			t.Fatalf("%s pre-purge result = %#v text=%q", name, result, resultText)
		}
	}

	raw, err = sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("reopen raw store: %v", err)
	}
	defer raw.Close()
	var version, rows int
	if err := raw.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatalf("read schema version: %v", err)
	}
	if err := raw.QueryRow(`SELECT COUNT(*) FROM resources WHERE resource_type = 'current-user-api-token'`).Scan(&rows); err != nil {
		t.Fatalf("count sensitive rows: %v", err)
	}
	if version != 9 || rows != 1 {
		t.Fatalf("MCP mutated refused store: version=%d rows=%d", version, rows)
	}
}
