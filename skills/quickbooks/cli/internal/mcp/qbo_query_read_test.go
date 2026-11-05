// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
package mcp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	mcplib "github.com/mark3labs/mcp-go/mcp"
)

// Every QuickBooks Online list is a GET /query (Intuit's query endpoint takes
// only SELECT statements), so query.run is a read. Classed as a write, it gated
// every endpoint that shares GET /query and quickbooks_read could list nothing.
func TestQBOQueryEndpointsAreReads(t *testing.T) {
	hits := make(chan string, 64)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits <- r.Method + " " + r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"QueryResponse":{}}`))
	}))
	defer srv.Close()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("QUICKBOOKS_HOME", home)
	t.Setenv("QUICKBOOKS_BASE_URL", srv.URL)
	t.Setenv("QUICKBOOKS_ACCESS_TOKEN", "fixture-token")
	t.Setenv("QUICKBOOKS_CLIENT_ID", "")
	t.Setenv("QUICKBOOKS_CLIENT_SECRET", "")
	t.Setenv("PRINTING_PRESS_CLIENT_PROFILE", "")
	cfg := filepath.Join(home, ".config", "quickbooks-cli", "config.toml")
	if err := os.MkdirAll(filepath.Dir(cfg), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg, []byte("access_token = \"fixture-token\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("QUICKBOOKS_CONFIG", cfg)

	var ids []string
	for _, ep := range codeOrchEndpoints {
		if ep.Method == "GET" && ep.Path == "/query" {
			ids = append(ids, ep.ID)
		}
	}
	if len(ids) < 9 {
		t.Fatalf("expected the 8 entity lists plus query.run on GET /query, found %v", ids)
	}
	for _, readOnly := range []string{"", "1"} {
		t.Setenv("QUICKBOOKS_MCP_READ_ONLY", readOnly)
		for _, id := range ids {
			params := map[string]any{}
			if id == "query.run" {
				params["query"] = "select * from Customer maxresults 1"
			}
			req := mcplib.CallToolRequest{}
			req.Params.Arguments = map[string]any{"endpoint_id": id, "params": params}
			res, err := handleCodeOrchRead(context.Background(), req)
			if err != nil || res == nil || res.IsError {
				t.Fatalf("read_only=%q %s: quickbooks_read refused: %v %s", readOnly, id, err, resultText(res))
			}
			select {
			case got := <-hits:
				if got != "GET /query" {
					t.Fatalf("%s sent %s", id, got)
				}
			default:
				t.Fatalf("read_only=%q %s: nothing reached the server", readOnly, id)
			}
		}
	}
	// The execute tool still refuses a read instead of sending it.
	req := mcplib.CallToolRequest{}
	req.Params.Arguments = map[string]any{"endpoint_id": "query.run", "params": map[string]any{"query": "select * from Customer"}, "confirm": true}
	res, err := handleCodeOrchExecute(context.Background(), req)
	if err != nil || res == nil || !res.IsError || !strings.Contains(resultText(res), "read-only") {
		t.Fatalf("execute on query.run: %v %s", err, resultText(res))
	}
}

func resultText(res *mcplib.CallToolResult) string {
	if res == nil {
		return "<nil>"
	}
	var b strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(mcplib.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String()
}
