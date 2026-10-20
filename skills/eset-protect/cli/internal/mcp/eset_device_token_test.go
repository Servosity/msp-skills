// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"eset-protect-pp-cli/internal/cliutil"
	"eset-protect-pp-cli/internal/cliutil/testenv"
	mcplib "github.com/mark3labs/mcp-go/mcp"
)

const mcpDeviceTokenSentinel = "XX-DEVICE-TOKEN-SENTINEL-XX"

// esetFakeServer stands in for ESET Connect; ESET_PROTECT_BASE_URL rebases
// the absolute *.eset.systems hosts onto it.
func esetFakeServer(t *testing.T, h http.HandlerFunc) {
	t.Helper()
	testenv.Isolate(t)
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	t.Setenv("ESET_PROTECT_BASE_URL", srv.URL)
	t.Setenv("ESET_PROTECT_TOKEN", "test-token")
	t.Setenv("ESET_PROTECT_REGION", "eu")
	t.Setenv("ESET_PROTECT_USERNAME", "")
	t.Setenv("ESET_PROTECT_PASSWORD", "")
	t.Setenv(cliutil.VerifyEnvVar, "")
	t.Setenv(cliutil.DogfoodEnvVar, "")
}

func toolText(t *testing.T, r *mcplib.CallToolResult) string {
	t.Helper()
	var b strings.Builder
	for _, c := range r.Content {
		if tc, ok := c.(mcplib.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String()
}

// eset-protect_execute must return device rows with deviceToken redacted
// (handfixes.json: device-token-redacted-at-decode).
func TestCodeOrchExecuteRedactsDeviceToken(t *testing.T) {
	esetFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		dev := map[string]any{"uuid": "00000000-0000-0000-0000-000000000001", "displayName": "FRONTDESK-01", "deviceToken": mcpDeviceTokenSentinel}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/devices/00000000-0000-0000-0000-000000000001":
			_ = json.NewEncoder(w).Encode(map[string]any{"device": dev})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{"devices": []any{dev}, "nextPageToken": ""})
		}
	})
	for _, call := range []map[string]any{
		{"endpoint_id": "devices.list"},
		{"endpoint_id": "devices.get", "params": map[string]any{"deviceUuid": "00000000-0000-0000-0000-000000000001"}},
		{"endpoint_id": "devices-batch-get.devices", "params": map[string]any{"devicesUuids": []any{"00000000-0000-0000-0000-000000000001"}}},
		{"endpoint_id": "device-groups.devices.groups-list-member", "params": map[string]any{"groupUuid": "00000000-0000-0000-0000-0000000000a1"}},
	} {
		res, err := handleCodeOrchExecute(context.Background(), mcplib.CallToolRequest{Params: mcplib.CallToolParams{Arguments: call}})
		if err != nil {
			t.Fatalf("%v: %v", call, err)
		}
		text := toolText(t, res)
		if res.IsError || !strings.Contains(text, "FRONTDESK-01") {
			t.Fatalf("%v: no device came back: %s", call, text)
		}
		if strings.Contains(text, mcpDeviceTokenSentinel) {
			t.Fatalf("%v: execute returned the device token: %s", call, text)
		}
	}
}
