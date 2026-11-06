// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package dsblackbox

// Codex review round 1 class read-call-reaches-a-mint-route: a read (a tool
// or command marked read-only, or any call not issued as a mutation) whose
// path argument names a sign-in link route never reaches it, from the MCP
// server or the CLI; only a call issued as a mutation (the confirmed endpoint
// tools, the CLI commands that mint) does.

import (
	"strings"
	"testing"
)

func TestNoReadReachesASignInLinkRoute(t *testing.T) {
	sv := echoServer(t)
	e := newEnv(t, sv, true)
	var calls []map[string]any
	for _, id := range []string{"generate_redirect_link", "generate_support_sso_link", "generate_redirect_link.json",
		"GENERATE_REDIRECT_LINK", "../users/generate_redirect_link", "%67enerate_redirect_link"} {
		calls = append(calls,
			map[string]any{"name": "organizations_get", "arguments": map[string]any{"id": id}},
			map[string]any{"name": "organizations_list-tenants", "arguments": map[string]any{"id": id}},
			map[string]any{"name": "sharepoint_sites_list", "arguments": map[string]any{"domain_id": id}})
	}
	out := e.mcpCalls(t, calls)
	cli := []string{
		e.run(t, "organizations", "get", "generate_redirect_link", "--json"),
		e.run(t, "organizations", "get", "generate_support_sso_link", "--json"),
		e.run(t, "organizations", "list-tenants", "generate_redirect_link", "--json"),
		e.run(t, "sharepoint-sites", "../../users/generate_redirect_link", "--org", bbOrg, "--json"),
	}
	sv.mu.Lock()
	minted := sv.ssoMint
	paths := strings.Join(sv.paths, " ")
	sv.mu.Unlock()
	if minted != 0 {
		t.Fatalf("reads reached a sign-in link route %d time(s) (paths: %s)\nMCP:\n%.3000s\nCLI:\n%.3000s", minted, paths, out, strings.Join(cli, "\n"))
	}
	if !strings.Contains(paths, "/api/users") {
		t.Fatalf("the reads never reached the API at all; the test did not run: %s", paths)
	}
	// A call issued as a mutation still mints: the CLI command that mints a
	// link, and the confirmed MCP endpoint tool.
	got := e.run(t, "portal", "get-login-link", "--org", bbOrg, "--json")
	e.mcpCalls(t, []map[string]any{{"name": "organizations_get-support-sso-link", "arguments": map[string]any{"email": "owner@example.com", "confirm": true}}})
	sv.mu.Lock()
	minted = sv.ssoMint
	sv.mu.Unlock()
	if minted != 2 || !strings.Contains(got, bbSSOLink) {
		t.Fatalf("the two mutation calls minted %d link(s), want 2; CLI output:\n%.2000s", minted, got)
	}
}
