// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): tests for the MCP credential-tier choke point
// (s1_credential_tier_mcp.go). Every call here goes through the server's real
// tools/call path (HandleMessage), so the middleware chain is the one main.go
// builds (TestMCPCredentialTierChokePointCoversEveryTool pins that).

package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"sentinelone-pp-cli/internal/cli"
	"sentinelone-pp-cli/internal/cliutil"
	"sentinelone-pp-cli/internal/cliutil/testenv"
	mcptools "sentinelone-pp-cli/internal/mcp"
	"sentinelone-pp-cli/internal/store"
)

// The adversarial round-2 Ranger body: three credentials under keys
// (community, sshKey, authKey) the secret-field NAME rule does not match, so
// only refusing the resource keeps them from an agent.
const (
	rangerSNMPCommunity = "SNMP-COMMUNITY-SECRET-555"
	rangerSSHKey        = "SSHKEY-SECRET-444"
	rangerSNMPv3AuthKey = "SNMPV3-AUTHKEY-222"
	rangerCredBody      = `{"data":[` +
		`{"id":"c1","credGroupId":"g1","title":"core switches","credType":"snmpv2","community":"` + rangerSNMPCommunity + `"},` +
		`{"id":"c2","credGroupId":"g1","title":"linux deploy","credType":"ssh","username":"deploy","sshKey":"` + rangerSSHKey + `"},` +
		`{"id":"c3","credGroupId":"g1","title":"core v3","credType":"snmpv3","authProtocol":"SHA","authKey":"` + rangerSNMPv3AuthKey + `"}` +
		`],"pagination":{"nextCursor":null,"totalItems":3}}`
)

var rangerSentinels = []string{rangerSNMPCommunity, rangerSSHKey, rangerSNMPv3AuthKey}

type tierStub struct {
	srv  *httptest.Server
	mu   sync.Mutex
	hits []string
}

func newTierStub(t *testing.T) *tierStub {
	t.Helper()
	st := &tierStub{}
	st.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		st.mu.Lock()
		st.hits = append(st.hits, r.Method+" "+r.URL.Path)
		st.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(rangerCredBody))
	}))
	t.Cleanup(st.srv.Close)
	return st
}

func (s *tierStub) take() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	h := s.hits
	s.hits = nil
	return h
}

// newProductionMCPServer builds the server main.go builds: the same tool
// middlewares in the same order, then every tool.
func newProductionMCPServer(t *testing.T) *server.MCPServer {
	t.Helper()
	s := server.NewMCPServer("t", "0",
		server.WithToolHandlerMiddleware(funnelToolErrors),
		server.WithToolHandlerMiddleware(withholdToolSecrets),
		server.WithToolHandlerMiddleware(refuseCredentialTier),
		server.WithToolCapabilities(false),
	)
	mcptools.RegisterTools(s)
	return s
}

type toolCallResult struct {
	IsError   bool
	Text      string
	ProtoErr  string
	Responded bool
}

