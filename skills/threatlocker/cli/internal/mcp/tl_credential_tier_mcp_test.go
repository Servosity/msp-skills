// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored: the credential tier on the MCP surface. A credential-returning
// read is not an ordinary read: GET /Organization/OrganizationGetAuthKeyById
// returns the organization's agent-install AuthKey and GET
// /Computer/ComputerGetForNewComputer returns the install key and its download
// URL. Either enrolls a new agent into the tenant. An MCP client (an agent,
// whose context, transcript and tool logs are not a secret store) must never
// receive one, whatever the read-only switch says: threatlocker_read and
// threatlocker_execute refuse it, threatlocker_search and threatlocker_get do
// not offer it, tools/list carries no mirror of the CLI command, and the MCP
// client's transport refuses the path outright.

package mcp

import (
	"context"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"threatlocker-pp-cli/internal/cli"
	"threatlocker-pp-cli/internal/cliutil"
	"threatlocker-pp-cli/internal/mcp/cobratree"
)

// This test's own oracle for the tier, deliberately independent of the
// production allowlist.
var tlCredentialTierEndpoints = []struct {
	id, path string
	command  []string
}{
	{"organizations.auth-key", "/Organization/OrganizationGetAuthKeyById", []string{"organizations", "auth-key"}},
	{"computers.install-info", "/Computer/ComputerGetForNewComputer", []string{"computers", "install-info"}},
}

const tlMCPSentinel = "SENTINEL-mcp-authkey-5ca1ab1e"

// tlMCPCredentialEnv points the MCP client at a fixture that answers every
// request with the sentinel and counts wire hits.
func tlMCPCredentialEnv(t *testing.T) func() int64 {
	t.Helper()
	resetMCPPathEnv(t)
	var hits int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&hits, 1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"organizationId": "org-credential-fixture",
			"authKey":        tlMCPSentinel,
			"installKey":     tlMCPSentinel,
			"downloadUrl":    "https://download.example.test/agent?key=" + tlMCPSentinel,
		})
	}))
	t.Cleanup(srv.Close)
	t.Setenv("THREATLOCKER_BASE_URL", srv.URL)
	t.Setenv("THREATLOCKER_API_KEY", "0000000000000000000000000000000000000000000000000000000000000004")
	t.Setenv("THREATLOCKER_ORG_ID", "")
	t.Setenv("PRINTING_PRESS_CLIENT_PROFILE", "")
	t.Setenv(cliutil.MCPChildEnvVar, "")
	return func() int64 { return atomic.LoadInt64(&hits) }
}

