// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package mcp

// Hand-fix (handfixes.json mcp-read-only-switch): tools/maintainer/
// replay_handfixes.py writes this file from
// handfix_replay/mcp_read_only_execute_test.go.tmpl.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	mcplib "github.com/mark3labs/mcp-go/mcp"
)

// mcpReadOnlyResultText joins the text content of a tool result.
func mcpReadOnlyResultText(res *mcplib.CallToolResult) string {
	var b strings.Builder
	for _, c := range res.Content {
		switch tc := c.(type) {
		case mcplib.TextContent:
			b.WriteString(tc.Text)
		case *mcplib.TextContent:
			b.WriteString(tc.Text)
		}
	}
	return b.String()
}

// The API execute tool is not listed while the switch is on, and its handler
// refuses every non-GET endpoint before it builds a client, so nothing reaches
// the API. The base and token URLs point at a recorder: a request that slipped
// through is counted there and never reaches a vendor.
func TestCodeOrchExecuteRefusesNonGETUnderMCPReadOnly(t *testing.T) {
	var hits atomic.Int32
	rec := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":1,"access_token":"read-only-test","token_type":"Bearer","expires_in":3600}`))
	}))
	defer rec.Close()
	home := t.TempDir()
	for _, k := range []string{"HOME", "USERPROFILE", "APPDATA", "LOCALAPPDATA", "XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_CACHE_HOME", "XDG_STATE_HOME"} {
		t.Setenv(k, home)
	}
	t.Setenv("SYNCRO_BASE_URL", rec.URL)
	t.Setenv("SYNCRO_TOKEN_URL", rec.URL+"/token")
	t.Setenv(mcpReadOnlySwitchEnvVar, "1")

	tried := map[string]bool{}
	for i := range codeOrchEndpoints {
		ep := &codeOrchEndpoints[i]
		if strings.EqualFold(strings.TrimSpace(ep.Method), "GET") || tried[ep.Method] {
			continue
		}
		tried[ep.Method] = true
		params := map[string]any{"probe_marker": "read-only-test"}
		for _, name := range ep.Positional {
			params[name] = "1"
		}
		var req mcplib.CallToolRequest
		req.Params.Arguments = map[string]any{"endpoint_id": ep.ID, "params": params}
		res, err := handleCodeOrchExecute(context.Background(), req)
		if err != nil {
			t.Fatalf("%s %s: %v", ep.Method, ep.ID, err)
		}
		want := "refusing " + ep.Method + " " + ep.ID
		if res == nil || !res.IsError || !strings.Contains(mcpReadOnlyResultText(res), want) {
			text := ""
			if res != nil {
				text = mcpReadOnlyResultText(res)
			}
			t.Errorf("%s=1: execute %s %s was not refused (want %q), got %q", mcpReadOnlySwitchEnvVar, ep.Method, ep.ID, want, text)
		}
	}
	if len(tried) == 0 {
		t.Fatal("the endpoint registry has no non-GET endpoint, so this test proves nothing")
	}
	if n := hits.Load(); n != 0 {
		t.Errorf("%s=1: %d request(s) reached the API through execute", mcpReadOnlySwitchEnvVar, n)
	}
}