// callMCPTool sends one JSON-RPC tools/call through the server, exactly as a
// client's request arrives over stdio or HTTP.
func callMCPTool(t *testing.T, s *server.MCPServer, name string, args any) toolCallResult {
	t.Helper()
	msg, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": map[string]any{"name": name, "arguments": args},
	})
	if err != nil {
		t.Fatal(err)
	}
	resp := s.HandleMessage(context.Background(), msg)
	raw, err := json.Marshal(resp)
	if err != nil {
		t.Fatal(err)
	}
	var env struct {
		Result *struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
			IsError bool `json:"isError"`
		} `json:"result"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("decoding %s response %s: %v", name, raw, err)
	}
	out := toolCallResult{Responded: true}
	if env.Error != nil {
		out.ProtoErr = env.Error.Message
		return out
	}
	if env.Result != nil {
		out.IsError = env.Result.IsError
		for _, c := range env.Result.Content {
			out.Text += c.Text
		}
	}
	return out
}

func leakedRangerSentinels(text string) []string {
	var got []string
	for _, s := range rangerSentinels {
		if strings.Contains(text, s) {
			got = append(got, s)
		}
	}
	return got
}

// buildCompanionCLI builds the real sentinelone-cli so the mirrored tools
// (tail, export) run as the MCP server runs them: a child process. Called
// before testenv.Isolate so the go command keeps the operator's build and
// module caches.
func buildCompanionCLI(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not on PATH; cannot build the companion CLI")
	}
	bin := filepath.Join(t.TempDir(), "sentinelone-cli")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	// The test runs in cmd/sentinelone-mcp, so the CLI package is its sibling.
	cmd := exec.Command("go", "build", "-o", bin, "../sentinelone-cli")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("building the companion CLI: %v\n%s", err, out)
	}
	return bin
}

// TestMCPCredentialTierRefusedOnEveryReader replays adversarial round 2: with
// and without SENTINELONE_MCP_READ_ONLY=1, tail and export of the Ranger
// credential details (by the CLI name, the API path, and the snake and camel
// spellings) are refused before anything reaches the API; sentinelone_read is
// refused; an unconfirmed sentinelone_execute only previews; and
// sentinelone_execute with confirm: true is the one call that returns the
// values.
func TestMCPCredentialTierRefusedOnEveryReader(t *testing.T) {
	bin := buildCompanionCLI(t)
	stub := newTierStub(t)
	testenv.Isolate(t)
	t.Setenv("SENTINELONE_CLI_PATH", bin)
	t.Setenv("SENTINELONE_BASE_URL", stub.srv.URL)
	t.Setenv("SENTINELONE_API_TOKEN", "mcp-credential-tier-test-token")

	spellings := []string{"ranger", "/ranger/cred-groups/details", "Ranger_Cred_Groups", "rangerCredGroups", "/web/api/v2.1/ranger/cred-groups/details"}
	for _, ro := range []string{"", "1"} {
		t.Run("read-only="+ro, func(t *testing.T) {
			t.Setenv(cliutil.MCPReadOnlyEnvVar, ro)
			s := newProductionMCPServer(t)
			stub.take()

			type call struct {
				tool string
				args map[string]any
			}
			var calls []call
			for _, sp := range spellings {
				calls = append(calls,
					call{"tail", map[string]any{"resource": sp, "follow": false}},
					call{"tail", map[string]any{"positional-resource": sp, "follow": false}},
					call{"export", map[string]any{"resource": sp}},
					call{"export", map[string]any{"resource": sp, "format": "json"}},
				)
			}
			// A path parameter that smuggles a credential path into an
			// ordinary detail read.
			calls = append(calls, call{"export", map[string]any{"resource": "accounts", "id": "123/uninstall-password/view"}})
			for _, c := range calls {
				if s.GetTool(c.tool) == nil {
					// export is a non-read-only tool, dropped under the switch:
					// it cannot be called at all, which is a refusal too.
					if ro == "" || c.tool != "export" {
						t.Fatalf("%s is not registered", c.tool)
					}
					res := callMCPTool(t, s, c.tool, c.args)
					if res.ProtoErr == "" {
						t.Errorf("%s answered while not registered: %+v", c.tool, res)
					}
					continue
				}
				res := callMCPTool(t, s, c.tool, c.args)
				if leaks := leakedRangerSentinels(res.Text); len(leaks) > 0 {
					t.Errorf("%s %v returned Ranger credentials %v:\n%s", c.tool, c.args, leaks, res.Text)
				}
				if !res.IsError || !strings.Contains(res.Text, tierRefusalPrefix) {
					t.Errorf("%s %v was not refused as credential-tier: %+v", c.tool, c.args, res)
				}
				if hits := stub.take(); len(hits) > 0 {
					t.Errorf("%s %v sent %v to the API", c.tool, c.args, hits)
				}
			}

			// sentinelone_read on the endpoint: refused (it needs confirm).
			res := callMCPTool(t, s, "sentinelone_read", map[string]any{"endpoint_id": "ranger.get-cred-group-details"})
			if !res.IsError || len(leakedRangerSentinels(res.Text)) > 0 {
				t.Errorf("sentinelone_read ranger.get-cred-group-details answered: %+v", res)
			}
			if hits := stub.take(); len(hits) > 0 {
				t.Errorf("sentinelone_read sent %v to the API", hits)
			}

			if s.GetTool("sentinelone_execute") == nil {
				if ro != "1" {
					t.Fatal("sentinelone_execute is not registered")
				}
				return
			}
			// Unconfirmed: a preview, nothing sent.
			res = callMCPTool(t, s, "sentinelone_execute", map[string]any{"endpoint_id": "ranger.get-cred-group-details"})
			if res.IsError || !strings.Contains(res.Text, "confirmation_required") || len(leakedRangerSentinels(res.Text)) > 0 {
				t.Errorf("unconfirmed sentinelone_execute did not only preview: %+v", res)
			}
			for _, notTrue := range []any{"true", 1, false} {
				res = callMCPTool(t, s, "sentinelone_execute", map[string]any{"endpoint_id": "ranger.get-cred-group-details", "confirm": notTrue})
				if len(leakedRangerSentinels(res.Text)) > 0 {
					t.Errorf("sentinelone_execute with confirm %#v returned credentials: %+v", notTrue, res)
				}
			}
			if hits := stub.take(); len(hits) > 0 {
				t.Errorf("unconfirmed sentinelone_execute sent %v to the API", hits)
			}
			// Confirmed: the one path that returns the values.
			res = callMCPTool(t, s, "sentinelone_execute", map[string]any{"endpoint_id": "ranger.get-cred-group-details", "confirm": true})
			if res.IsError || len(leakedRangerSentinels(res.Text)) != len(rangerSentinels) {
				t.Errorf("sentinelone_execute with confirm: true did not return the values: %+v", res)
			}
			if hits := stub.take(); len(hits) != 1 || !strings.HasSuffix(hits[0], "/ranger/cred-groups/details") {
				t.Errorf("confirmed sentinelone_execute requests = %v, want one GET /ranger/cred-groups/details", hits)
			}
		})
	}
}

// TestMCPSqlCredentialTierRefused: Codex round 2's alias query is answered
// without the passphrase (the store's write path drops it, so the row has no
// passphrase to alias), and every query naming a credential-tier table,
// resource or path, in any spelling and through ||, comments, quoting and
// concat() splits, is refused before it runs.
func TestMCPSqlCredentialTierRefused(t *testing.T) {
	testenv.Isolate(t)
	t.Setenv("SENTINELONE_BASE_URL", "http://127.0.0.1:1")
	t.Setenv("SENTINELONE_API_TOKEN", "mcp-sql-tier-test-token")
	const pass = "CODEX ALIAS PASSPHRASE 7781"
	path, err := cli.MCPStorePath()
	if err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	// Seed through the store's own write paths.
	if err := db.UpsertActions(json.RawMessage(`{"id":"11","agents_id":"a1","agentId":"a1","action":"fetch","passphrase":"` + pass + `"}`)); err != nil {
		t.Fatal(err)
	}
	if err := db.Upsert("agents-passphrases", "11", json.RawMessage(`{"id":"11","passphrase":"`+pass+`"}`)); err != nil {
		t.Fatal(err)
	}
	if err := db.Upsert("agents", "a1", json.RawMessage(`{"id":"a1","computerName":"WS1","rangerStatus":"Enabled"}`)); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()

	s := newProductionMCPServer(t)
	answered := func(q string) string {
		t.Helper()
		res := callMCPTool(t, s, "sql", map[string]any{"query": q})
		if res.IsError || res.ProtoErr != "" {
			t.Fatalf("sql %q was not answered: %+v", q, res)
		}
		if strings.Contains(res.Text, pass) {
			t.Fatalf("sql %q returned the passphrase: %s", q, res.Text)
		}
		return res.Text
	}
	// Codex round 2, verbatim: answered, row present, passphrase absent.
	if got := answered(`SELECT json_extract(data,'$.passphrase') AS v FROM actions`); !strings.Contains(got, `"v"`) {
		t.Errorf("Codex's query returned no row: %s", got)
	}
	// What the normalization cannot see finds nothing: the store kept no
	// credential-tier content (store invariant).
	answered(`SELECT data FROM resources WHERE resource_type LIKE 'agents-pass%'`)
	answered(`SELECT value AS v FROM resources, json_each(resources.data) WHERE key LIKE '%phrase'`)
	// Ordinary queries that mention ranger as part of a longer name still run.
	answered(`SELECT json_extract(data,'$.rangerStatus') AS r FROM resources WHERE resource_type='agents'`)
	answered(`SELECT count(*) AS n FROM resources WHERE resource_type='ranger-table-view'`)

	for _, q := range []string{
		`SELECT * FROM uninstall_password`,
		`SELECT data FROM "Uninstall_Password"`,
		`SELECT data FROM [uninstall_password]`,
		"SELECT data FROM `uninstall_password`",
		`SELECT data FROM uninstall/**/_password`,
		`SELECT * FROM sites_regenerate_key`,
		`SELECT * FROM groups_regenerate_key`,
		`SELECT data FROM resources WHERE resource_type='agents-passphrases'`,
		`SELECT data FROM resources WHERE resource_type='AgentsPassphrases'`,
		`SELECT data FROM resources WHERE resource_type='agents-pass'||'phrases'`,
		`SELECT data FROM resources WHERE resource_type = 'agents_' || 'passphrases'`,
		`SELECT data FROM resources WHERE resource_type = "agents-pass" || "phrases"`,
		`SELECT data FROM resources WHERE resource_type = concat('agents-pass', 'phrases')`,
		`SELECT data FROM resources WHERE resource_type = 'agents-pass' /* split */ || 'phrases'`,
		"SELECT data FROM resources -- comment\nWHERE resource_type='login-by-token'",
		`SELECT data FROM resources WHERE resource_type='users-login-by-token'`,
		`SELECT data FROM resources WHERE resource_type='ranger'`,
		`SELECT data FROM resources WHERE resource_type='RANGER'`,
		`SELECT data FROM resources WHERE resource_type='ran'||'ger'`,
		`SELECT data FROM resources WHERE resource_type IN ('agents','ranger')`,
		`SELECT data FROM resources WHERE resource_type='Ranger_Cred_Groups'`,
		`SELECT data FROM resources WHERE resource_type='rangerCredGroups'`,
		`SELECT data FROM resources WHERE resource_type='uninstall-password'`,
		`SELECT data FROM resources WHERE json_extract(data,'$.path')='/web/api/v2.1/ranger/cred-groups/details'`,
		`SELECT data FROM resources WHERE json_extract(data,'$.path')='/ranger/cred-' || 'groups/details'`,
	} {
		res := callMCPTool(t, s, "sql", map[string]any{"query": q})
		if !res.IsError || !strings.Contains(res.Text, tierRefusalPrefix) {
			t.Errorf("sql %q was not refused as credential-tier: %+v", q, res)
		}
	}
}

// TestMCPCredentialTierChokePointCoversEveryTool: main.go registers the
// choke point in the chain the tests build, and every tool the server
// registers (with and without SENTINELONE_MCP_READ_ONLY=1, plus a tool added
// after RegisterTools, standing in for a future reader) is refused before its
// handler runs when any argument at any depth names the credential tier;
// ordinary arguments still reach the handler; and sentinelone_execute with a
// top-level boolean confirm: true is the only call let through.
func TestMCPCredentialTierChokePointCoversEveryTool(t *testing.T) {
	src, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	main := string(src)
	order := []string{
		"server.WithToolHandlerMiddleware(funnelToolErrors)",
		"server.WithToolHandlerMiddleware(withholdToolSecrets)",
		"server.WithToolHandlerMiddleware(refuseCredentialTier)",
		"mcptools.RegisterTools(s)",
	}
	last := -1
	for _, want := range order {
		i := strings.Index(main, want)
		if i < 0 || i < last {
			t.Errorf("main.go must register, in order, %q (missing or out of order: %q)", order, want)
			continue
		}
		last = i
	}

	testenv.Isolate(t)
	t.Setenv("SENTINELONE_BASE_URL", "http://127.0.0.1:1")
	t.Setenv("SENTINELONE_API_TOKEN", "mcp-choke-point-test-token")
	probes := []any{
		map[string]any{"probe": "agents-passphrases"},
		map[string]any{"probe": "Ranger"},
		map[string]any{"params": map[string]any{"a": []any{"ordinary", map[string]any{"deep": "/web/api/v2.1/ranger/cred-groups/details?limit=1"}}}},
		map[string]any{"probe": "sites,agents-passphrases"},
		map[string]any{"probe": "123%2Funinstall-password%2Fview"},
		map[string]any{"probe": "SELECT data FROM resources WHERE resource_type='login-by-'||'token'"},
		map[string]any{"uninstall_password": "used as a key"},
		map[string]any{"confirm": "true", "probe": "agents-passphrases"},
		map[string]any{"params": map[string]any{"confirm": true}, "probe": "agents-passphrases"},
	}
	for _, ro := range []string{"", "1"} {
		t.Run("read-only="+ro, func(t *testing.T) {
			t.Setenv(cliutil.MCPReadOnlyEnvVar, ro)
			s := newProductionMCPServer(t)
			s.AddTool(mcplib.NewTool("future_reader", mcplib.WithReadOnlyHintAnnotation(true)), nil)
			var mu sync.Mutex
			ran := map[string]int{}
			tools := s.ListTools()
			var names []string
			for name, entry := range tools {
				names = append(names, name)
				name := name
				s.AddTool(entry.Tool, func(context.Context, mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
					mu.Lock()
					ran[name]++
					mu.Unlock()
					return mcplib.NewToolResultText("handler ran"), nil
				})
			}
			sort.Strings(names)
			for _, must := range []string{"tail", "sql", "search", "analytics", "sentinelone_read", "sentinelone_get", "sentinelone_search", "workflow_status", "future_reader"} {
				if tools[must] == nil {
					t.Fatalf("expected tool %s to be registered; have %v", must, names)
				}
			}
			if ro == "" {
				for _, must := range []string{"export", "sync", "workflow_archive", "sentinelone_execute", "import", "trace_a_threat_across_the_fleet"} {
					if tools[must] == nil {
						t.Fatalf("expected tool %s to be registered; have %v", must, names)
					}
				}
			}
			for _, name := range names {
				for _, p := range probes {
					res := callMCPTool(t, s, name, p)
					if !res.IsError || !strings.Contains(res.Text, tierRefusalPrefix) {
						t.Errorf("tool %s bypasses the credential-tier choke point with %v: %+v", name, p, res)
					}
				}
				if ran[name] != 0 {
					t.Errorf("tool %s's handler ran %d time(s) for a credential-tier argument", name, ran[name])
				}
				// Not a blanket refusal: an ordinary argument reaches the handler.
				ordinary := map[string]any{"probe": "agents", "query": "SELECT count(*) FROM resources WHERE resource_type='ranger-table-view'"}
				if res := callMCPTool(t, s, name, ordinary); res.IsError || res.Text != "handler ran" {
					t.Errorf("tool %s refused an ordinary call: %+v", name, res)
				}
			}
			if tools["sentinelone_execute"] != nil {
				before := ran["sentinelone_execute"]
				res := callMCPTool(t, s, "sentinelone_execute", map[string]any{"endpoint_id": "ranger.get-cred-group-details", "confirm": true, "params": map[string]any{"note": "agents-passphrases"}})
				if res.IsError || ran["sentinelone_execute"] != before+1 {
					t.Errorf("sentinelone_execute with confirm: true was refused: %+v", res)
				}
			}
		})
	}
}