func tlResultText(result *mcplib.CallToolResult) string {
	if result == nil {
		return ""
	}
	var b strings.Builder
	for _, c := range result.Content {
		if tc, ok := c.(mcplib.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String()
}

func tlCall(t *testing.T, handler server.ToolHandlerFunc, args map[string]any) *mcplib.CallToolResult {
	t.Helper()
	req := mcplib.CallToolRequest{}
	req.Params.Arguments = args
	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	return result
}

// Failing input (1): tools/call threatlocker_read
// {"endpoint_id":"organizations.auth-key","params":{}} returned the AuthKey,
// including with THREATLOCKER_MCP_READ_ONLY=1. threatlocker_execute must not
// serve it either, previewed or confirmed.
func TestMCPReadNeverReturnsACredentialTierResponse(t *testing.T) {
	for _, readOnly := range []string{"", "1"} {
		for _, ep := range tlCredentialTierEndpoints {
			ep, readOnly := ep, readOnly
			t.Run(ep.id+"/read_only="+readOnly, func(t *testing.T) {
				hits := tlMCPCredentialEnv(t)
				t.Setenv(cliutil.MCPReadOnlyEnvVar, readOnly)
				for name, call := range map[string]func() *mcplib.CallToolResult{
					"threatlocker_read": func() *mcplib.CallToolResult {
						return tlCall(t, handleCodeOrchRead, map[string]any{"endpoint_id": ep.id, "params": map[string]any{}})
					},
					"threatlocker_execute preview": func() *mcplib.CallToolResult {
						return tlCall(t, handleCodeOrchExecute, map[string]any{"endpoint_id": ep.id, "params": map[string]any{}})
					},
					"threatlocker_execute confirmed": func() *mcplib.CallToolResult {
						return tlCall(t, handleCodeOrchExecute, map[string]any{"endpoint_id": ep.id, "params": map[string]any{}, "confirm": true})
					},
				} {
					result := call()
					text := tlResultText(result)
					if strings.Contains(text, tlMCPSentinel) {
						t.Fatalf("%s returned the credential to the MCP client: %s", name, text)
					}
					if result == nil || !result.IsError {
						t.Fatalf("%s did not refuse %s: %s", name, ep.id, text)
					}
					if name == "threatlocker_read" && !strings.Contains(text, "terminal") {
						t.Fatalf("threatlocker_read refusal does not point at the CLI in a terminal: %s", text)
					}
				}
				if got := hits(); got != 0 {
					t.Fatalf("a refused credential-tier call still reached the API %d time(s)", got)
				}
			})
		}
	}
}

// threatlocker_search must not offer a credential-tier endpoint as callable,
// even to a query that names it: it may say the endpoint exists (so an agent
// can tell its operator where to read the key), but only as callable:false
// with the refusal, never with the method and path an endpoint to call
// carries. threatlocker_get must not describe one.
func TestMCPSearchAndGetNeverOfferACredentialTierEndpoint(t *testing.T) {
	resetMCPPathEnv(t)
	queries := []string{
		"auth key", "authkey", "installation auth key for the current organization",
		"install info", "deployment install info for adding new computers", "install key",
		"organizations", "computers", "organization", "computer",
		"organizations.auth-key", "computers.install-info",
	}
	for _, q := range queries {
		result := tlCall(t, handleCodeOrchSearch, map[string]any{"query": q, "limit": float64(100)})
		var payload struct {
			Results []map[string]any `json:"results"`
		}
		if err := json.Unmarshal([]byte(tlResultText(result)), &payload); err != nil {
			t.Fatalf("search %q: %v\n%s", q, err, tlResultText(result))
		}
		for _, r := range payload.Results {
			for _, ep := range tlCredentialTierEndpoints {
				if r["endpoint_id"] != ep.id {
					continue
				}
				refusal, _ := r["refusal"].(string)
				_, hasMethod := r["method"]
				_, hasPath := r["path"]
				if callable, ok := r["callable"].(bool); !ok || callable || hasMethod || hasPath || !strings.Contains(refusal, "terminal") {
					t.Errorf("threatlocker_search %q offers %s as callable: %v", q, ep.id, r)
				}
			}
		}
	}
	for _, ep := range tlCredentialTierEndpoints {
		result := tlCall(t, handleCodeOrchGet, map[string]any{"endpoint_id": ep.id})
		if result == nil || !result.IsError {
			t.Errorf("threatlocker_get describes %s as a callable endpoint: %s", ep.id, tlResultText(result))
		}
	}
}

// tools/list must never carry a mirror of the CLI command, with or without
// the read-only switch. Today the walker skips it only because it is an
// endpoint command; the second half drops that classification (what a reprint
// that loses the pp:endpoint annotation would do) and requires the tier to
// keep it out anyway.
func TestMCPToolsListNeverMirrorsACredentialTierCommand(t *testing.T) {
	assertNoMirror := func(t *testing.T, s *server.MCPServer) {
		t.Helper()
		for name, entry := range s.ListTools() {
			folded := strings.NewReplacer("-", "", "_", "", ".", "").Replace(strings.ToLower(name))
			if strings.Contains(folded, "authkey") || strings.Contains(folded, "installinfo") {
				t.Errorf("tools/list carries %q", name)
			}
			if entry.Tool.Meta != nil {
				for _, v := range entry.Tool.Meta.AdditionalFields {
					s, _ := v.(string)
					for _, ep := range tlCredentialTierEndpoints {
						if s == strings.Join(ep.command, " ") {
							t.Errorf("tools/list tool %q mirrors %q", name, s)
						}
					}
				}
			}
		}
	}
	for _, readOnly := range []string{"", "1"} {
		readOnly := readOnly
		t.Run("registered/read_only="+readOnly, func(t *testing.T) {
			resetMCPPathEnv(t)
			t.Setenv(cliutil.MCPReadOnlyEnvVar, readOnly)
			s := server.NewMCPServer("threatlocker", "test")
			RegisterTools(s)
			assertNoMirror(t, s)
		})
		t.Run("classification-lost/read_only="+readOnly, func(t *testing.T) {
			resetMCPPathEnv(t)
			t.Setenv(cliutil.MCPReadOnlyEnvVar, readOnly)
			root := cli.RootCmd()
			for _, ep := range tlCredentialTierEndpoints {
				leaf, _, err := root.Find(ep.command)
				if err != nil || leaf == nil {
					t.Fatalf("command %v not found: %v", ep.command, err)
				}
				delete(leaf.Annotations, "pp:endpoint")
				if cobratree.CommandExposableOverMCP(leaf) {
					t.Errorf("%q is exposable over MCP", strings.Join(ep.command, " "))
				}
			}
			s := server.NewMCPServer("threatlocker", "test")
			cobratree.RegisterAll(s, root, func() (string, error) { return os.Args[0], nil })
			assertNoMirror(t, s)
		})
	}
}

// Defence below the handlers: the MCP server's own client refuses a
// credential-tier path at the transport, so a future tool, typed endpoint
// handler, or in-process novel command that reaches it fails closed.
func TestMCPClientRefusesCredentialTierAtTheTransport(t *testing.T) {
	for _, ep := range tlCredentialTierEndpoints {
		ep := ep
		t.Run(ep.id, func(t *testing.T) {
			hits := tlMCPCredentialEnv(t)
			c, session, err := newMCPClient(context.Background())
			if err != nil {
				t.Fatalf("newMCPClient: %v", err)
			}
			if session != nil {
				defer session.ZeroCredentials()
			}
			data, err := c.Get(context.Background(), ep.path, nil)
			if err == nil || strings.Contains(string(data), tlMCPSentinel) {
				t.Fatalf("the MCP client fetched %s: data=%s err=%v", ep.path, data, err)
			}
			if got := hits(); got != 0 {
				t.Fatalf("refused path reached the wire %d time(s)", got)
			}
		})
	}
}

// Tripwire for the next reprint or spec change: every endpoint whose name,
// summary or declared response type carries a credential-shaped field is
// refused by threatlocker_read. It is a detector, not the gate (the gate is
// the explicit allowlist); its job is to make an unlisted credential read
// fail a test rather than ship as an ordinary read.
func TestEveryCredentialShapedResponseIsRefusedOverMCP(t *testing.T) {
	credential := regexp.MustCompile(`(?i)auth.?key|install.?key|api.?key|private.?key|secret|passw(or)?d|credential|(^|[^a-z])token`)
	paging := regexp.MustCompile(`(?i)(page|next|continuation|cursor).?token`)
	shaped := func(s string) bool { return credential.MatchString(paging.ReplaceAllString(s, "")) }

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "../types/types.go", nil, 0)
	if err != nil {
		t.Fatalf("parse types.go: %v", err)
	}
	fieldsOf := map[string][]string{}
	for _, decl := range file.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, spec := range gd.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			st, ok := ts.Type.(*ast.StructType)
			if !ok {
				continue
			}
			for _, f := range st.Fields.List {
				if f.Tag == nil {
					continue
				}
				tag := reflect.StructTag(strings.Trim(f.Tag.Value, "`")).Get("json")
				fieldsOf[ts.Name.Name] = append(fieldsOf[ts.Name.Name], strings.Split(tag, ",")[0])
			}
		}
	}

	raw, err := os.ReadFile("../../../press/tools-manifest.json")
	if err != nil {
		t.Fatalf("read tools-manifest.json: %v", err)
	}
	var manifest struct {
		Tools []struct {
			Name, Description, Method, Path string
		} `json:"tools"`
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	returns := regexp.MustCompile(`Returns (?:the (?:new |updated )?|array of )([A-Z][A-Za-z0-9]*)`)
	detected := map[string]string{}
	for _, tool := range manifest.Tools {
		evidence := ""
		if shaped(tool.Name + " " + tool.Description) {
			evidence = "name/summary"
		}
		if m := returns.FindStringSubmatch(tool.Description); m != nil {
			for _, f := range fieldsOf[m[1]] {
				if shaped(f) {
					evidence = m[1] + "." + f
				}
			}
		}
		if evidence == "" {
			continue
		}
		var id string
		for i := range codeOrchEndpoints {
			if codeOrchEndpoints[i].Path == tool.Path && strings.EqualFold(codeOrchEndpoints[i].Method, tool.Method) {
				id = codeOrchEndpoints[i].ID
			}
		}
		if id == "" {
			t.Fatalf("manifest tool %s (%s %s) has no executor endpoint", tool.Name, tool.Method, tool.Path)
		}
		detected[id] = evidence
	}
	for _, ep := range tlCredentialTierEndpoints {
		if _, ok := detected[ep.id]; !ok {
			t.Fatalf("detector missed known member %s; it cannot be trusted to find a new one (detected: %v)", ep.id, detected)
		}
	}
	for id, evidence := range detected {
		hits := tlMCPCredentialEnv(t)
		result := tlCall(t, handleCodeOrchRead, map[string]any{"endpoint_id": id, "params": map[string]any{}})
		if strings.Contains(tlResultText(result), tlMCPSentinel) || result == nil || !result.IsError || hits() != 0 {
			t.Errorf("%s returns credential-shaped data (%s) but threatlocker_read serves it: %s", id, evidence, tlResultText(result))
		}
	}
}

