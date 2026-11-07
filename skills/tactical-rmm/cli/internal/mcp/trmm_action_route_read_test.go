// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written (NOT generated): tactical-rmm handfix action-gets-need-confirm.

package mcp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// Codex review r2: an encoded "/" in a read's path parameter must not reach an
// agent-dispatching route (vendor-src/agents/urls.py "<agent>/ping/").
func TestCodeOrchReadCannotReachAnActionThroughAnEncodedSlash(t *testing.T) {
	actions := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(strings.ToLower(r.URL.EscapedPath()), "ping") {
			actions++
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	resetMCPPathEnv(t)
	t.Setenv("TACTICAL_RMM_BASE_URL", srv.URL)
	t.Setenv("TRMM_API_KEY", "test-key")
	s := server.NewMCPServer("t", "1")
	RegisterTools(s)
	read, ok := s.ListTools()["tactical-rmm_read"]
	if !ok {
		t.Fatal("tactical-rmm_read not registered")
	}
	req := mcplib.CallToolRequest{}
	req.Params.Name = "tactical-rmm_read"
	req.Params.Arguments = map[string]any{"endpoint_id": "agents.agents-agentid-2", "params": map[string]any{"agent_id": "kbmLWvmXfuIjpzSGeUlAXNKwlBCMGJmMKTjrYJsh/ping"}}
	if _, err := read.Handler(context.Background(), req); err != nil {
		t.Logf("handler error (fine): %v", err)
	}
	if actions != 0 {
		t.Fatalf("a read reached the ping action %d time(s)", actions)
	}
}
