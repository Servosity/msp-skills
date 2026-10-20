// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/dnsfilter/handfixes.json
// under the "mcp-read-only-withholds-credentials",
// "mcp-execute-writes-refuse-unknown-args" and "mcp-read-only-tool-hints"
// entries.

package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"dnsfilter-pp-cli/internal/cliutil"
	"github.com/mark3labs/mcp-go/server"
)

// dnsfilterMock is a stand-in DNSFilter API that records every request it
// receives, so a test can prove a refused call never left the server.
type dnsfilterMock struct {
	srv  *httptest.Server
	mu   sync.Mutex
	hits []string
}

func newDNSFilterMock(t *testing.T) *dnsfilterMock {
	t.Helper()
	m := &dnsfilterMock{}
	m.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		m.mu.Lock()
		m.hits = append(m.hits, r.Method+" "+r.URL.RequestURI()+" "+string(body))
		m.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasPrefix(r.URL.Path, "/v1/api_keys"):
			_, _ = io.WriteString(w, `{"data":[{"id":1,"name":"ci","last_four":"abcd","token":"SENTINEL-API-TOKEN-91c2"}]}`)
		case r.URL.Path == "/v1/user_agents/uninstall_pin":
			_, _ = io.WriteString(w, `{"data":{"pin":"493817"}}`)
		case strings.HasPrefix(r.URL.Path, "/v1/networks"):
			_, _ = io.WriteString(w, `{"data":[{"id":7,"name":"Main Office","organization_id":3,"secret_key":"`+credentialSentinel+`"}]}`)
		default:
			_, _ = io.WriteString(w, `{"data":{}}`)
		}
	}))
	t.Cleanup(m.srv.Close)
	home := t.TempDir()
	t.Setenv("DNSFILTER_HOME", home)
	t.Setenv("DNSFILTER_CONFIG", "")
	t.Setenv("DNSFILTER_BASE_URL", m.srv.URL)
	t.Setenv("DNSFILTER_API_KEY", "test-key")
	restore, err := cliutil.SetHomeOverride(home)
	if err != nil {
		t.Fatalf("home override: %v", err)
	}
	t.Cleanup(restore)
	return m
}

func (m *dnsfilterMock) requests(prefix string) []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []string
	for _, h := range m.hits {
		if strings.HasPrefix(h, prefix) {
			out = append(out, h)
		}
	}
	return out
}

func newRegisteredServer(t *testing.T) *server.MCPServer {
	t.Helper()
	s := server.NewMCPServer("dnsfilter-test", "test")
	RegisterTools(s)
	return s
}

// TestReadOnlyExecuteRefusesCredentialEndpoints: under DNSFILTER_MCP_READ_ONLY
// the api-keys reads, the uninstall PIN and the GET that creates a PSA
// integration never reach the API, and a networks read returns the site name
// with secret_key withheld. With the switch off the same uninstall-pin call
// reaches the mock, so the refusal is what stops it.
func TestReadOnlyExecuteRefusesCredentialEndpoints(t *testing.T) {
	m := newDNSFilterMock(t)
	t.Setenv(cliutil.MCPReadOnlyEnvVar, "1")
	s := newRegisteredServer(t)

	refused := map[string]map[string]any{
		"api-keys.index":                 {},
		"api-keys.show":                  {"id": 1},
		"user-agents.uninstall-pin":      {},
		"psa-integrations.redirect-link": {},
	}
	for id, params := range refused {
		frame := callToolWire(t, s, "dnsfilter_execute", map[string]any{"endpoint_id": id, "params": params})
		if !strings.Contains(frame, "refusing") || !strings.Contains(frame, cliutil.MCPReadOnlyEnvVar) {
			t.Fatalf("%s was not refused under read-only: %s", id, frame)
		}
	}
	for _, path := range []string{"GET /v1/api_keys", "GET /v1/user_agents/uninstall_pin", "GET /v1/psa_integrations"} {
		if got := m.requests(path); len(got) != 0 {
			t.Fatalf("refused call reached the API: %v", got)
		}
	}

	for _, id := range []string{"networks.index", "networks.all", "networks.msp", "networks.msp-all"} {
		frame := callToolWire(t, s, "dnsfilter_execute", map[string]any{"endpoint_id": id})
		if strings.Contains(frame, "SENTINEL") {
			t.Fatalf("%s returned the network secret_key under read-only: %s", id, frame)
		}
		if !strings.Contains(frame, "Main Office") {
			t.Fatalf("%s lost the site name: %s", id, frame)
		}
	}
	frame := callToolWire(t, s, "dnsfilter_execute", map[string]any{"endpoint_id": "networks.show", "params": map[string]any{"id": 7}})
	if strings.Contains(frame, "SENTINEL") || !strings.Contains(frame, "Main Office") {
		t.Fatalf("networks.show under read-only: %s", frame)
	}
	if got := m.requests("GET /v1/networks"); len(got) == 0 {
		t.Fatal("networks reads never reached the mock; the test proves nothing")
	}

	t.Setenv(cliutil.MCPReadOnlyEnvVar, "")
	s = newRegisteredServer(t)
	frame = callToolWire(t, s, "dnsfilter_execute", map[string]any{"endpoint_id": "user-agents.uninstall-pin"})
	if !strings.Contains(frame, "493817") || len(m.requests("GET /v1/user_agents/uninstall_pin")) != 1 {
		t.Fatalf("with the switch off uninstall-pin should reach the mock: %s", frame)
	}
}

