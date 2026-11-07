// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"immybot-pp-cli/internal/cli"
	"immybot-pp-cli/internal/cliutil"
	"immybot-pp-cli/internal/mcp/cobratree"
)

// The credential tier over MCP (internal/cliutil/immybot_credential_tier.go):
// AzureTenantAuthDetails.customAppRegSecret, returned by the GET and echoed by
// the update, is never returned to an MCP client, with or without the
// read-only switch.

const credTierMCPSecret = "credTierMCPSentinel<&>Q9"

var credTierMCPMembers = map[string]string{
	"access.get-get-azure-tenant-auth-details-by-azure-tenant-principal-id": "immybot-cli access get-get-azure-tenant-auth-details-by-azure-tenant-principal-id",
	"access.create-update-azure-tenant-auth-details":                        "immybot-cli access create-update-azure-tenant-auth-details",
}

func credTierMCPStub(t *testing.T) (*httptest.Server, *atomic.Int32, *atomic.Value) {
	t.Helper()
	var hits atomic.Int32
	var seen atomic.Value
	seen.Store("")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/auth/get-ip-addresses" {
			_, _ = w.Write([]byte(`[{"id":1}]`))
			return
		}
		hits.Add(1)
		seen.Store(r.Method + " " + r.URL.String())
		details := map[string]any{"azureTenantPrincipalId": "11111111-1111-1111-1111-111111111111", "customAppRegSecret": credTierMCPSecret}
		_ = json.NewEncoder(w).Encode(map[string]any{"authDetails": details, "customAppRegSecret": credTierMCPSecret})
	}))
	t.Cleanup(srv.Close)
	requestContractConfig(t, srv.URL)
	return srv, &hits, &seen
}

func credTierMCPCall(t *testing.T, handler server.ToolHandlerFunc, args map[string]any) (string, bool) {
	t.Helper()
	var req mcplib.CallToolRequest
	req.Params.Arguments = args
	res, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if res == nil || len(res.Content) == 0 {
		t.Fatalf("empty result for %v", args)
	}
	text, _ := res.Content[0].(mcplib.TextContent)
	return text.Text, res.IsError
}

// TestCredentialTierRefusedByMCPExecutor drives the Codex P1 input and every
// other executor route onto both members, with and without the read-only
// switch: immybot_read, immybot_execute (preview and confirmed) and
// immybot_get each refuse before any request and name the CLI command (the
// read-only switch refuses the update first, with its own message). The stub
// must see zero requests.
func TestCredentialTierRefusedByMCPExecutor(t *testing.T) {
	_, hits, seen := credTierMCPStub(t)
	params := map[string]any{
		"azureTenantPrincipalId": "11111111-1111-1111-1111-111111111111",
		"authDetails":            map[string]any{"azureTenantPrincipalId": "11111111-1111-1111-1111-111111111111"},
	}
	for _, readOnly := range []string{"", "1"} {
		t.Setenv(cliutil.MCPReadOnlyEnvVar, readOnly)
		for id, command := range credTierMCPMembers {
			calls := map[string]struct {
				handler server.ToolHandlerFunc
				args    map[string]any
			}{
				"immybot_read":              {handleCodeOrchRead, map[string]any{"endpoint_id": id, "params": params}},
				"immybot_execute":           {handleCodeOrchExecute, map[string]any{"endpoint_id": id, "params": params}},
				"immybot_execute confirmed": {handleCodeOrchExecute, map[string]any{"endpoint_id": id, "params": params, "confirm": true}},
				"immybot_get":               {handleCodeOrchGet, map[string]any{"endpoint_id": id}},
			}
			for name, call := range calls {
				text, isErr := credTierMCPCall(t, call.handler, call.args)
				if !isErr {
					t.Errorf("read-only=%q %s %s: not refused: %s", readOnly, name, id, text)
				}
				readOnlyRefusal := readOnly == "1" && strings.Contains(text, cliutil.MCPReadOnlyEnvVar)
				if !strings.Contains(text, command) && !readOnlyRefusal {
					t.Errorf("read-only=%q %s %s: refusal does not name %q: %s", readOnly, name, id, command, text)
				}
				if strings.Contains(text, credTierMCPSecret) {
					t.Errorf("read-only=%q %s %s: returned the secret: %s", readOnly, name, id, text)
				}
			}
		}
	}
	if n := hits.Load(); n != 0 {
		t.Errorf("an MCP call reached a credential endpoint %d time(s) (last %v)", n, seen.Load())
	}
}

// TestCredentialTierSearchListsItAsCLIOnly: immybot_search still finds both
// members, so an agent asked for the secret can tell its operator where to
// get it, but it offers no method or path to call and says it is CLI-only.
func TestCredentialTierSearchListsItAsCLIOnly(t *testing.T) {
	credTierMCPStub(t)
	text, isErr := credTierMCPCall(t, handleCodeOrchSearch, map[string]any{"query": "azure tenant auth details", "limit": float64(100)})
	if isErr {
		t.Fatalf("search failed: %s", text)
	}
	var got struct {
		Results []map[string]any `json:"results"`
	}
	if err := json.Unmarshal([]byte(text), &got); err != nil {
		t.Fatalf("search result is not JSON: %v\n%s", err, text)
	}
	listed := map[string]bool{}
	for _, item := range got.Results {
		id, _ := item["endpoint_id"].(string)
		command, member := credTierMCPMembers[id]
		if !member {
			continue
		}
		listed[id] = true
		if item["callable"] != false {
			t.Errorf("%s is listed as callable: %v", id, item)
		}
		if _, ok := item["method"]; ok {
			t.Errorf("%s is listed with a method to call: %v", id, item)
		}
		if _, ok := item["path"]; ok {
			t.Errorf("%s is listed with a path to call: %v", id, item)
		}
		refusal, _ := item["refusal"].(string)
		if !strings.Contains(refusal, command) || !strings.Contains(refusal, "CLI only") {
			t.Errorf("%s: search does not say it is CLI only via %q: %v", id, command, item)
		}
	}
	for id := range credTierMCPMembers {
		if !listed[id] {
			t.Errorf("search no longer lists %s", id)
		}
	}
}

