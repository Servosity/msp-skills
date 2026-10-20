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
	"strings"
	"sync"
	"testing"

	"dnsfilter-pp-cli/internal/cliutil"
)

// TestToolArgumentsMustHaveTheirDeclaredType: an argument whose declared type
// is object, array, number or boolean is refused when it arrives as another
// JSON type, a required argument cannot be null, and dnsfilter_execute
// refuses a path placeholder left unfilled; nothing reaches the API or the
// companion CLI. The reported shape: params given as the STRING
// "{\"dry_run\":true}" for organizations.promote-to-msp sent the live POST.
func TestToolArgumentsMustHaveTheirDeclaredType(t *testing.T) {
	var mu sync.Mutex
	var hits []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hits = append(hits, r.Method+" "+r.URL.Path)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{}}`))
	}))
	t.Cleanup(srv.Close)
	setIsolatedHome(t, srv.URL)
	marker := filepath.Join(t.TempDir(), "cli-ran")
	t.Setenv("DNSFILTER_CLI_PATH", os.Args[0])
	t.Setenv(fakeCLIModeEnv, "marker")
	t.Setenv("DNSFILTER_MCP_TEST_MARKER", marker)
	t.Setenv(cliutil.MCPReadOnlyEnvVar, "")
	s := newRegisteredServer(t)
	sent := func() []string {
		mu.Lock()
		defer mu.Unlock()
		out := append([]string{}, hits...)
		hits = nil
		return out
	}

	refused := []struct {
		tool string
		args map[string]any
		want string
	}{
		{"dnsfilter_execute", map[string]any{"endpoint_id": "organizations.promote-to-msp", "params": `{"dry_run":true}`}, `"params" for dnsfilter_execute must be a JSON object, not a string`},
		{"dnsfilter_execute", map[string]any{"endpoint_id": "organizations.promote-to-msp", "params": []any{"dry_run"}}, "must be a JSON object, not an array"},
		{"dnsfilter_execute", map[string]any{"endpoint_id": "organizations.promote-to-msp", "params": 1}, "must be a JSON object, not a number"},
		{"dnsfilter_execute", map[string]any{"endpoint_id": "organizations.promote-to-msp", "params": true}, "must be a JSON object, not a boolean"},
		{"dnsfilter_execute", map[string]any{"endpoint_id": "networks.destroy", "params": nil}, `needs the path parameter "id"`},
		{"dnsfilter_execute", map[string]any{"endpoint_id": "networks.destroy"}, `needs the path parameter "id"`},
		{"dnsfilter_execute", map[string]any{"endpoint_id": nil}, "is required and cannot be null"},
		{"tail", map[string]any{"resource": "networks", "follow": "false"}, `"follow" for tail must be a JSON boolean, not a string`},
		{"search", map[string]any{"query": "x", "limit": "25"}, "must be a JSON number, not a string"},
		{"sql", map[string]any{"query": nil}, "is required and cannot be null"},
	}
	for _, c := range refused {
		frame := callToolWire(t, s, c.tool, c.args)
		if !strings.Contains(frame, strings.ReplaceAll(c.want, `"`, `\"`)) || !strings.Contains(frame, "nothing was") {
			t.Errorf("%s %v was not refused with %q: %.300s", c.tool, c.args, c.want, frame)
		}
	}
	if got := sent(); len(got) != 0 {
		t.Fatalf("a refused call reached the API: %v", got)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("a refused call ran the companion CLI")
	}

	// Controls: correctly typed calls still run (a null optional params on an
	// endpoint without placeholders is the same as no params).
	callToolWire(t, s, "dnsfilter_execute", map[string]any{"endpoint_id": "organizations.promote-to-msp", "params": nil})
	callToolWire(t, s, "dnsfilter_execute", map[string]any{"endpoint_id": "networks.destroy", "params": map[string]any{"id": 7}})
	if got := sent(); len(got) != 2 || got[0] != "POST /v1/organizations/promote_to_msp" || got[1] != "DELETE /v1/networks/7" {
		t.Fatalf("correctly typed calls did not run: %v", got)
	}
}
