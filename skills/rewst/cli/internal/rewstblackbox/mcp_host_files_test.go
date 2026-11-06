// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package rewstblackbox

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Threat-model class host-file-read-from-mcp-argument: no MCP tool argument
// makes the server read (and send) or write a file on the host. Rewst has no
// upload endpoint: a path given as an endpoint parameter (even with confirm
// true, even as file:// or curl's @file) is sent as the string it is, never
// as the file's content; the mirrored tools' path flags (import's --input,
// teach's --playbook-file and --playbook-notes-file, sync's and workflow
// archive's --db, a receipt file, an audit dir) are not tool arguments. A
// file in the server's home holding a marker is named every way; the marker
// never reaches the fake or a result, and nothing is written where a call
// named.
func TestMCPCannotReadOrWriteAHostFile(t *testing.T) {
	budget(t)
	sv := echoServer(t)
	e := newEnv(t, sv, credEnv)
	const marker = "HQ-HostFile-Marker-7c1e9a"
	host := filepath.Join(e.home, "host-note.txt")
	if err := os.WriteFile(host, []byte(marker+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	sv.mu.Lock()
	sv.watch = marker
	sv.mu.Unlock()
	written := filepath.Join(e.home, "written-by-mcp.db")
	receipt := filepath.Join(e.home, "receipt-by-mcp.json")
	audit := filepath.Join(e.home, "audit-by-mcp")
	blocked := []map[string]any{
		tool("import", map[string]any{"resource": "pages", "input": host, "confirm": true}),
		tool("teach", map[string]any{"query": "find", "resource-type": "pages", "resource": []any{bbPageID}, "playbook-file": host}),
		tool("teach", map[string]any{"query": "find", "resource-type": "pages", "resource": []any{bbPageID}, "playbook-notes-file": host}),
		tool("sync", map[string]any{"resources": []any{"pages"}, "db": written}),
		tool("workflow_archive", map[string]any{"db": written}),
		tool("sync", map[string]any{"resources": []any{"pages"}, "receipt-file": receipt}),
		tool("health", map[string]any{"org": bbOrgID, "audit-dir": audit}),
	}
	sent := []map[string]any{
		confirmed(execute("org-variables.create", map[string]any{"name": "hq_file", "value": host, "category": "general", "orgId": bbOrgID}), true),
		confirmed(execute("org-variables.create", map[string]any{"name": "hq_file", "value": "file://" + host, "category": "general", "orgId": bbOrgID}), true),
		confirmed(execute("pages.create", map[string]any{"name": "@" + host, "path": "/file", "orgId": bbOrgID, "file": host}), true),
		confirmed(execute("organizations.create", map[string]any{"query": "query { getCannyToken }", "variables": map[string]any{"file": host}}), true),
	}
	out := e.mcpCalls(t, append(append([]map[string]any{}, blocked...), sent...))
	if !strings.Contains(out, `"id":11`) {
		t.Fatalf("MCP session did not answer every call:\n%.2000s", out)
	}
	replies := mcpReplies(out)
	for i := range blocked {
		if r := replies[i+1]; !strings.Contains(r, `"isError":true`) {
			t.Errorf("call %d named a host path in a blocked argument and was not refused:\n%.1000s", i+1, r)
		}
	}
	sv.mu.Lock()
	hits := sv.watchHits
	sv.mu.Unlock()
	if hits != 0 {
		t.Errorf("%d request(s) carried the host file's content to the API", hits)
	}
	var writes int
	for _, m := range sv.allMethods() {
		if isWrite(m) {
			writes++
		}
	}
	if writes < 3 {
		t.Errorf("the confirmed writes naming a path did not reach the API as strings (%d writes): %v", writes, sv.allMethods())
	}
	if strings.Contains(out, marker) {
		t.Errorf("an MCP result carries the host file's content:\n%.2000s", out)
	}
	for _, p := range []string{written, receipt, audit} {
		if _, err := os.Stat(p); err == nil {
			t.Errorf("a tool wrote %s for its caller", p)
		}
	}
}
