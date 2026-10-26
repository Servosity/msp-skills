// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	mcplib "github.com/mark3labs/mcp-go/mcp"
)

// An agent walking a NinjaOne list through ninjaone_read and next_cursor must
// see every row exactly once (class ninja-paging-every-reader, and class
// mcp-continuation-never-replays-a-stateful-cursor).

func ninjaMCPEnv(t *testing.T, h http.HandlerFunc) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("NINJAONE_HOME", home)
	t.Setenv("NINJAONE_BASE_URL", srv.URL)
	t.Setenv("NINJAONE_ACCESS_TOKEN", "fixture-token")
	t.Setenv("NINJAONE_CLIENT_ID", "")
	t.Setenv("NINJAONE_CLIENT_SECRET", "")
	t.Setenv("PRINTING_PRESS_CLIENT_PROFILE", "")
	cfg := filepath.Join(home, ".config", "ninjaone-cli", "config.toml")
	if err := os.MkdirAll(filepath.Dir(cfg), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg, []byte("access_token = \"fixture-token\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("NINJAONE_CONFIG", cfg)
}

// walkMCP follows next_cursor (sent under cursorParam) and returns every row's key field.
func walkMCP(t *testing.T, endpointID, cursorParam, key string, base map[string]any) ([]string, int) {
	t.Helper()
	var keys []string
	cursor := ""
	calls := 0
	for calls < 200 {
		params := map[string]any{}
		for k, v := range base {
			params[k] = v
		}
		if cursor != "" {
			params[cursorParam] = cursor
		}
		req := mcplib.CallToolRequest{}
		req.Params.Arguments = map[string]any{"endpoint_id": endpointID, "params": params}
		result, err := handleCodeOrchRead(context.Background(), req)
		calls++
		if err != nil {
			t.Fatal(err)
		}
		text := ""
		for _, c := range result.Content {
			if tc, ok := c.(mcplib.TextContent); ok {
				text += tc.Text
			}
		}
		if result.IsError {
			t.Fatalf("read %d failed: %s", calls, text)
		}
		var page map[string]json.RawMessage
		if err := json.Unmarshal([]byte(text), &page); err != nil {
			t.Fatalf("page %d is not JSON: %v\n%s", calls, err, text)
		}
		if inner, ok := page["data"]; ok {
			_ = json.Unmarshal(inner, &page)
		}
		var rows []map[string]any
		for _, field := range []string{"items", "results"} {
			if raw, ok := page[field]; ok {
				_ = json.Unmarshal(raw, &rows)
			}
		}
		for _, r := range rows {
			keys = append(keys, fmt.Sprint(r[key]))
		}
		cursor = ""
		if raw, ok := page["next_cursor"]; ok {
			_ = json.Unmarshal(raw, &cursor)
		}
		if cursor == "" {
			return keys, calls
		}
	}
	t.Fatalf("walk did not end after %d calls", calls)
	return nil, calls
}

func assertEveryRowOnce(t *testing.T, keys []string, total int) {
	t.Helper()
	seen := map[string]int{}
	for _, k := range keys {
		seen[k]++
	}
	var missing, dup []int
	for i := 1; i <= total; i++ {
		switch seen[strconv.Itoa(i)] {
		case 0:
			missing = append(missing, i)
		case 1:
		default:
			dup = append(dup, i)
		}
	}
	if len(missing) > 0 || len(dup) > 0 || len(keys) != total {
		t.Fatalf("walk returned %d rows for %d: %d missing (first %v), %d duplicated (first %v)",
			len(keys), total, len(missing), missing[:min(5, len(missing))], len(dup), dup[:min(5, len(dup))])
	}
}

func TestMCPReadWalksTheAnchorNaturalIdKeyset(t *testing.T) {
	const total = 130
	ninjaMCPEnv(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/ticketing/app-user-contact" {
			http.NotFound(w, r)
			return
		}
		size, _ := strconv.Atoi(r.URL.Query().Get("pageSize"))
		if size <= 0 {
			size = 100
		}
		anchor, _ := strconv.Atoi(r.URL.Query().Get("anchorNaturalId"))
		var rows []string
		for n := anchor + 1; n <= total && len(rows) < size; n++ {
			rows = append(rows, fmt.Sprintf(`{"uid":"u-%d","id":%d,"naturalId":%d,"userType":"END_USER"}`, n, 5000+n, n))
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, "[%s]", strings.Join(rows, ","))
	})
	keys, _ := walkMCP(t, "ticketing.get-all-user-and-contacts", "anchorNaturalId", "naturalId", map[string]any{"pageSize": float64(40)})
	assertEveryRowOnce(t, keys, total)
}

