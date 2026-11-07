// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored (handfixes.json: ion-mcp-write-confirm).

package mcp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func confirmTestServer(t *testing.T) (*server.MCPServer, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(fake.Close)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", home)
	t.Setenv("TD_SYNNEX_CONFIG", home+"/config.toml")
	t.Setenv("TD_SYNNEX_BASE_URL", fake.URL)
	t.Setenv("TD_SYNNEX_ACCESS_TOKEN", "ion-test-token")
	t.Setenv("TD_SYNNEX_ACCOUNT_ID", "10505")
	t.Setenv("TD_SYNNEX_MCP_READ_ONLY", "")
	s := server.NewMCPServer("confirm", "test")
	RegisterTools(s)
	return s, &hits
}

func callTool(t *testing.T, s *server.MCPServer, name string, args map[string]any) *mcplib.CallToolResult {
	t.Helper()
	entry := s.GetTool(name)
	if entry == nil {
		t.Fatalf("tool %s not registered", name)
	}
	req := mcplib.CallToolRequest{}
	req.Params.Name = name
	req.Params.Arguments = args
	res, err := entry.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return res
}

func resultText(res *mcplib.CallToolResult) string {
	var b strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(mcplib.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String()
}

// TestEndpointWritesPreviewUntilConfirmTrue: the endpoint tools that place,
// cancel or check out orders and delete carts send nothing on the first call,
// nor with confirm set to anything but boolean true.
func TestEndpointWritesPreviewUntilConfirmTrue(t *testing.T) {
	s, hits := confirmTestServer(t)
	cases := map[string]map[string]any{
		"customer_orders_cancel-order": {"accountId": "10505", "customerId": "108536", "orderId": "O1"},
		"customer_orders_create-order": {"accountId": "10505", "customerId": "108536", "name": "o", "referenceId": "r"},
		"carts_checkout":               {"customerId": "108536", "cartId": "C1"},
		"carts_delete":                 {"customerId": "108536", "cartId": "C1"},
		"products_disable":             {"accountId": "10505", "productId": "P1"},
	}
	for name, args := range cases {
		for _, confirm := range []any{nil, "true", 1.0, false} {
			a := map[string]any{}
			for k, v := range args {
				a[k] = v
			}
			if confirm != nil {
				a["confirm"] = confirm
			}
			res := callTool(t, s, name, a)
			if got := hits.Load(); got != 0 {
				t.Fatalf("%s confirm=%v sent %d request(s) before confirm:true", name, confirm, got)
			}
			if txt := resultText(res); !strings.Contains(txt, "Preview only: nothing was sent to StreamOne ION") {
				t.Fatalf("%s confirm=%v: want a preview, got %s", name, confirm, txt)
			}
		}
		a := map[string]any{"confirm": true}
		for k, v := range args {
			a[k] = v
		}
		res := callTool(t, s, name, a)
		if got := hits.Load(); got != 1 {
			t.Fatalf("%s confirm:true sent %d request(s), want 1: %s", name, got, resultText(res))
		}
		hits.Store(0)
	}
}

// TestEveryWriteToolIsGatedOrLocal fails closed: a tool that is not read-only
// must carry a boolean confirm argument, or be a mirrored command that changes
// only local state.
func TestEveryWriteToolIsGatedOrLocal(t *testing.T) {
	s, _ := confirmTestServer(t)
	for name, entry := range s.ListTools() {
		tool := entry.Tool
		if ro := tool.Annotations.ReadOnlyHint; ro != nil && *ro {
			continue
		}
		if localOnlyMirroredTools[name] && isMirroredTool(tool) {
			continue
		}
		prop, ok := tool.InputSchema.Properties["confirm"].(map[string]any)
		if !ok || prop["type"] != "boolean" {
			t.Errorf("write tool %s has no boolean confirm gate", name)
		}
	}
}

// TestMirroredConfirmIsBooleanOnly: subscriptions set previews until its own
// --confirm; the gate passes confirm to the CLI only when it is boolean true.
func TestMirroredConfirmIsBooleanOnly(t *testing.T) {
	var got map[string]any
	h := booleanConfirmOnly(func(_ context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		got = req.GetArguments()
		return mcplib.NewToolResultText("ok"), nil
	})
	for _, v := range []any{"true", "yes", 1.0, false, []any{true}} {
		req := mcplib.CallToolRequest{}
		req.Params.Arguments = map[string]any{"confirm": v, "seats": 12.0}
		if _, err := h(context.Background(), req); err != nil {
			t.Fatal(err)
		}
		if _, ok := got["confirm"]; ok {
			t.Fatalf("confirm=%#v reached the CLI", v)
		}
		if got["seats"] != 12.0 {
			t.Fatalf("other arguments dropped: %v", got)
		}
	}
	req := mcplib.CallToolRequest{}
	req.Params.Arguments = map[string]any{"confirm": true}
	if _, err := h(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if got["confirm"] != true {
		t.Fatalf("confirm:true did not reach the CLI: %v", got)
	}
}

// TestSubscriptionsSetKeepsItsOwnConfirm: the typed seat/status write keeps a
// boolean confirm argument (its CLI --confirm), which the boolean-only gate guards.
func TestSubscriptionsSetKeepsItsOwnConfirm(t *testing.T) {
	s, _ := confirmTestServer(t)
	entry := s.GetTool("subscriptions_set")
	if entry == nil {
		t.Fatal("subscriptions_set not registered")
	}
	if p, ok := entry.Tool.InputSchema.Properties["confirm"].(map[string]any); !ok || p["type"] != "boolean" {
		t.Fatal("subscriptions_set: confirm is not a boolean argument")
	}
}