// An ordinary read the server redirects onto a credential read must not carry
// the credential back: the id-level refusal never sees it (the endpoint_id is
// ordinary), so the client's redirect guard is what holds, read-only or not.
func TestMCPReadOfAnOrdinaryEndpointRedirectedOntoACredentialIsRefused(t *testing.T) {
	for _, readOnly := range []string{"", "1"} {
		readOnly := readOnly
		t.Run("read_only="+readOnly, func(t *testing.T) {
			resetMCPPathEnv(t)
			var memberHits int64
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/Organization/OrganizationGetForMoveComputers" {
					http.Redirect(w, r, "/Organization/OrganizationGetAuthKeyById", http.StatusFound)
					return
				}
				atomic.AddInt64(&memberHits, 1)
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{"organizationId": "org-1", "authKey": tlMCPSentinel})
			}))
			t.Cleanup(srv.Close)
			t.Setenv("THREATLOCKER_BASE_URL", srv.URL)
			t.Setenv("THREATLOCKER_API_KEY", "0000000000000000000000000000000000000000000000000000000000000006")
			t.Setenv("THREATLOCKER_ORG_ID", "")
			t.Setenv("PRINTING_PRESS_CLIENT_PROFILE", "")
			t.Setenv(cliutil.MCPChildEnvVar, "")
			t.Setenv(cliutil.MCPReadOnlyEnvVar, readOnly)
			result := tlCall(t, handleCodeOrchRead, map[string]any{"endpoint_id": "organizations.for-move", "params": map[string]any{}})
			if text := tlResultText(result); strings.Contains(text, tlMCPSentinel) {
				t.Fatalf("a redirected ordinary read returned the credential to the MCP client: %s", text)
			}
			if got := atomic.LoadInt64(&memberHits); got != 0 {
				t.Fatalf("the credential endpoint was reached %d time(s) through a redirect", got)
			}
		})
	}
}
