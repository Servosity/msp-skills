// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package mcp

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"bitdefender-pp-cli/internal/cliutil"

	mcplib "github.com/mark3labs/mcp-go/mcp"
)

// Under BITDEFENDER_MCP_READ_ONLY=1 a listed read runs through bitdefender_read
// and returns the vendor's answer; a read that returns or takes a credential,
// and every write, is refused before any request leaves the machine
// (handfixes.json: mcp-read-only-allows-gravityzone-reads).
func TestReadOnlySwitchRunsListedReadsAndRefusesCredentialReads(t *testing.T) {
	var mu sync.Mutex
	var methods []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		methods = append(methods, string(body))
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"jsonrpc":"2.0","id":"`+rpcID(string(body))+`","result":{"total":1,"page":1,"perPage":30,"pagesCount":1,"items":[{"id":"e1","name":"ACME-PC-01"}]}}`)
	}))
	defer srv.Close()
	resetMCPPathEnv(t)
	t.Setenv("BITDEFENDER_BASE_URL", srv.URL)
	t.Setenv("BITDEFENDER_API_KEY", "test-key-0123456789")
	t.Setenv(cliutil.MCPReadOnlyEnvVar, "1")
	call := func(h func(context.Context, mcplib.CallToolRequest) (*mcplib.CallToolResult, error), args map[string]any) string {
		res, err := h(context.Background(), mcplib.CallToolRequest{Params: mcplib.CallToolParams{Arguments: args}})
		if err != nil {
			t.Fatal(err)
		}
		if res == nil || len(res.Content) == 0 {
			return ""
		}
		text, _ := res.Content[0].(mcplib.TextContent)
		return text.Text
	}
	if got := call(handleCodeOrchRead, map[string]any{"endpoint_id": "endpoints.list"}); !strings.Contains(got, "ACME-PC-01") {
		t.Fatalf("listed read under read-only did not return the vendor answer: %s", got)
	}
	mu.Lock()
	seen := len(methods)
	mu.Unlock()
	for _, id := range []string{"push.get", "license.get", "policies.get", "inventory.list", "packages.get", "packages.install-links", "integrations.get", "companies.get-by-user"} {
		if got := call(handleCodeOrchRead, map[string]any{"endpoint_id": id}); !strings.Contains(got, "refusing") {
			t.Fatalf("credential-bearing read %s ran under read-only: %s", id, got)
		}
	}
	for _, id := range []string{"isolation.isolate", "endpoints.delete", "endpoints.list"} {
		if got := call(handleCodeOrchExecute, map[string]any{"endpoint_id": id, "confirm": true}); !strings.Contains(got, "refusing") {
			t.Fatalf("bitdefender_execute %s ran under read-only: %s", id, got)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if len(methods) != seen {
		t.Fatalf("refused calls reached the server: %v", methods[seen:])
	}
}

func rpcID(body string) string {
	_, rest, ok := strings.Cut(body, `"id":"`)
	if !ok {
		return ""
	}
	id, _, _ := strings.Cut(rest, `"`)
	return id
}
