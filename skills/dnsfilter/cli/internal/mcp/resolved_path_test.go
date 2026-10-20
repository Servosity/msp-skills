// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/dnsfilter/handfixes.json
// under the "mcp-read-only-resolved-path-refusal" entry.

package mcp

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"dnsfilter-pp-cli/internal/cliutil"
	mcplib "github.com/mark3labs/mcp-go/mcp"
)

// TestReadOnlyExecuteRefusesResolvedCredentialPaths is adversarial round 2's
// P1: user-agents.show {"id":"uninstall_pin"} passed the template check
// (/v1/user_agents/{id}) and sent GET /v1/user_agents/uninstall_pin, whose
// answer is the PIN as a bare JSON string no key-name redaction can see.
// Every spelling is refused on the resolved path before any request; with the
// switch off the same call reaches the mock and returns the PIN.
func TestReadOnlyExecuteRefusesResolvedCredentialPaths(t *testing.T) {
	var mu sync.Mutex
	var hits []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.ReadAll(r.Body)
		mu.Lock()
		hits = append(hits, r.Method+" "+r.URL.EscapedPath())
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(strings.ToLower(r.URL.Path), "uninstall_pin") {
			_, _ = io.WriteString(w, `"493817"`)
			return
		}
		_, _ = io.WriteString(w, `{"data":{"id":"1","type":"x","attributes":{"name":"ok"}}}`)
	}))
	t.Cleanup(srv.Close)
	setIsolatedHome(t, srv.URL)

	variants := []struct {
		id     string
		params map[string]any
	}{
		{"user-agents.show", map[string]any{"id": "uninstall_pin", "organization_id": 10}},
		{"user-agents.show", map[string]any{"id": "UNINSTALL_PIN"}},
		{"user-agents.show", map[string]any{"id": "uninstall_pin/"}},
		{"user-agents.show", map[string]any{"id": "uninstall%5Fpin"}},
		{"user-agents.show", map[string]any{"id": "uninstall_pin.json"}},
		{"user-agents.show", map[string]any{"id": "../uninstall_pin"}},
		{"users.show", map[string]any{"id": "../api_keys"}},
		{"users.show", map[string]any{"id": "../API-KEYS/1"}},
		{"networks.show", map[string]any{"id": "7/secret_key"}},
		{"organizations.show", map[string]any{"id": "../psa_integrations/redirect_link"}},
	}

	t.Setenv(cliutil.MCPReadOnlyEnvVar, "1")
	s := newRegisteredServer(t)
	for _, v := range variants {
		frame := callToolWire(t, s, "dnsfilter_execute", map[string]any{"endpoint_id": v.id, "params": v.params})
		if strings.Contains(frame, "493817") || !strings.Contains(frame, "refusing GET "+v.id) {
			t.Fatalf("%s %v was not refused on its resolved path: %s", v.id, v.params, frame)
		}
	}
	mu.Lock()
	sent := append([]string{}, hits...)
	mu.Unlock()
	if len(sent) != 0 {
		t.Fatalf("refused credential reads reached the API: %v", sent)
	}
	// A plain show still runs under read-only.
	if frame := callToolWire(t, s, "dnsfilter_execute", map[string]any{"endpoint_id": "user-agents.show", "params": map[string]any{"id": "12345"}}); strings.Contains(frame, "refusing") {
		t.Fatalf("a plain show was refused: %s", frame)
	}

	t.Setenv(cliutil.MCPReadOnlyEnvVar, "")
	s = newRegisteredServer(t)
	frame := callToolWire(t, s, "dnsfilter_execute", map[string]any{"endpoint_id": "user-agents.show", "params": map[string]any{"id": "uninstall_pin", "organization_id": 10}})
	if !strings.Contains(frame, "493817") {
		t.Fatalf("with the switch off the receipt call should reach the mock and return the PIN: %s", frame)
	}
}

// TestReadOnlyIntentRefusesCredentialPathArg: the recipe intent's argument
// gets the same credential-path check as every mirror argument.
func TestReadOnlyIntentRefusesCredentialPathArg(t *testing.T) {
	t.Setenv(cliutil.MCPReadOnlyEnvVar, "1")
	savedPath, savedErr := recipeCLIPath, recipeCLIPathErr
	t.Cleanup(func() { recipeCLIPath, recipeCLIPathErr = savedPath, savedErr })
	recipeCLIPath, recipeCLIPathErr = "/nonexistent/dnsfilter-cli", nil
	res, err := handlePrecedentCheckBeforeAllowingADomain(context.Background(), mcplib.CallToolRequest{Params: mcplib.CallToolParams{Arguments: map[string]any{"domain": "UNINSTALL%5FPIN"}}})
	if err != nil || res == nil || !res.IsError {
		t.Fatalf("intent with a credential-path argument was not refused: %+v %v", res, err)
	}
	if text := res.Content[0].(mcplib.TextContent).Text; !strings.Contains(text, cliutil.MCPReadOnlyEnvVar) {
		t.Fatalf("unexpected refusal text: %s", text)
	}
}
