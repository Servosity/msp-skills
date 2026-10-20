// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/synology/handfixes.json
// under the "mcp-context-describes-dsm" entry.

package mcp

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// TestMCPContextDescribesDSM: the context tool ("call this first") shipped
// the template's description of a cursor-paged developer-platform API with a
// sync-everything store, so an agent paged with a cursor, called abb, storage
// and system "searchable" and ran sql against resource_type='abb' rows that
// sync never stores.
func TestMCPContextDescribesDSM(t *testing.T) {
	s := sweepMCPServer(t, nil, "")
	text, isErr := callTool(t, s, "context", map[string]any{})
	if isErr {
		t.Fatalf("context failed: %s", text)
	}
	for _, bad := range []string{"cursor-based paging", "developer-platform", "Developer platform", "resource_type='abb'", "Set during initial auth setup"} {
		if strings.Contains(text, bad) {
			t.Fatalf("context still says %q", bad)
		}
	}
	var ctx struct {
		Archetype   string   `json:"archetype"`
		ToolSurface string   `json:"tool_surface"`
		QueryTips   []string `json:"query_tips"`
		Resources   []struct {
			Name       string `json:"name"`
			Searchable bool   `json:"searchable"`
			Syncable   bool   `json:"syncable"`
		} `json:"resources"`
	}
	if err := json.Unmarshal([]byte(text), &ctx); err != nil {
		t.Fatalf("context is not JSON: %v", err)
	}
	if ctx.Archetype != "infrastructure" {
		t.Fatalf("archetype = %q", ctx.Archetype)
	}
	tips := strings.Join(ctx.QueryTips, " ")
	for _, want := range []string{"offset and limit", "fleet_sync", "never contain ABB jobs"} {
		if !strings.Contains(tips, want) {
			t.Fatalf("query_tips do not say %q: %s", want, tips)
		}
	}
	if !strings.Contains(ctx.ToolSurface, "api= and method=") || !strings.Contains(ctx.ToolSurface, "fleet_sync") {
		t.Fatalf("tool_surface does not describe the DSM surface: %s", ctx.ToolSurface)
	}
	for _, r := range ctx.Resources {
		if r.Searchable && !r.Syncable {
			t.Fatalf("resource %s is marked searchable but sync never stores it", r.Name)
		}
	}
	sql := s.ListTools()["sql"].Tool
	if strings.Contains(sql.Description+string(mustJSON(t, sql.InputSchema)), "resource_type='abb'") {
		t.Fatal("the sql tool still points agents at resource_type='abb'")
	}
	if msg := mcpSQLQueryError(t.Context(), errors.New("no such table: abb_tasks")); strings.Contains(msg, "resource_type='abb'") || !strings.Contains(msg, "fleet_status") {
		t.Fatalf("the no-such-table hint still points at abb rows: %s", msg)
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
