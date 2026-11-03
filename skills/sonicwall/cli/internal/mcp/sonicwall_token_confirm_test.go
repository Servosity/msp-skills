// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package mcp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	mcplib "github.com/mark3labs/mcp-go/mcp"
)

// Every token exchange needs confirm, and sonicwall_read refuses to mint
// (handfix token-exchange-needs-confirm).
func TestTokenExchangesNeedConfirm(t *testing.T) {
	seen := map[string]bool{}
	for _, ep := range codeOrchEndpoints {
		if isTokenExchangePath(ep.Path) {
			seen[ep.ID] = true
			if !ep.NeedsConfirm {
				t.Errorf("%s (%s) mints a credential without confirm", ep.ID, ep.Path)
			}
		}
	}
	for _, id := range []string{"nsm_session.sso", "nsm_session.refresh", "msw_tenants.access-code"} {
		if !seen[id] {
			t.Errorf("%s not recognised as a token exchange", id)
		}
	}
	// Hermetic: a local server stands in for NSM, so a broken gate fails here instead of reaching the vendor.
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		_, _ = w.Write([]byte(`{"status":{"success":true,"info":[{"message":"minted"}]}}`))
	}))
	defer srv.Close()
	t.Setenv("SONICWALL_BASE_URL", srv.URL)
	t.Setenv("SONICWALL_NSM_TOKEN", strings.Repeat("x", 40))
	req := mcplib.CallToolRequest{}
	req.Params.Arguments = map[string]any{"endpoint_id": "nsm_session.sso", "params": map[string]any{"tenantserial": "S", "code": "C", "client": "nsm-api"}}
	res, err := handleCodeOrchRead(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	text := ""
	for _, c := range res.Content {
		if tc, ok := c.(mcplib.TextContent); ok {
			text += tc.Text
		}
	}
	if !res.IsError || !strings.Contains(text, "changes state") || hits.Load() != 0 {
		t.Fatalf("sonicwall_read must refuse the token mint before any request (requests=%d), got %q", hits.Load(), text)
	}
}
