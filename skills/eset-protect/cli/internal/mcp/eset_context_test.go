// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package mcp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"eset-protect-pp-cli/internal/store"
	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// The sql tool's documented example, run verbatim against a mirror holding
// the shape sync writes (resource_type 'devices', ESET's displayName field).
const esetSQLExample = "SELECT json_extract(data,'$.displayName') AS name FROM resources WHERE resource_type='devices'"

// TestMCPContextDescribesESETSurface: the context tool is what an agent reads
// first, so it must describe ESET Connect (pageSize/pageToken paging, the
// registry tools, the local mirror) and not the generator's project-management
// boilerplate (handfixes.json: mcp-context-describes-eset).
func TestMCPContextDescribesESETSurface(t *testing.T) {
	s := server.NewMCPServer("eset-protect", "test")
	RegisterTools(s)
	result, err := handleContext(s)(context.Background(), mcplib.CallToolRequest{})
	if err != nil || result == nil || result.IsError {
		t.Fatalf("context failed: %v", err)
	}
	text := mcpTextContent(t, result)
	var payload struct {
		Archetype   string   `json:"archetype"`
		ToolSurface string   `json:"tool_surface"`
		QueryTips   []string `json:"query_tips"`
		Playbook    []struct {
			Topic   string `json:"topic"`
			Insight string `json:"insight"`
		} `json:"playbook"`
	}
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Archetype != "security" {
		t.Errorf("archetype = %q, want security", payload.Archetype)
	}
	for _, want := range []string{"eset-protect_search", "eset-protect_execute", "eset-protect_get", "sync"} {
		if !strings.Contains(payload.ToolSurface, want) {
			t.Errorf("tool_surface does not mention %s: %q", want, payload.ToolSurface)
		}
	}
	tips := strings.Join(payload.QueryTips, "\n")
	for _, want := range []string{"pageToken", "nextPageToken", "resources(resource_type, id, data)", "[redacted]", "ESET_PROTECT_MCP_READ_ONLY"} {
		if !strings.Contains(tips, want) {
			t.Errorf("query_tips do not mention %q:\n%s", want, tips)
		}
	}
	for _, banned := range []string{"default 100", "PM APIs", "assignee", "Load analysis", "Bulk operations", "Finding stale work", "typed endpoint tools"} {
		if strings.Contains(text, banned) {
			t.Errorf("context still carries generator boilerplate %q", banned)
		}
	}

	sqlTool := s.GetTool("sql")
	if sqlTool == nil {
		t.Fatal("sql tool not registered")
	}
	raw, _ := json.Marshal(sqlTool.Tool.InputSchema)
	if !strings.Contains(string(raw), esetSQLExample) {
		t.Fatalf("sql tool description does not carry the tested example %q: %s", esetSQLExample, raw)
	}
}

// The sql example and the search tool both work against what sync writes.
func TestMCPSQLExampleAndSearchWorkOnSyncedDevices(t *testing.T) {
	resetMCPPathEnv(t)
	path, err := mcpDBPath()
	if err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := db.UpsertBatch("devices", []json.RawMessage{json.RawMessage(`{"uuid":"00000000-0000-0000-0000-000000000001","displayName":"FRONTDESK-01"}`)}); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()

	res, err := handleSQL(context.Background(), mcplib.CallToolRequest{Params: mcplib.CallToolParams{Arguments: map[string]any{"query": esetSQLExample}}})
	if err != nil || res.IsError || !strings.Contains(mcpTextContent(t, res), "FRONTDESK-01") {
		t.Fatalf("documented sql example found nothing: %v %s", err, mcpTextContent(t, res))
	}
	res, err = handleSearch(context.Background(), mcplib.CallToolRequest{Params: mcplib.CallToolParams{Arguments: map[string]any{"query": "FRONTDESK"}}})
	if err != nil || res.IsError || !strings.Contains(mcpTextContent(t, res), "FRONTDESK-01") {
		t.Fatalf("search found nothing in synced devices: %v %s", err, mcpTextContent(t, res))
	}
}
