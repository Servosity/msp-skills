// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/saas-alerts/handfixes.json
// under the "test-event-delivery-failure-surfaced" entry.

package mcp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	"saas-alerts-pp-cli/internal/cliutil/testenv"
)

// TestCodeOrchExecuteTestEventDeliveryErrorIsAnError: the MCP twin of
// `subscriptions test-event` returns an error result, not a success, when
// sendTestEvent answers 200 with a delivery-error list.
func TestCodeOrchExecuteTestEventDeliveryErrorIsAnError(t *testing.T) {
	answer := `true`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(answer))
	}))
	defer srv.Close()
	testenv.Isolate(t)
	t.Setenv("SAAS_ALERTS_BASE_URL", srv.URL)
	t.Setenv("SAAS_ALERTS_API_KEY", "test-key")
	t.Setenv("PRINTING_PRESS_VERIFY", "")
	t.Setenv("PRINTING_PRESS_DOGFOOD", "")
	call := func() *mcplib.CallToolResult {
		t.Helper()
		res, err := handleCodeOrchExecute(context.Background(), mcplib.CallToolRequest{Params: mcplib.CallToolParams{
			Arguments: map[string]any{"endpoint_id": "subscriptions.test-event"},
		}})
		if err != nil || res == nil {
			t.Fatalf("execute: res=%v err=%v", res, err)
		}
		return res
	}
	if res := call(); res.IsError {
		t.Fatalf("a delivered test event (true) is an error: %+v", res.Content)
	}
	for _, answer = range []string{`[{"channelId":"CH_TEST","message":"delivery failed"}]`, `"Success: false"`} {
		if res := call(); !res.IsError {
			t.Fatalf("%s came back as success: %+v", answer, res.Content)
		}
	}
}
