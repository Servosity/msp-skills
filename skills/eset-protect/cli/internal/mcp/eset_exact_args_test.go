// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package mcp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	mcplib "github.com/mark3labs/mcp-go/mcp"
)

// TestExecuteKeepsBigIntegers (handfixes.json: json-numbers-exact): mcp-go
// decodes tool arguments with a plain json.Unmarshal, so 2^53+1 inside an
// eset-protect_execute body reached ESET as 2^53. ExactArguments (wired in
// cmd/eset-protect-mcp/main.go) re-decodes the wire arguments with
// json.Number; the bare handler is the control that shows the rounding.
func TestExecuteKeepsBigIntegers(t *testing.T) {
	const big = "9007199254740993"
	var mu sync.Mutex
	var bodies []string
	esetFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		bodies = append(bodies, string(b))
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"task":{"uuid":"00000000-0000-0000-0000-000000000001"}}`)
	})
	wire := `{"name":"eset-protect_execute","arguments":{"endpoint_id":"device-tasks.create","params":{"task":{"action":{"name":"OnDemandScan","params":{"limit":` + big + `}}}}}}`
	call := func(h func(context.Context, mcplib.CallToolRequest) (*mcplib.CallToolResult, error)) string {
		t.Helper()
		var req mcplib.CallToolRequest
		if err := json.Unmarshal([]byte(wire), &req.Params); err != nil {
			t.Fatal(err)
		}
		res, err := h(context.Background(), req)
		if err != nil || res == nil || res.IsError {
			t.Fatalf("execute: %v %s", err, toolText(t, res))
		}
		mu.Lock()
		defer mu.Unlock()
		return bodies[len(bodies)-1]
	}
	if got := call(ExactArguments(handleCodeOrchExecute)); !strings.Contains(got, big) {
		t.Fatalf("execute changed the number before sending: %s", got)
	}
	if got := call(handleCodeOrchExecute); strings.Contains(got, big) {
		t.Fatalf("control: the bare handler no longer rounds, so this test proves nothing: %s", got)
	}
}

// TestExactArgumentsKeepsNonIntegerNumbersAsFloat: integral floats (7.0, 1e1)
// sent by clients that map JSON Schema "number" to float must keep working;
// json.Number would have printed "7.0", which integer flags and int32 query
// params reject.
func TestExactArgumentsKeepsNonIntegerNumbersAsFloat(t *testing.T) {
	var seen map[string]any
	h := ExactArguments(func(_ context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		seen = req.GetArguments()
		return mcplib.NewToolResultText("ok"), nil
	})
	var req mcplib.CallToolRequest
	if err := json.Unmarshal([]byte(`{"name":"x","arguments":{"days":7.0,"limit":1e1,"ratio":1.5,"neg":-5,"big":9007199254740993,"nested":{"n":10.0,"ids":[9007199254740993,2.0]}}}`), &req.Params); err != nil {
		t.Fatal(err)
	}
	if _, err := h(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"days": float64(7), "limit": float64(10), "ratio": 1.5, "neg": json.Number("-5"), "big": json.Number("9007199254740993")}
	for k, v := range want {
		if seen[k] != v {
			t.Errorf("%s = %#v, want %#v", k, seen[k], v)
		}
	}
	nested := seen["nested"].(map[string]any)
	ids := nested["ids"].([]any)
	if nested["n"] != float64(10) || ids[0] != json.Number("9007199254740993") || ids[1] != float64(2) {
		t.Errorf("nested = %#v", nested)
	}
}