// NinjaOne report cursors are stateful: resending the same token name returns the
// NEXT page (the offset advances server-side), so a local continuation inside a
// page that the MCP result budget split must never resend it.
func TestMCPReportContinuationNeverReplaysTheCursor(t *testing.T) {
	const total = 250
	var mu sync.Mutex
	served := 0
	ninjaMCPEnv(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/queries/volumes" {
			http.NotFound(w, r)
			return
		}
		mu.Lock()
		defer mu.Unlock()
		if r.URL.Query().Get("cursor") == "" {
			served = 0
		}
		size, _ := strconv.Atoi(r.URL.Query().Get("pageSize"))
		if size <= 0 {
			size = 100
		}
		offset := served
		var rows []string
		for i := offset; i < total && len(rows) < size; i++ {
			rows = append(rows, fmt.Sprintf(`{"deviceId":%d,"name":"C:","capacity":100,"freeSpace":40}`, i+1))
		}
		served += len(rows)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"cursor":{"name":"SESSION_TOKEN","offset":%d,"count":%d,"expires":"2026-01-01T00:00:00Z"},"results":[%s]}`,
			offset, len(rows), strings.Join(rows, ","))
	})
	// pageSize 100 is over the 50-item MCP budget, so every API page is split.
	keys, _ := walkMCP(t, "queries.get-volumes", "cursor", "deviceId", map[string]any{"pageSize": float64(100)})
	assertEveryRowOnce(t, keys, total)
}

func TestMCPRetainedContinuationResentWithOtherParamsIsRefused(t *testing.T) {
	var mu sync.Mutex
	requests := 0
	ninjaMCPEnv(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests++
		mu.Unlock()
		var rows []string
		for i := 0; i < 100; i++ {
			rows = append(rows, fmt.Sprintf(`{"deviceId":%d,"name":"C:"}`, i+1))
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"cursor":{"name":"T","offset":0,"count":100},"results":[%s]}`, strings.Join(rows, ","))
	})
	call := func(params map[string]any) (string, bool) {
		req := mcplib.CallToolRequest{}
		req.Params.Arguments = map[string]any{"endpoint_id": "queries.get-volumes", "params": params}
		result, err := handleCodeOrchRead(context.Background(), req)
		if err != nil {
			t.Fatal(err)
		}
		text := ""
		for _, c := range result.Content {
			if tc, ok := c.(mcplib.TextContent); ok {
				text += tc.Text
			}
		}
		return text, result.IsError
	}
	text, _ := call(map[string]any{"pageSize": float64(100)})
	var page map[string]any
	_ = json.Unmarshal([]byte(text), &page)
	next, _ := page["next_cursor"].(string)
	if next == "" {
		t.Fatalf("no continuation: %s", text)
	}
	before := requests
	if text, isErr := call(map[string]any{"pageSize": float64(99), "cursor": next}); !isErr || !strings.Contains(text, "different request") {
		t.Fatalf("a continuation resent with other params was not refused: %s", text)
	}
	if text, isErr := call(map[string]any{"pageSize": float64(100), "cursor": next}); isErr {
		t.Fatalf("the continuation with its own params failed: %s", text)
	}
	if requests != before {
		t.Fatalf("serving a retained continuation made %d upstream requests, want 0", requests-before)
	}
}

// pageSize above what NinjaOne serves per page: an oversized request a server
// answers in 100-row pages must not end the walk on the first page. MCP compares
// a page against min(requested, the endpoint's default), as sync does.
func TestMCPOversizedPageSizeDoesNotEndAClampedWalk(t *testing.T) {
	const total = 250
	var mu sync.Mutex
	served := 0
	ninjaMCPEnv(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.URL.Query().Get("cursor") == "" {
			served = 0
		}
		offset := served
		var rows []string
		for i := offset; i < total && len(rows) < 100; i++ {
			rows = append(rows, fmt.Sprintf(`{"deviceId":%d,"name":"C:"}`, i+1))
		}
		served += len(rows)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"cursor":{"name":"C","offset":%d,"count":%d},"results":[%s]}`, offset, len(rows), strings.Join(rows, ","))
	})
	keys, _ := walkMCP(t, "queries.get-volumes", "cursor", "deviceId", map[string]any{"pageSize": float64(1000)})
	assertEveryRowOnce(t, keys, total)
}
