// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated), pinned by skills/sentinelone/handfixes.json
// under "mcp-non-get-endpoints-need-confirm" (TestEveryNonGETEndpointNeedsConfirm)
// and the secret-persistence entry (TestSecretEndpointsNeedConfirm).

package mcp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	mcplib "github.com/mark3labs/mcp-go/mcp"

	"sentinelone-pp-cli/internal/cliutil"
	"sentinelone-pp-cli/internal/cliutil/testenv"
)

// TestEveryNonGETEndpointNeedsConfirm: an MCP caller reaches any endpoint
// through sentinelone_read or sentinelone_execute, and only NeedsConfirm makes
// the server ask before sending. A POST that asks agents to upload files or
// logs from customer endpoints is not a read, whatever its name says, so every
// method other than GET or HEAD must need confirmation and must be refused by
// the read-only switch.
func TestEveryNonGETEndpointNeedsConfirm(t *testing.T) {
	var bad []string
	for i := range codeOrchEndpoints {
		ep := &codeOrchEndpoints[i]
		switch strings.ToUpper(strings.TrimSpace(ep.Method)) {
		case "GET", "HEAD":
			continue
		}
		if !ep.NeedsConfirm {
			bad = append(bad, ep.Method+" "+ep.ID)
		}
	}
	if len(bad) > 0 {
		t.Fatalf("%d non-GET endpoints run without confirm:\n%s", len(bad), strings.Join(bad, "\n"))
	}
}

// TestSecretEndpointsNeedConfirm: a GET whose answer is a credential (the
// agent passphrases, the account uninstall password, a login-by-token
// session, the Ranger credential details) must need confirm, so
// sentinelone_read refuses it, sentinelone_execute previews it until confirm
// is boolean true, and SENTINELONE_MCP_READ_ONLY=1 refuses it outright. The
// set is derived from the same path rule sync and the store use
// (cliutil.IsSecretBearingPath) and must still cover the four endpoints.
func TestSecretEndpointsNeedConfirm(t *testing.T) {
	want := map[string]bool{
		"agents.get-passphrase":           true,
		"accounts.uninstall-password.get": true,
		"users.login-by-token":            true,
		"ranger.get-cred-group-details":   true,
	}
	derived := map[string]bool{}
	var bad []string
	for i := range codeOrchEndpoints {
		ep := &codeOrchEndpoints[i]
		if !cliutil.IsSecretBearingPath(ep.Path) {
			continue
		}
		derived[ep.ID] = true
		if !ep.NeedsConfirm {
			bad = append(bad, ep.Method+" "+ep.ID+" "+ep.Path)
		}
	}
	for id := range want {
		if !derived[id] {
			t.Errorf("the shared secret-path rule no longer covers %s", id)
		}
	}
	if len(bad) > 0 {
		t.Fatalf("%d secret-returning endpoints run without confirm:\n%s", len(bad), strings.Join(bad, "\n"))
	}

	// Drive the tools: no request may reach the API without confirm.
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"11","passphrase":"MCP CONFIRM SENTINEL"}]}`))
	}))
	defer srv.Close()
	testenv.Isolate(t)
	t.Setenv("SENTINELONE_BASE_URL", srv.URL)
	t.Setenv("SENTINELONE_API_TOKEN", "mcp-confirm-test-token")
	call := func(h func(context.Context, mcplib.CallToolRequest) (*mcplib.CallToolResult, error), args map[string]any) (*mcplib.CallToolResult, string) {
		t.Helper()
		res, err := h(context.Background(), mcplib.CallToolRequest{Params: mcplib.CallToolParams{Arguments: args}})
		if err != nil {
			t.Fatalf("transport error: %v", err)
		}
		text := ""
		for _, c := range res.Content {
			if tc, ok := c.(mcplib.TextContent); ok {
				text += tc.Text
			}
		}
		return res, text
	}
	for id := range want {
		t.Setenv(cliutil.MCPReadOnlyEnvVar, "")
		res, text := call(handleCodeOrchRead, map[string]any{"endpoint_id": id, "params": map[string]any{"account_id": "123"}})
		if !res.IsError || strings.Contains(text, "MCP CONFIRM SENTINEL") {
			t.Errorf("sentinelone_read %s answered without confirm: %s", id, text)
		}
		res, text = call(handleCodeOrchExecute, map[string]any{"endpoint_id": id, "params": map[string]any{"account_id": "123"}})
		if res.IsError || !strings.Contains(text, "confirmation_required") || strings.Contains(text, "MCP CONFIRM SENTINEL") {
			t.Errorf("sentinelone_execute %s without confirm did not preview: %s", id, text)
		}
		t.Setenv(cliutil.MCPReadOnlyEnvVar, "1")
		for _, h := range []func(context.Context, mcplib.CallToolRequest) (*mcplib.CallToolResult, error){handleCodeOrchRead, handleCodeOrchExecute} {
			res, text = call(h, map[string]any{"endpoint_id": id, "params": map[string]any{"account_id": "123"}, "confirm": true})
			if !res.IsError || !strings.Contains(text, cliutil.MCPReadOnlyEnvVar) {
				t.Errorf("%s under %s=1 was not refused: %s", id, cliutil.MCPReadOnlyEnvVar, text)
			}
		}
	}
	if hits != 0 {
		t.Fatalf("%d request(s) reached the API without confirm", hits)
	}
}
