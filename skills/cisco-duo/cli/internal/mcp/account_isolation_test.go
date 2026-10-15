// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"cisco-duo-pp-cli/internal/client"
	mcplib "github.com/mark3labs/mcp-go/mcp"
)

// TestMCPExecuteRequestAccountMustMatchPin is the MCP twin of the CLI
// account-isolation tests (internal/cli/account_isolation_test.go): an
// execute argument account_id is a request-level subaccount, so a server
// pinned with CISCO_DUO_ACCOUNT_ID refuses one that names another customer,
// for reads (query) and writes (body). An unpinned server keeps per-request
// routing, because execute never reads or writes the local store.
func TestMCPExecuteRequestAccountMustMatchPin(t *testing.T) {
	const acctA, acctB = "DAAAAAAAAAAAAAAAAAAA", "DBBBBBBBBBBBBBBBBBBB"
	var mu sync.Mutex
	var hits []string
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		acct := r.URL.Query().Get("account_id")
		if s, ok := body["account_id"].(string); ok {
			acct = s
		}
		mu.Lock()
		hits = append(hits, r.Method+" "+r.URL.Path+" "+acct)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/accounts/v1/account/list" {
			host := srv.Listener.Addr().String()
			fmt.Fprintf(w, `{"stat":"OK","response":[{"account_id":%q,"name":"Acme","api_hostname":%q},{"account_id":%q,"name":"Bravo","api_hostname":%q}]}`, acctA, host, acctB, host)
			return
		}
		fmt.Fprint(w, `{"stat":"OK","response":[]}`)
	}))
	defer srv.Close()
	reached := func(method, path, acct string) bool {
		mu.Lock()
		defer mu.Unlock()
		for _, h := range hits {
			if h == method+" "+path+" "+acct {
				return true
			}
		}
		return false
	}

	resetMCPPathEnv(t)
	t.Setenv("CISCO_DUO_BASE_URL", srv.URL)
	t.Setenv("CISCO_DUO_API_HOST", srv.Listener.Addr().String())
	t.Setenv("CISCO_DUO_IKEY", "DIMCPISOLATION000001")
	t.Setenv("CISCO_DUO_SKEY", "SECRETKEY")
	exec := func(endpoint string, params map[string]any) (bool, string) {
		t.Helper()
		res, err := handleCodeOrchExecute(context.Background(), mcplib.CallToolRequest{Params: mcplib.CallToolParams{
			Arguments: map[string]any{"endpoint_id": endpoint, "params": params},
		}})
		if err != nil || res == nil {
			t.Fatalf("execute %s: %v", endpoint, err)
		}
		text := ""
		if len(res.Content) > 0 {
			if c, ok := res.Content[0].(mcplib.TextContent); ok {
				text = c.Text
			}
		}
		return res.IsError, text
	}

	t.Setenv(client.DuoAccountEnv, acctA)
	if isErr, text := exec("users.list", map[string]any{"account_id": acctB}); !isErr || !strings.Contains(text, "pinned") {
		t.Fatalf("a read naming another subaccount on a pinned server must be refused: %v %s", isErr, text)
	}
	if isErr, text := exec("users.create", map[string]any{"account_id": "Bravo", "username": "bob"}); !isErr || !strings.Contains(text, "pinned") {
		t.Fatalf("a write naming another subaccount on a pinned server must be refused: %v %s", isErr, text)
	}
	if reached("GET", "/admin/v1/users", acctB) || reached("POST", "/admin/v1/users", acctB) {
		t.Fatalf("a refused execute reached customer B: %v", hits)
	}
	if isErr, text := exec("users.list", map[string]any{"account_id": "acme"}); isErr || !reached("GET", "/admin/v1/users", acctA) {
		t.Fatalf("naming the pinned subaccount must pass: %v %s %v", isErr, text, hits)
	}

	t.Setenv(client.DuoAccountEnv, "")
	if isErr, text := exec("users.list", map[string]any{"account_id": acctB}); isErr || !reached("GET", "/admin/v1/users", acctB) {
		t.Fatalf("an unpinned server routes execute per request (it stores nothing): %v %s %v", isErr, text, hits)
	}
}
