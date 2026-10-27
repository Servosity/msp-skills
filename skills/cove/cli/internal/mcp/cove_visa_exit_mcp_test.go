// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated), cove only: the stop-report inputs of the
// press 4.34.0 reprint review, through the MCP server_info tool. Hand-fix
// code: cove-short-credential-funnel.
package mcp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/server"

	"cove-pp-cli/internal/cliutil"
	"cove-pp-cli/internal/cliutil/testenv"
)

func coveMCPAnswering(t *testing.T, status int, headers map[string]string, body string) *server.MCPServer {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for k, v := range headers {
			w.Header().Set(k, v)
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	t.Setenv("COVE_BASE_URL", srv.URL)
	testenv.Isolate(t, cliutil.ConfigDir, cliutil.DataDir, cliutil.StateDir, cliutil.CacheDir)
	s := server.NewMCPServer("cove-visa-exit", "test")
	RegisterTools(s)
	return s
}

func coveMCPShown(t *testing.T, s *server.MCPServer, args map[string]any) string {
	t.Helper()
	result := callEndpointTool(t, s, "server_info", args)
	b, _ := json.Marshal(result)
	return string(b)
}

// Codex round 2 finding 1: the sent visa in a Content-Type parameter reached
// a successful server_info result.
func TestMCPServerInfoNeverQuotesTheContentType(t *testing.T) {
	const sent = "sent-visa-mcp-0123456789"
	s := coveMCPAnswering(t, 200, map[string]string{"Content-Type": "application/octet-stream; name=" + sent}, "ok")
	if shown := coveMCPShown(t, s, map[string]any{"visa": sent}); strings.Contains(shown, sent) || strings.Contains(shown, "c2VudC12aXNh") {
		t.Fatalf("the sent visa reached the MCP result: %s", shown)
	}
}

// Round 2 adversarial P1 through MCP: a text answer naming the visa after
// another name.
func TestMCPServerInfoWithholdsAVisaAfterAnotherName(t *testing.T) {
	const fresh = "FRESHVISAmcp8W3e5R7t9Y1u2I4K"
	for _, a := range []struct {
		status int
		body   string
	}{{400, "error: visa=" + fresh}, {200, "result: visa=" + fresh}} {
		s := coveMCPAnswering(t, a.status, map[string]string{"Content-Type": "text/plain"}, a.body)
		if shown := coveMCPShown(t, s, map[string]any{"visa": "sent-visa-0123456789"}); strings.Contains(shown, fresh) {
			t.Fatalf("HTTP %d %q: the visa reached the MCP result: %s", a.status, a.body, shown)
		}
	}
}
