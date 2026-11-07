// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package mcp

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	"eset-protect-pp-cli/internal/cliutil"
	mcplib "github.com/mark3labs/mcp-go/mcp"
)

// esetPOSTReadIDs are the ESET Connect reads that use POST: a search and two
// batch gets. They change nothing in ESET (handfixes.json:
// mcp-post-reads-are-reads).
var esetPOSTReadIDs = []string{
	"detection-groups-search.detection-groups",
	"detections_v2.batch-get",
	"users-batch-get-users.v1_users_batch_get_users",
}

// TestPOSTReadsRunFromTheReadToolInReadOnlyMode: with
// ESET_PROTECT_MCP_READ_ONLY=1, eset-protect_read serves each POST read
// without confirm, while a state-changing POST is still refused before any
// request leaves.
func TestPOSTReadsRunFromTheReadToolInReadOnlyMode(t *testing.T) {
	var mu sync.Mutex
	var hit []string
	esetFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hit = append(hit, r.Method+" "+r.URL.Path)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"items":[],"nextPageToken":""}`)
	})
	t.Setenv(cliutil.MCPReadOnlyEnvVar, "1")
	for _, id := range esetPOSTReadIDs {
		res, err := handleCodeOrchRead(context.Background(), mcplib.CallToolRequest{Params: mcplib.CallToolParams{
			Arguments: map[string]any{"endpoint_id": id, "params": map[string]any{}}}})
		if err != nil {
			t.Fatalf("%s: %v", id, err)
		}
		if text := toolText(t, res); res.IsError {
			t.Fatalf("%s was refused in read-only mode: %s", id, text)
		}
	}
	res, err := handleCodeOrchExecute(context.Background(), mcplib.CallToolRequest{Params: mcplib.CallToolParams{
		Arguments: map[string]any{"endpoint_id": "incidents.close", "params": map[string]any{"incidentUuid": "00000000-0000-0000-0000-000000000001"}, "confirm": true}}})
	if err != nil {
		t.Fatal(err)
	}
	if text := toolText(t, res); !res.IsError || !strings.Contains(text, "refusing") {
		t.Fatalf("a state-changing POST was not refused in read-only mode: %s", text)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(hit) != len(esetPOSTReadIDs) {
		t.Fatalf("want one request per POST read and none for the write, got %v", hit)
	}
	for _, h := range hit {
		if !strings.HasPrefix(h, "POST ") || !(strings.HasSuffix(h, ":search") || strings.HasSuffix(h, ":batchGet") || strings.HasSuffix(h, ":batchGetUsers")) {
			t.Fatalf("unexpected request %q", h)
		}
	}
}
