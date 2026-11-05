// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package rewstblackbox

import (
	"fmt"
	"strings"
	"testing"
)

// Class response-credentials-held, one documented Rewst credential at a
// time (the battery covers them all at once; this names the one that leaks):
// each answer that carries a credential reaches no output, from the CLI in
// JSON and table form and from MCP (rewst_read, or rewst_execute with
// confirm true for a GraphQL document); the credential kit withholds the
// whole answer, so each output is its notice. The fake echoes nothing here,
// so only a declared credential can make the kit withhold an answer (a
// typed write's --stdin body is held itself, so the CLI's --stdin answers
// are withheld either way). The credentials (spec.graphql):
// createOrganizationApiClient's ApiClientWithSecret auth0ClientSecret and
// auth0ClientId, rotateApiClientSecret's ApiClientSecretRotation
// auth0ClientSecret, the String answers of getCannyToken,
// getHaloLiveChatToken and getSkilljarLoginToken, an ApiClient's
// auth0ClientId (one client, a list of them, a User's tokenApiClient), and
// the integration credentials a PackConfig's config JSON holds under generic
// names (access_token, refresh_token, client_secret, encryption_key,
// password). An OrgVariable's value of category secret is the P2
// TestOrgVariableSecretValueHeldKnownP2.
func TestEveryDocumentedResponseCredentialIsHeld(t *testing.T) {
	budget(t)
	sv := echoServer(t)
	sv.quiet = true
	e := newEnv(t, sv, credEnv).lean()
	type tc struct {
		cred  []string
		stdin string // a GraphQL document the CLI sends through a typed write's --stdin
		args  []string
		mcp   map[string]any
	}
	create := []string{"organizations", "create", "--stdin", "--yes"}
	cases := []tc{
		{[]string{bbClientSecret, bbClientID}, gqlBody(mutCreateClient), create, confirmed(gqlExecute(mutCreateClient), true)},
		{[]string{bbRotated, bbClientID}, gqlBody(mutRotate), create, confirmed(gqlExecute(mutRotate), true)},
		{[]string{bbCannyTok}, gqlBody(qCanny), create, confirmed(gqlExecute(qCanny), true)},
		{[]string{bbHaloTok}, gqlBody(qHalo), create, confirmed(gqlExecute(qHalo), true)},
		{[]string{bbSkilljarTok}, gqlBody(qSkilljar), create, confirmed(gqlExecute(qSkilljar), true)},
		{[]string{bbClientID}, "", []string{"api-client-lists", "--org-id", bbOrgID}, read("api-client-lists.get", map[string]any{"orgId": bbOrgID})},
		{[]string{bbClientID}, "", []string{"users", "get"}, read("users.get", map[string]any{})},
		{[]string{bbGzipResp, bbRefreshResp, bbClientSecResp, bbEncKey, bbShortResp}, "", []string{"pack-configs", "get", "--pack-ids", bbPackID, "--org-id", bbOrgID},
			read("pack-configs.get", map[string]any{"orgId": bbOrgID})},
		// One API client by id: its answer also splits the held token across
		// adjacent values, lines and spaces.
		{[]string{bbClientID}, "", []string{"api-clients", bbAPIClientID}, read("api-clients.get", map[string]any{"id": bbAPIClientID})},
	}
	// One call per phase: mcp-go serves one phase's calls concurrently, so the
	// session's delivery history (and the by-design over-holding it can
	// trigger, a P2 in .review/followups.md) would depend on scheduling and
	// this test would pass or fail at random.
	var phases [][]map[string]any
	for _, c := range cases {
		phases = append(phases, []map[string]any{c.mcp})
	}
	replies := mcpReplies(e.mcpPhases(t, phases...))
	for i, c := range cases {
		// The default run: MCP answers every case; the CLI runs one --stdin answer
		// and the gzip-encoded pack config, in one form.
		forms := []string{"--json", ""}
		if !fullSweep {
			forms = forms[:1]
			if i != 0 && i != 7 {
				forms = nil
			}
		}
		for _, form := range forms {
			args := c.args
			if form != "" {
				args = append(append([]string{}, c.args...), form)
			}
			out := e.runIn(t, c.stdin, args...)
			if !kitWithheld(out) {
				t.Errorf("%v: the answer carrying a credential was not withheld:\n%.1500s", args, out)
			}
			for _, cred := range c.cred {
				for _, f := range echoForms(cred) {
					if strings.Contains(out, f) {
						t.Errorf("%v prints the credential %q as %q", args, cred, f)
						break
					}
				}
			}
			assertNoEcho(t, fmt.Sprintf("output of %v", args), out)
		}
		r := replies[i+1]
		if !kitWithheld(r) {
			t.Errorf("MCP %v: the answer carrying a credential was not withheld:\n%.1500s", c.mcp["arguments"], r)
		}
		for _, cred := range c.cred {
			for _, f := range echoForms(cred) {
				if strings.Contains(r, f) {
					t.Errorf("MCP %v returns the credential %q as %q", c.mcp["arguments"], cred, f)
					break
				}
			}
		}
		assertNoEcho(t, fmt.Sprintf("MCP %v", c.mcp["arguments"]), r)
	}
	checkOneAuth(t, sv, credEnv)
}
