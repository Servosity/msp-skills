// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: covers codeOrchBindRemainingPathVars (handfixes.json
// mcp-execute-binds-every-path-variable).

package mcp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	mcplib "github.com/mark3labs/mcp-go/mcp"
)

func TestCodeOrchBindRemainingPathVars(t *testing.T) {
	params := map[string]any{"customerKey": "my_customer", "keep": "x"}
	got := codeOrchBindRemainingPathVars("/admin/directory/v1/customers/{customerKey}", params)
	if got != "/admin/directory/v1/customers/my_customer" {
		t.Fatalf("path: %q", got)
	}
	if _, ok := params["customerKey"]; ok || params["keep"] != "x" {
		t.Fatalf("bound var must leave params, others must stay: %v", params)
	}
	// Two variables, one unsupplied: the supplied one binds and is escaped,
	// the other stays for the client to reject.
	params = map[string]any{"parent": "a/b"}
	got = codeOrchBindRemainingPathVars("/v1/{parent}/x/{missing}", params)
	if got != "/v1/a%2Fb/x/{missing}" {
		t.Fatalf("path: %q", got)
	}
}

// TestCodeOrchExecuteBindsFlagBoundPathVar pins the call site in
// handleCodeOrchExecute: customers.get has no Positional entry for
// {customerKey}, so only the hand-fix can put it in the request path.
func TestCodeOrchExecuteBindsFlagBoundPathVar(t *testing.T) {
	resetMCPPathEnv(t)
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"C0abc"}`))
	}))
	defer srv.Close()
	t.Setenv("GOOGLE_WORKSPACE_BASE_URL", srv.URL)
	t.Setenv("GOOGLE_WORKSPACE_TOKEN", "ya29.test")

	res, err := handleCodeOrchExecute(context.Background(), mcplib.CallToolRequest{Params: mcplib.CallToolParams{
		Arguments: map[string]any{"endpoint_id": "customers.get", "params": map[string]any{"customerKey": "my_customer"}},
	}})
	if err != nil || res == nil || res.IsError {
		t.Fatalf("execute failed: err=%v res=%+v", err, res)
	}
	if gotPath != "/admin/directory/v1/customers/my_customer" {
		t.Fatalf("request path = %q", gotPath)
	}
}
