// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/dnsfilter/handfixes.json
// under the "mcp-tools-refuse-undeclared-arguments" entry.

package mcp

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"testing"

	"dnsfilter-pp-cli/internal/cliutil"
	mcplib "github.com/mark3labs/mcp-go/mcp"
)

// TestEveryToolRefusesUndeclaredTopLevelArguments sends each of dry_run,
// dry-run, confirm and force (whichever a tool does not declare) as a
// top-level argument to EVERY registered tool, next to otherwise valid
// arguments, and requires a refusal with no API request and no companion CLI
// run. The reported shape: dnsfilter_execute networks.destroy with a
// top-level dry_run used to send DELETE /v1/networks/7.
func TestEveryToolRefusesUndeclaredTopLevelArguments(t *testing.T) {
	var hits int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&hits, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	t.Cleanup(srv.Close)
	setIsolatedHome(t, srv.URL)
	marker := filepath.Join(t.TempDir(), "cli-ran")
	t.Setenv("DNSFILTER_CLI_PATH", os.Args[0])
	t.Setenv(fakeCLIModeEnv, "marker")
	t.Setenv("DNSFILTER_MCP_TEST_MARKER", marker)
	t.Setenv(cliutil.MCPReadOnlyEnvVar, "")
	savedPath, savedErr := recipeCLIPath, recipeCLIPathErr
	t.Cleanup(func() { recipeCLIPath, recipeCLIPathErr = savedPath, savedErr })
	recipeCLIPath, recipeCLIPathErr = os.Args[0], nil

	s := newRegisteredServer(t)
	base := map[string]map[string]any{
		"dnsfilter_execute": {"endpoint_id": "networks.destroy", "params": map[string]any{"id": 7}},
		"dnsfilter_get":     {"endpoint_id": "networks.index"},
		"dnsfilter_search":  {"query": "networks"},
		"sql":               {"query": "SELECT 1"},
		"search":            {"query": "site"},
		"context":           {},
		"precedent_check_before_allowing_a_domain": {"domain": "example.com"},
		"tail": {"resource": "networks", "follow": false},
	}
	names := make([]string, 0, len(s.ListTools()))
	for name := range s.ListTools() {
		names = append(names, name)
	}
	sort.Strings(names)
	checked := 0
	for _, name := range names {
		declared := declaredToolProperties(s.GetTool(name).Tool)
		for _, extra := range []string{"dry_run", "dry-run", "confirm", "force"} {
			if declared[extra] {
				continue
			}
			args := map[string]any{}
			for k, v := range base[name] {
				args[k] = v
			}
			args[extra] = true
			frame := callToolWire(t, s, name, args)
			if !strings.Contains(frame, `unknown MCP parameter \"`+extra+`\"`) {
				t.Errorf("%s with top-level %q was not refused: %.300s", name, extra, frame)
			}
			checked++
		}
	}
	if checked < 4*len(base) {
		t.Fatalf("only %d tool/argument pairs were checked", checked)
	}
	if n := atomic.LoadInt64(&hits); n != 0 {
		t.Fatalf("a refused call reached the API %d times", n)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("a refused call ran the companion CLI")
	}
}

// TestArgumentGateKeepsDeclaredArguments: declared arguments pass (a mirror
// tool's own --confirm / --dry-run flags, execute's params), and non-object
// arguments are refused.
func TestArgumentGateKeepsDeclaredArguments(t *testing.T) {
	t.Setenv(cliutil.MCPReadOnlyEnvVar, "")
	s := newRegisteredServer(t)
	ok := map[string]map[string]any{
		"allow":             {"confirm": true, "force": false, "dry-run": true, "fqdn": "example.com"},
		"dnsfilter_execute": {"endpoint_id": "networks.index", "params": map[string]any{"dry_run": true}},
		"context":           nil,
	}
	for name, args := range ok {
		req := mcplib.CallToolRequest{Params: mcplib.CallToolParams{Name: name}}
		if args != nil {
			req.Params.Arguments = args
		}
		if msg := undeclaredArgumentRefusal(s, req); msg != "" {
			t.Fatalf("%s refused declared arguments %v: %s", name, args, msg)
		}
	}
	req := mcplib.CallToolRequest{Params: mcplib.CallToolParams{Name: "sql", Arguments: []any{"SELECT 1"}}}
	if msg := undeclaredArgumentRefusal(s, req); msg == "" {
		t.Fatal("non-object arguments were accepted")
	}
}
