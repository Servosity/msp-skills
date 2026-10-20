// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/dnsfilter/handfixes.json
// under the "mcp-execute-notes-resource-placeholder" entry.

package mcp

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"dnsfilter-pp-cli/internal/cliutil"
)

// TestCodeOrchExecuteFillsNotesResource: the five notes endpoints substitute
// {resource} from params (default policies, as the CLI's --resource does);
// the read-only resolved-path check sees the filled value.
func TestCodeOrchExecuteFillsNotesResource(t *testing.T) {
	var mu sync.Mutex
	var hits []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.ReadAll(r.Body)
		mu.Lock()
		hits = append(hits, r.Method+" "+r.URL.EscapedPath())
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"note":"ok"}}`)
	}))
	t.Cleanup(srv.Close)
	setIsolatedHome(t, srv.URL)
	sent := func() []string {
		mu.Lock()
		defer mu.Unlock()
		out := append([]string{}, hits...)
		hits = nil
		return out
	}

	t.Setenv(cliutil.MCPReadOnlyEnvVar, "")
	s := newRegisteredServer(t)
	cases := []struct {
		id     string
		params map[string]any
		want   string
	}{
		{"notes.domain-show", map[string]any{"id": 7, "domain": "example.com"}, "GET /v1/notes/policies/7/example.com"},
		{"notes.domain-show", map[string]any{"id": 7, "domain": "example.com", "resource": "msps"}, "GET /v1/notes/msps/7/example.com"},
		{"notes.domain-update", map[string]any{"id": 7, "domain": "example.com", "resource": "organizations", "note": "x"}, "PATCH /v1/notes/organizations/7/example.com"},
		{"notes.batch-update.domain-notes", map[string]any{"id": 7}, "PATCH /v1/notes/policies/7/batch_update"},
		{"notes.notes.domain-batch-destroy", map[string]any{"id": 7, "domains": "a.example"}, "DELETE /v1/notes/policies/7/notes/batch_destroy"},
		{"notes.notes.domain-destroy", map[string]any{"id": 7, "domain": "example.com"}, "DELETE /v1/notes/policies/7/notes/example.com"},
	}
	for _, c := range cases {
		frame := callToolWire(t, s, "dnsfilter_execute", map[string]any{"endpoint_id": c.id, "params": c.params})
		got := sent()
		if len(got) != 1 || got[0] != c.want {
			t.Fatalf("%s %v sent %v, want [%s] (%s)", c.id, c.params, got, c.want, frame)
		}
	}

	t.Setenv(cliutil.MCPReadOnlyEnvVar, "1")
	s = newRegisteredServer(t)
	frame := callToolWire(t, s, "dnsfilter_execute", map[string]any{"endpoint_id": "notes.domain-show", "params": map[string]any{"id": 7, "domain": "example.com", "resource": "api_keys"}})
	if !strings.Contains(frame, "refusing GET notes.domain-show") || len(sent()) != 0 {
		t.Fatalf("read-only did not refuse a credential resource on the resolved notes path: %s", frame)
	}
	// The MCP layer does not treat a hostname path segment as a credential
	// (internal/client keeps its own, stricter backstop).
	if msg := codeOrchResolvedPathRefusal(findCodeOrchEndpoint("notes.domain-show"), "/v1/notes/policies/7/secret-key.io"); msg != "" {
		t.Fatalf("MCP refused a hostname notes path: %s", msg)
	}
	if msg := codeOrchResolvedPathRefusal(findCodeOrchEndpoint("notes.domain-show"), "/v1/notes/api_keys/7/example.com"); msg == "" {
		t.Fatal("MCP let a credential resource through on the resolved notes path")
	}
}
