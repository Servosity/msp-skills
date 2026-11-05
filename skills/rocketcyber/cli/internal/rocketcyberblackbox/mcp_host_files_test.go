// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package rocketcyberblackbox

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Threat-model class host-file-read-from-mcp-argument: no MCP tool argument
// makes the server read (and send) or write a file on the host. RocketCyber
// has no upload endpoint; the CLI's file-taking flags (export's --output,
// teach's --playbook-file and --playbook-notes-file, teach-playbook's
// --notes-file and --playbook-file, every --db, and the global --config,
// --home, --audit-dir, --deliver file: and --ca-cert) are not tool arguments.
// A file in the server's home holding a marker is named every way; each call
// naming a host path is refused, the marker never reaches the fake or a
// result, and nothing is written where the caller named.
func TestMCPCannotReadOrWriteAHostFile(t *testing.T) {
	sv := echoServer(t)
	e := newEnv(t, sv, credEnv)
	const marker = "HQ-HostFile-Marker-7c1e9a"
	host := filepath.Join(e.home, "host-note.txt")
	if err := os.WriteFile(host, []byte(marker+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	written := filepath.Join(e.home, "written-by-mcp.json")
	auditDir := filepath.Join(e.home, "audit-by-mcp")
	otherHome := filepath.Join(e.home, "home-by-mcp")
	teach := map[string]any{"query": "find", "resource-type": "agents", "resource": []any{bbAgentID}}
	with := func(base map[string]any, k string, v any) map[string]any {
		out := map[string]any{k: v}
		for bk, bv := range base {
			out[bk] = bv
		}
		return out
	}
	calls := []map[string]any{
		tool("export", map[string]any{"resource": "agents", "output": written}),
		tool("export", map[string]any{"resource": "agents", "deliver": "file:" + written}),
		tool("teach", with(teach, "playbook-file", host)),
		tool("teach", with(teach, "playbook-notes-file", host)),
		tool("teach_playbook", map[string]any{"query": "find", "notes-file": host}),
		tool("teach_playbook", map[string]any{"query": "find", "playbook-file": host}),
		tool("sync", map[string]any{"resources": []any{"apps"}, "db": written}),
		tool("analytics", map[string]any{"type": "agents", "db": host}),
		tool("agents_list", map[string]any{"config": host}),
		tool("triage", map[string]any{"audit-dir": auditDir}),
		tool("agents_stale", map[string]any{"home": otherHome}),
		tool("account_get", map[string]any{"ca-cert": host}),
		tool("incidents_mttr", map[string]any{"config": host}),
	}
	out := e.mcpCalls(t, calls)
	if !strings.Contains(out, fmt.Sprintf(`"id":%d`, len(calls))) {
		t.Fatalf("MCP session did not answer every call:\n%.2000s", out)
	}
	replies := mcpReplies(out)
	for i, c := range calls {
		if r := replies[i+1]; !strings.Contains(r, `"isError":true`) && !strings.Contains(r, `"error":`) {
			t.Errorf("%s %v names a host path and was not refused:\n%.1000s", c["name"], c["arguments"], r)
		}
	}
	for _, m := range sv.allMethods() {
		if isWrite(m) {
			t.Errorf("a call naming a host file sent %s", m)
		}
	}
	if strings.Contains(out, marker) {
		t.Errorf("an MCP result carries the host file's content:\n%.2000s", out)
	}
	if sv.received(marker) {
		t.Errorf("a request carried the host file's content")
	}
	for _, p := range []string{written, auditDir, otherHome} {
		if _, err := os.Stat(p); err == nil {
			t.Errorf("an MCP call wrote %s for its caller", p)
		}
	}
}