// TestReadOnlyCredentialRefusalCoversRegistry pins the rule over the whole
// generated registry: every credential path is refused, every other GET runs.
func TestReadOnlyCredentialRefusalCoversRegistry(t *testing.T) {
	t.Setenv(cliutil.MCPReadOnlyEnvVar, "1")
	mustRefuse := map[string]bool{
		"api-keys.index": true, "api-keys.show": true,
		"user-agents.uninstall-pin": true, "psa-integrations.redirect-link": true,
	}
	for i := range codeOrchEndpoints {
		ep := &codeOrchEndpoints[i]
		msg := codeOrchReadOnlyRefusal(ep)
		lower := strings.ToLower(ep.Path)
		credential := strings.Contains(lower, "/api_keys") || strings.Contains(lower, "/secret_key") || strings.Contains(lower, "/uninstall_pin")
		switch {
		case mustRefuse[ep.ID] || credential || ep.Method != "GET":
			if msg == "" {
				t.Errorf("%s %s (%s) runs under read-only", ep.Method, ep.Path, ep.ID)
			}
		default:
			if msg != "" {
				t.Errorf("read-only refuses the plain read %s: %s", ep.ID, msg)
			}
		}
	}
}

// TestCodeOrchExecuteRefusesUnknownWriteParams: a write never forwards a param
// outside its schema (the CLI spelling dry-run or dry_run used to land in the
// body or query and run the write live); a schema-shaped write and a GET with
// an extra query param still go through.
func TestCodeOrchExecuteRefusesUnknownWriteParams(t *testing.T) {
	m := newDNSFilterMock(t)
	t.Setenv(cliutil.MCPReadOnlyEnvVar, "")
	s := newRegisteredServer(t)

	cases := []struct {
		id     string
		params map[string]any
		bad    string
	}{
		{"networks.create", map[string]any{"network": map[string]any{"name": "x", "organization_id": 1}, "dry_run": true}, "dry_run"},
		{"networks.create", map[string]any{"name": "flat-not-wrapped"}, "name"},
		{"networks.update", map[string]any{"id": 7, "network": map[string]any{"name": "y"}, "dry-run": true}, "dry-run"},
		{"networks.destroy", map[string]any{"id": 7, "dry-run": true}, "dry-run"},
		{"networks.destroy", map[string]any{"id": 7, "confirm": false}, "confirm"},
		// A GET that writes (it creates a PSA integration) is a write too.
		{"psa-integrations.redirect-link", map[string]any{"organization_id": 5, "psa_name": "connectwise", "dry_run": true}, "dry_run"},
	}
	for _, tc := range cases {
		frame := callToolWire(t, s, "dnsfilter_execute", map[string]any{"endpoint_id": tc.id, "params": tc.params})
		if !strings.Contains(frame, `unknown MCP parameter \"`+tc.bad+`\" on a write tool`) {
			t.Fatalf("%s with %q was not refused: %s", tc.id, tc.bad, frame)
		}
	}
	for _, method := range []string{"POST ", "PATCH ", "DELETE ", "GET /v1/psa_integrations"} {
		if got := m.requests(method); len(got) != 0 {
			t.Fatalf("a refused write reached the API: %v", got)
		}
	}

	frame := callToolWire(t, s, "dnsfilter_execute", map[string]any{"endpoint_id": "networks.create", "params": map[string]any{"network": map[string]any{"name": "Branch", "organization_id": 3}}})
	if strings.Contains(frame, "unknown MCP parameter") {
		t.Fatalf("schema-shaped write refused: %s", frame)
	}
	posts := m.requests("POST /v1/networks")
	if len(posts) != 1 || !strings.Contains(posts[0], `"network":{`) {
		t.Fatalf("schema-shaped write did not reach the API as sent: %v", posts)
	}
	frame = callToolWire(t, s, "dnsfilter_execute", map[string]any{"endpoint_id": "networks.destroy", "params": map[string]any{"id": 7}})
	if strings.Contains(frame, "unknown MCP parameter") || len(m.requests("DELETE /v1/networks/7")) != 1 {
		t.Fatalf("schema-shaped delete did not go through: %s", frame)
	}
	callToolWire(t, s, "dnsfilter_execute", map[string]any{"endpoint_id": "networks.index", "params": map[string]any{"page[number]": 2}})
	if got := m.requests("GET /v1/networks?page%5Bnumber%5D=2"); len(got) != 1 {
		t.Fatalf("GET extra query param no longer forwarded: %v", m.requests("GET "))
	}
}

