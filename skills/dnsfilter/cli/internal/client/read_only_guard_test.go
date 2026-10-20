// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"context"
	"strings"
	"testing"

	"dnsfilter-pp-cli/internal/cliutil"
)

// TestClientReadOnlyGuard guards handfixes.json: client-read-only-guard.
// Under DNSFILTER_MCP_READ_ONLY the companion CLI the MCP server runs must
// never send a write or a credential read, however the path was built.
func TestClientReadOnlyGuard(t *testing.T) {
	t.Setenv(cliutil.MCPReadOnlyEnvVar, "1")
	c, rec := newClientWithRecorder(t)
	ctx := context.Background()

	refused := []struct{ method, path string }{
		{"GET", "/v1/user_agents/uninstall_pin"},
		{"GET", "/v1/user_agents/UNINSTALL_PIN/"},
		{"GET", "/v1/user_agents/uninstall%5Fpin"},
		{"GET", "/v1/user_agents/uninstall%255Fpin"},
		{"GET", "/v1/user_agents/uninstall-pin.json"},
		{"GET", "/v1/users/..%2Fapi_keys"},
		{"GET", "/v1/api_keys/4?x=1"},
		{"GET", "/v1/psa_integrations/redirect_link"},
		{"POST", "/v1/networks"},
		{"DELETE", "/v1/policies/3"},
	}
	for _, r := range refused {
		_, _, err := c.do(ctx, r.method, r.path, nil, nil, nil)
		if err == nil || !strings.Contains(err.Error(), "read-only mode") {
			t.Errorf("%s %s was not refused: %v", r.method, r.path, err)
		}
	}
	if rec.calls != 0 {
		t.Fatalf("%d refused requests reached the transport", rec.calls)
	}
	allowed := []string{"/v1/organizations", "/v1/notes/policies/7/secret-key.io", "/v1/domains/api-keys.example.com"}
	for _, p := range allowed {
		if _, _, err := c.do(ctx, "GET", p, nil, nil, nil); err != nil {
			t.Fatalf("ordinary GET %s refused: %v", p, err)
		}
	}
	if rec.calls != len(allowed) {
		t.Fatalf("ordinary GETs did not reach the transport (calls=%d)", rec.calls)
	}

	t.Setenv(cliutil.MCPReadOnlyEnvVar, "")
	if _, _, err := c.do(ctx, "POST", "/v1/networks", nil, map[string]any{}, nil); err != nil {
		t.Fatalf("POST refused with the switch off: %v", err)
	}
}
