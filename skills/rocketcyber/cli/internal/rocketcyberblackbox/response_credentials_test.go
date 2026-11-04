// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package rocketcyberblackbox

import (
	"fmt"
	"strings"
	"testing"
)

// The answers that carry credential-named members, one at a time: the
// account's integration OAuth access_token, refresh_token, client_secret,
// encryption_key and username (gzip-encoded), every agent's password (a
// 7-character value, gzip-encoded list), an incident's api_key and
// credentials container (BOM-prefixed list), a firewall's secret and token,
// and an app's access_token. RocketCyber documents none of them and the
// connector declares no response field, so the contract holds none
// (outOfContract; the connector's own response-credentials-held layer is
// retired, handfixes.json credkit-wiring). Each answer, from the CLI in JSON
// and table form and from MCP: prints its public data when it repeats nothing
// it was sent (the quiet fake), and when the fake echoes the held token
// beside those members, no held credential reaches the output. Each MCP read
// runs in its own session: in one session an answer that repeats credentials
// can make later answers withheld whole (the P2
// TestMCPSessionEchoWithholdsALaterAnswerKnownP2).
func TestCredentialNamedResponseMembers(t *testing.T) {
	sv := echoServer(t)
	e := newEnv(t, sv, credEnv)
	quiet := quietServer(t)
	q := newEnv(t, quiet, credEnv)
	cases := []struct {
		args   []string
		mcp    map[string]any
		public string
	}{
		{[]string{"account"}, tool("account_get", map[string]any{}), bbAccountName},
		{[]string{"agents"}, tool("agents_list", map[string]any{}), bbHostname},
		{[]string{"incidents"}, tool("incidents_list", map[string]any{}), bbIncTitle},
		{[]string{"firewalls"}, tool("firewalls_list", map[string]any{}), "sonicwall"},
		{[]string{"apps"}, tool("apps_list", map[string]any{}), "Advanced Breach Detection"},
	}
	for _, c := range cases {
		for _, form := range []string{"--json", ""} {
			args := c.args
			if form != "" {
				args = append(append([]string{}, c.args...), form)
			}
			if out := q.run(t, args...); !strings.Contains(out, c.public) {
				t.Errorf("%v: the answer's public data (%q) did not print:\n%.1500s", args, c.public, out)
			}
			assertNoEcho(t, fmt.Sprintf("output of %v", args), e.run(t, args...))
		}
		if r := mcpReplies(q.mcpCalls(t, []map[string]any{c.mcp}))[1]; !strings.Contains(r, c.public) {
			t.Errorf("MCP %s: the answer's public data (%q) did not print:\n%.1500s", c.mcp["name"], c.public, r)
		}
		assertNoEcho(t, fmt.Sprintf("MCP %s", c.mcp["name"]), e.mcpCalls(t, []map[string]any{c.mcp}))
	}
	checkOneAuth(t, sv, credEnv)
}
