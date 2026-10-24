// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/cipp/handfixes.json under the
// "mcp-execute-cipp-results-failure" entry.

package mcp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	mcplib "github.com/mark3labs/mcp-go/mcp"
)

func executeAgainst(t *testing.T, body string) *mcplib.CallToolResult {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	t.Setenv("CIPP_BASE_URL", srv.URL)
	t.Setenv("CIPP_API_KEY", "test-static-token")
	result, err := handleCodeOrchExecute(context.Background(), mcplib.CallToolRequest{Params: mcplib.CallToolParams{
		Arguments: map[string]any{
			"endpoint_id": "exec-offboard-user.create",
			"confirm":     true,
			"params":      map[string]any{"tenantFilter": "t.example", "user": []any{"u@t.example"}},
		},
	}})
	if err != nil {
		t.Fatalf("transport error: %v", err)
	}
	if result == nil {
		t.Fatal("nil result")
	}
	return result
}

func resultText(r *mcplib.CallToolResult) string {
	var b strings.Builder
	for _, c := range r.Content {
		if tc, ok := c.(mcplib.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String()
}

// TestExecuteReportsCippResultsFailure: CIPP answers a failed write with HTTP
// 200 and the failure inside Results; cipp_execute must return an error result.
func TestExecuteReportsCippResultsFailure(t *testing.T) {
	r := executeAgainst(t, `{"Results":["Failed to disable sign-in for u@t.example"]}`)
	if !r.IsError {
		t.Fatalf("a Results failure came back as success: %s", resultText(r))
	}
	if !strings.Contains(resultText(r), "Failed to disable sign-in") {
		t.Fatalf("the CIPP failure text was lost: %s", resultText(r))
	}
	// Codex round 3: an empty resultText must not hide a failure in message.
	if r := executeAgainst(t, `{"Results":[{"resultText":"","message":"Failed to disable sign-in"}]}`); !r.IsError {
		t.Fatalf("a failure in message behind an empty resultText came back as success: %s", resultText(r))
	}
	for _, msg := range []string{`"Access denied"`, `["Forbidden"]`, `["Insufficient privileges to complete the operation."]`, `["Request_ResourceNotFound"]`, `["Invalid user u@t.example"]`} {
		if r := executeAgainst(t, `{"Results":`+msg+`}`); !r.IsError {
			t.Fatalf("Results %s came back as success: %s", msg, resultText(r))
		}
	}
	ok := executeAgainst(t, `{"Results":["Successfully offboarded u@t.example"]}`)
	if ok.IsError {
		t.Fatalf("a successful write came back as an error: %s", resultText(ok))
	}
}