// TestCredentialTierTransportRefusedInsideMCPServer: the MCP server's own
// client refuses every spelling of a member path before dialing, so a tool
// added later, or a resolved path that lands on a member, cannot fetch it.
// The control read on an ordinary path still reaches the stub.
func TestCredentialTierTransportRefusedInsideMCPServer(t *testing.T) {
	srv, hits, seen := credTierMCPStub(t)
	c, session, err := newMCPClient(context.Background())
	if err != nil {
		t.Fatalf("newMCPClient: %v", err)
	}
	if session != nil {
		defer session.ZeroCredentials()
	}
	for _, path := range []string{
		"/api/v1/auth/get-azure-tenant-auth-details/11111111-1111-1111-1111-111111111111",
		"/API/V1/Auth/Get-Azure-Tenant-Auth-Details/abc/",
		"/api/v1/auth/get-azure-tenant-auth-details/a%2Fb?x=1",
		"/api/v1//auth/./get-azure-tenant-auth-details/abc",
		"/api/v1/auth/x/../get-azure-tenant-auth-details/abc",
		"/api/v1/auth/get-azure-tenant-auth-details%2Fabc",
		srv.URL + "/api/v1/auth/get-azure-tenant-auth-details/abc",
	} {
		if data, err := c.Get(context.Background(), path, nil); err == nil {
			t.Errorf("GET %s was not refused inside the MCP server: %s", path, data)
		}
	}
	if _, _, err := c.Post(context.Background(), "/api/v1/auth/update-azure-tenant-auth-details", map[string]any{}); err == nil {
		t.Error("POST update-azure-tenant-auth-details was not refused inside the MCP server")
	}
	if n := hits.Load(); n != 0 {
		t.Errorf("the MCP server's client reached a credential endpoint %d time(s) (last %v)", n, seen.Load())
	}
	if _, err := c.Get(context.Background(), "/api/v1/auth/get-ip-addresses", nil); err != nil {
		t.Fatalf("control: an ordinary read failed, so the refusal proves nothing: %v", err)
	}
}

// TestCredentialTierNoMirroredToolExposesIt: no tool the server registers,
// mirrored or recipe, wraps a member command.
func TestCredentialTierNoMirroredToolExposesIt(t *testing.T) {
	credTierMCPStub(t)
	s := server.NewMCPServer("immybot", "test")
	RegisterTools(s)
	root := cli.RootCmd()
	for name, tool := range s.ListTools() {
		if tool.Tool.Meta == nil {
			continue
		}
		path, _ := tool.Tool.Meta.AdditionalFields["pp:cli-command"].(string)
		if path == "" {
			continue
		}
		cmd := cobratree.CommandAtPath(root, strings.Fields(path))
		if cmd == nil {
			continue
		}
		for id := range credTierMCPMembers {
			if cmd.Annotations["pp:endpoint"] == id {
				t.Errorf("MCP tool %q mirrors credential-tier command %q", name, path)
			}
		}
	}
	for id, command := range credTierMCPMembers {
		cmd := cobratree.CommandAtPath(root, strings.Fields(strings.TrimPrefix(command, "immybot-cli ")))
		if cmd == nil || cmd.Annotations["pp:endpoint"] != id {
			t.Fatalf("%s: command %q not found", id, command)
		}
		if cobratree.CommandExposableOverMCP(cmd) {
			t.Errorf("%q is exposable over MCP", command)
		}
	}
}

// TestCredentialTierMembershipIsExactlyTheTwoEndpoints: of every endpoint the
// executor registry knows, only the two members are refused by id or path.
func TestCredentialTierMembershipIsExactlyTheTwoEndpoints(t *testing.T) {
	_, hits, _ := credTierMCPStub(t)
	refused := map[string]bool{}
	for i := range codeOrchEndpoints {
		ep := &codeOrchEndpoints[i]
		text, isErr := credTierMCPCall(t, handleCodeOrchGet, map[string]any{"endpoint_id": ep.ID})
		if isErr && strings.Contains(text, "CLI only") {
			refused[ep.ID] = true
		}
	}
	for id := range credTierMCPMembers {
		if !refused[id] {
			t.Errorf("%s is not refused by immybot_get", id)
		}
	}
	for id := range refused {
		if _, ok := credTierMCPMembers[id]; !ok {
			t.Errorf("%s is refused as a credential read but is not a member", id)
		}
	}
	if n := hits.Load(); n != 0 {
		t.Errorf("immybot_get reached the vendor %d time(s)", n)
	}
}
