// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package rewstblackbox

// Codex review round 1 (liongard 4.34.2 reprint, the shared credential layer)
// class holder-incomplete, end to end on rewst: a credential that is a JSON
// number under a credential name (the liongard input was
// {"password":12345678}) was printed by a preview. The Rewst forms of the
// class are a credential-named query value the user sends as digits
// (sync --param password=48261937, previewed and sent) and a number under a
// credential name in an answer (a user's password, api_key, token and
// secret; a page's password; a pack config's secret and api_key,
// gzip-encoded; an API client's auth0ClientSecret as a GraphQL answer), in
// every output format and over MCP. The credential kit withholds a whole
// answer that carries one. (A secret org variable's value sent as a number
// is the P2 TestOrgVariableSecretValueHeldKnownP2.)

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestNumericCredentialValuesNeverPrint(t *testing.T) {
	budget(t)
	const named = "48261937" // under a credential name (password, api_key, token, secret)
	sv := shapeServer(t, func(s shapeReq) (string, bool) {
		switch {
		case s.field() == "createOrganizationApiClient":
			return fmt.Sprintf(`{"createOrganizationApiClient":{"id":%q,"name":"HQ numeric client","auth0ClientSecret":%s}}`, bbAPIClientID, named), true
		case s.g.isDoc():
			return "", false
		case s.r.Method == http.MethodGet:
			return fmt.Sprintf(`{"getTestUserSession":{"id":%q,"username":"HQ numeric user","orgId":%q,"activeSessionCount":3,`+
				`"password":%s,"api_key":%s,"token":%s,"secret":%s},`+
				`"pages":[{"id":%q,"name":"HQ numeric page","password":%s}],`+
				`"packConfigsForOrg":[{"id":%q,"name":"HQ numeric pack","config":{"secret":%s,"api_key":%s}}]}`,
				bbUserID, bbOrgID, named, named, named, named, bbPageID, named, bbPackConfigID, named, named), true
		}
		return "", false
	})
	e := newEnv(t, sv, credEnv).lean()
	var out strings.Builder
	reads := [][]string{
		{"users", "get", "--json"},
		{"users", "get", "--csv"},
		{"users", "get", "--plain"},
		{"users", "get"},
		{"pages", "get", "--json"},
		{"pack-configs", "get", "--pack-ids", bbPackID, "--org-id", bbOrgID, "--json"},
		{"pack-configs", "get", "--pack-ids", bbPackID, "--org-id", bbOrgID, "--csv"},
		{"pack-configs", "get", "--pack-ids", bbPackID, "--org-id", bbOrgID},
		{"sync", "--resources", "pages", "--param", "password=" + named, "--dry-run"},
		{"sync", "--resources", "pages", "--param", "password=" + named, "--json"},
		{"search", "HQ", "--json"},
	}
	if !fullSweep {
		reads = [][]string{reads[0], reads[8]} // an answer and a preview; MCP below runs every kind
	}
	for _, args := range reads {
		out.WriteString(e.run(t, args...))
	}
	out.WriteString(e.runIn(t, gqlBody(mutCreateClient), "organizations", "create", "--stdin", "--yes", "--json"))
	out.WriteString(e.mcpCalls(t, []map[string]any{
		read("users.get", map[string]any{}),
		read("pages.get", map[string]any{}),
		read("pack-configs.get", map[string]any{"orgId": bbOrgID}),
		tool("sync", map[string]any{"resources": []any{"pages"}, "param": []any{"password=" + named}, "dry-run": true}),
		confirmed(gqlExecute(mutCreateClient), true),
	}))
	got := out.String()
	// The preview's own line repeats the held value, so it is withheld whole;
	// the sync's event says the dry run happened.
	if !strings.Contains(got, "sync_dryrun") || !kitWithheld(got) {
		t.Fatalf("the reads and previews did not run:\n%.3000s", got)
	}
	if strings.Contains(got, named) {
		t.Errorf("output repeats the numeric credential %s:\n%.4000s", named, got)
	}
	var sent bool
	for _, m := range sv.allMethods() {
		sent = sent || strings.HasPrefix(m, "GET /graphql")
	}
	if !sent {
		t.Errorf("the sync never reached the fake; requests %v", sv.allMethods())
	}
}