// TestReadOnlyToolHints: the recipe intent is a local read in every mode, and
// under the read-only switch dnsfilter_execute (which then runs only safe
// reads) advertises itself as read-only.
func TestReadOnlyToolHints(t *testing.T) {
	for _, ro := range []string{"", "1"} {
		t.Setenv(cliutil.MCPReadOnlyEnvVar, ro)
		s := newRegisteredServer(t)
		intent := s.GetTool("precedent_check_before_allowing_a_domain")
		if intent == nil {
			t.Fatal("intent not registered")
		}
		a := intent.Tool.Annotations
		if a.ReadOnlyHint == nil || !*a.ReadOnlyHint || a.DestructiveHint == nil || *a.DestructiveHint {
			t.Fatalf("read-only=%q: intent annotations %+v, want readOnly=true destructive=false", ro, a)
		}
		exec := s.GetTool("dnsfilter_execute").Tool.Annotations
		gotRO := exec.ReadOnlyHint != nil && *exec.ReadOnlyHint
		gotDestructive := exec.DestructiveHint == nil || *exec.DestructiveHint
		if ro == "1" && (!gotRO || gotDestructive) {
			t.Fatalf("read-only mode: dnsfilter_execute annotations %+v, want readOnly=true destructive=false", exec)
		}
		if ro == "" && (gotRO || !gotDestructive) {
			t.Fatalf("default mode: dnsfilter_execute must stay destructive (it can DELETE): %+v", exec)
		}
	}
}

// TestContextToolDescribesDNSFilter: the context tool agents call first states
// the real paging (page[number]/page[size]), the real tool surface, and names
// only tools that exist.
func TestContextToolDescribesDNSFilter(t *testing.T) {
	t.Setenv(cliutil.MCPReadOnlyEnvVar, "")
	s := newRegisteredServer(t)
	frame := callToolWire(t, s, "context", map[string]any{})
	texts := resultTexts(t, frame)
	var ctx map[string]any
	if err := json.Unmarshal([]byte(texts[0]), &ctx); err != nil {
		t.Fatalf("context is not JSON: %v", err)
	}
	surface, _ := ctx["tool_surface"].(string)
	tips, _ := json.Marshal(ctx["query_tips"])
	all := surface + string(tips)
	for _, bad := range []string{"cursor-based", "typed endpoint tools", "updated_at_from for incremental"} {
		if strings.Contains(all, bad) {
			t.Fatalf("context still claims %q: %s", bad, all)
		}
	}
	for _, want := range []string{"page[number]", "page[size]", "no dry-run", cliutil.MCPReadOnlyEnvVar} {
		if !strings.Contains(all, want) {
			t.Fatalf("context does not mention %q: %s", want, all)
		}
	}
	for _, tool := range []string{"dnsfilter_search", "dnsfilter_get", "dnsfilter_execute", "sync", "sql", "search", "why_blocked", "allow", "unblocks_triage", "agents_drift", "agents_stale", "lists_where", "policy_audit"} {
		if !strings.Contains(surface, tool) {
			t.Fatalf("tool_surface does not name %s: %s", tool, surface)
		}
		if s.GetTool(tool) == nil {
			t.Fatalf("tool_surface names %s, which is not registered", tool)
		}
	}
	// Under the read-only switch the tip that explains read-only mode must
	// survive the result funnel (a one-line list of field names is not a CSV
	// header).
	t.Setenv(cliutil.MCPReadOnlyEnvVar, "1")
	roFrame := callToolWire(t, newRegisteredServer(t), "context", map[string]any{})
	if strings.Contains(roFrame, "[withheld") || !strings.Contains(roFrame, "Under DNSFILTER_MCP_READ_ONLY=1, dnsfilter_execute runs only GETs") {
		t.Fatalf("read-only mode withheld the context tool's own tip: %.400s", roFrame)
	}
	t.Setenv(cliutil.MCPReadOnlyEnvVar, "")
	hint := mcpSQLQueryError(context.Background(), errors.New("no such table: network"))
	if strings.Contains(hint, "not one SQL table per resource") || !strings.Contains(hint, "resource_type='policies'") {
		t.Fatalf("sql no-such-table hint is not truthful for this store: %s", hint)
	}
}
