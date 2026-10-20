// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: an argument a write tool does not declare never reaches ION.

package mcp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	mcplib "github.com/mark3labs/mcp-go/mcp"
)

func TestEndpointWriteRefusesUndeclaredArgument(t *testing.T) {
	resetMCPPathEnv(t)
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	t.Setenv("TD_SYNNEX_BASE_URL", srv.URL)
	t.Setenv("TD_SYNNEX_ACCESS_TOKEN", "test-token")

	bindings := []mcpParamBinding{{PublicName: "customerId", WireName: "customerId", Location: "path"}, {PublicName: "cartId", WireName: "cartId", Location: "path"}}
	call := func(method string, args map[string]any) *mcplib.CallToolResult {
		t.Helper()
		h := makeAPIHandler(method, "/customers/{customerId}/carts/{cartId}", false, false, nil, mcpPageConfig{}, bindings, []string{"customerId", "cartId"})
		req := mcplib.CallToolRequest{}
		req.Params.Arguments = args
		res, err := h(context.Background(), req)
		if err != nil {
			t.Fatalf("handler error: %v", err)
		}
		return res
	}

	res := call("DELETE", map[string]any{"customerId": "1", "cartId": "C1", "dry-run": true})
	if !res.IsError || hits.Load() != 0 {
		t.Fatalf("an undeclared dry-run must refuse the delete before any request: isError=%v hits=%d", res.IsError, hits.Load())
	}
	// Controls: the same delete without it, and a read with an unknown
	// filter, do reach the server, so the refusal above is not vacuous.
	if res := call("DELETE", map[string]any{"customerId": "1", "cartId": "C1"}); res.IsError || hits.Load() != 1 {
		t.Fatalf("declared delete should be sent: isError=%v hits=%d", res.IsError, hits.Load())
	}
	if res := call("GET", map[string]any{"customerId": "1", "cartId": "C1", "pageSize": 5}); res.IsError || hits.Load() != 2 {
		t.Fatalf("a read keeps the filter pass-through: isError=%v hits=%d", res.IsError, hits.Load())
	}
}
