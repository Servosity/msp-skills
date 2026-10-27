// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Codex review round 1 (press 4.34.0 reprint). P1: the typed endpoint tools
// accepted a "method" override, so devices_get could dispatch RemoveAccount
// while advertising read-only behaviour (also under COVE_MCP_READ_ONLY=1). P2:
// they sent neither jsonrpc, method nor id unless the agent passed them,
// although each description advertises those defaults. Hand-fix code:
// cove-rpc-read-method-guard.
package mcp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sort"
	"sync"
	"testing"

	"cove-pp-cli/internal/cliutil"
	"cove-pp-cli/internal/cliutil/testenv"
	"cove-pp-cli/internal/coverpc"
	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

type mcpRPCRecorder struct {
	mu     sync.Mutex
	bodies []map[string]any
}

func (r *mcpRPCRecorder) requests() []map[string]any {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]map[string]any(nil), r.bodies...)
}

func (r *mcpRPCRecorder) reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.bodies = nil
}

func newMCPRPCRecorder(t *testing.T) *mcpRPCRecorder {
	t.Helper()
	rec := &mcpRPCRecorder{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		raw, _ := io.ReadAll(req.Body)
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		if body == nil {
			body = map[string]any{"_raw": string(raw)}
		}
		rec.mu.Lock()
		rec.bodies = append(rec.bodies, body)
		rec.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":1,"jsonrpc":"2.0","result":{"result":{"Id":42}}}`)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("COVE_BASE_URL", srv.URL)
	testenv.Isolate(t, cliutil.ConfigDir, cliutil.DataDir, cliutil.StateDir, cliutil.CacheDir)
	return rec
}

func callEndpointTool(t *testing.T, s *server.MCPServer, name string, args map[string]any) *mcplib.CallToolResult {
	t.Helper()
	entry := s.GetTool(name)
	if entry == nil {
		t.Fatalf("tool %s is not registered", name)
	}
	result, err := entry.Handler(context.Background(), mcplib.CallToolRequest{
		Params: mcplib.CallToolParams{Name: name, Arguments: args},
	})
	if err != nil {
		t.Fatalf("%s: protocol error: %v", name, err)
	}
	if result == nil {
		t.Fatalf("%s: nil result", name)
	}
	return result
}

func TestMCPEndpointToolRefusesAWriteMethod(t *testing.T) {
	for _, readOnly := range []string{"", "1"} {
		t.Run("COVE_MCP_READ_ONLY="+readOnly, func(t *testing.T) {
			rec := newMCPRPCRecorder(t)
			t.Setenv("COVE_MCP_READ_ONLY", readOnly)
			s := server.NewMCPServer("cove-rpc-guard", "test")
			RegisterTools(s)
			for _, method := range []string{"RemoveAccount", " GetAccountInfoById", "GetPartnerInfoById", "removeAccount"} {
				result := callEndpointTool(t, s, "devices_get", map[string]any{"visa": "visa-0123456789", "params-account-id": 42, "method": method})
				if !result.IsError {
					t.Fatalf("devices_get with method %q succeeded", method)
				}
			}
			if reqs := rec.requests(); len(reqs) != 0 {
				t.Fatalf("devices_get sent %d request(s) for another method: %v", len(reqs), reqs)
			}
		})
	}
}

func TestMCPEndpointToolSendsTheAdvertisedEnvelopeDefaults(t *testing.T) {
	rec := newMCPRPCRecorder(t)
	s := server.NewMCPServer("cove-rpc-guard", "test")
	RegisterTools(s)
	result := callEndpointTool(t, s, "devices_get", map[string]any{"visa": "visa-0123456789", "params-account-id": 42})
	if result.IsError {
		t.Fatalf("devices_get with the advertised defaults failed: %v", result.Content)
	}
	reqs := rec.requests()
	if len(reqs) != 1 {
		t.Fatalf("devices_get sent %d requests, want 1: %v", len(reqs), reqs)
	}
	got := reqs[0]
	if got["jsonrpc"] != "2.0" || got["method"] != "GetAccountInfoById" || got["id"] != float64(1) {
		t.Fatalf("devices_get envelope = %v, want jsonrpc 2.0, method GetAccountInfoById, id 1", got)
	}
	params, _ := got["params"].(map[string]any)
	if params["accountId"] != float64(42) {
		t.Fatalf("devices_get params = %v", got["params"])
	}

	// An agent that repeats the defaults explicitly (or sends an empty
	// method, as some hosts do for optional strings) gets the same request.
	rec.reset()
	result = callEndpointTool(t, s, "devices_get", map[string]any{"visa": "visa-0123456789", "params-account-id": 42, "method": "GetAccountInfoById", "jsonrpc": "2.0", "id": 7})
	if result.IsError {
		t.Fatalf("devices_get with its own method failed: %v", result.Content)
	}
	result = callEndpointTool(t, s, "devices_get", map[string]any{"visa": "visa-0123456789", "params-account-id": 42, "method": ""})
	if result.IsError {
		t.Fatalf("devices_get with an empty method failed: %v", result.Content)
	}
	reqs = rec.requests()
	if len(reqs) != 2 || reqs[0]["method"] != "GetAccountInfoById" || reqs[0]["id"] != float64(7) || reqs[1]["method"] != "GetAccountInfoById" {
		t.Fatalf("explicit-default requests = %v", reqs)
	}
}

// Every typed endpoint tool sends the JSON-RPC method its own description
// names when the agent passes none.
func TestMCPEveryEndpointToolSendsItsDescribedMethod(t *testing.T) {
	rec := newMCPRPCRecorder(t)
	s := server.NewMCPServer("cove-rpc-guard", "test")
	RegisterTools(s)
	described := regexp.MustCompile(`\(JSON-RPC ([A-Za-z0-9]+)`)
	var names []string
	for name, entry := range s.ListTools() {
		if _, ok := entry.Tool.InputSchema.Properties["method"]; !ok {
			continue
		}
		if entry.Tool.Meta != nil && entry.Tool.Meta.AdditionalFields != nil && entry.Tool.Meta.AdditionalFields[mcpTenantGateOwnerKey] == "child-cli" {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	if len(names) < 16 {
		t.Fatalf("found %d typed endpoint tools, want at least 16: %v", len(names), names)
	}
	for _, name := range names {
		m := described.FindStringSubmatch(s.GetTool(name).Tool.Description)
		if m == nil {
			t.Fatalf("%s: description names no JSON-RPC method", name)
		}
		rec.reset()
		args := map[string]any{"visa": "visa-0123456789"}
		if name == "devices_stats" {
			args["params-query-partner-id"] = 9000
		}
		result := callEndpointTool(t, s, name, args)
		if result.IsError {
			t.Fatalf("%s failed: %v", name, result.Content)
		}
		reqs := rec.requests()
		if len(reqs) != 1 || reqs[0]["method"] != m[1] || reqs[0]["jsonrpc"] != "2.0" || reqs[0]["id"] != float64(1) {
			t.Fatalf("%s sent %v, want one %s request with jsonrpc 2.0 and id 1", name, reqs, m[1])
		}
		got, _ := reqs[0]["method"].(string)
		if got == "" || !coverpc.IsReadMethod(got) {
			t.Fatalf("%s sent method %q, want a non-empty read", name, got)
		}
		if want, ok := coverpc.EndpointMethodForTool(name); !ok || want != got {
			t.Fatalf("%s: coverpc.EndpointMethodForTool = %q, %v; the tool sent %q", name, want, ok, got)
		}
	}
}
