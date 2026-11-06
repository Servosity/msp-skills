// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package rewstblackbox

// Codex review round 2 (domotz 4.34.2 reprint) class holder-incomplete /
// whitespace-only credential: a credential made only of whitespace cannot be
// held (the text funnel trims it to nothing), and the output redaction kept it
// as written because it read the trimmed value as the program's own empty
// marker. A whitespace value under a credential name must never print: in
// an answer Rewst returns (the API client createOrganizationApiClient makes,
// its auth0ClientSecret and auth0ClientId; a pack config's integration
// access_token and encryption_key, gzip-encoded; a user's password and
// api_key), in JSON, CSV, plain and table output and MCP results, where the
// credential kit withholds the whole answer; and a credential-named query
// value the user sends (sync --param password=" \t "), in its --dry-run
// preview. A credential pair's value (a member named value beside one that
// says password or secret: a user's custom field, a secret org variable) is
// the P2 TestOrgVariableSecretValueHeldKnownP2: the kit holds a field by its
// own name only.

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

// whitespaceCredentials: a run like Codex's community, a distinctive run of
// tabs (found raw in CSV and plain output too), spaces only.
var whitespaceCredentials = []string{" \t ", "\t \t\t  \t", "      "}

// wsCredMember is a credential-named JSON member whose value is only
// whitespace (raw or JSON-escaped).
var wsCredMember = regexp.MustCompile(`"(password|api_key|token|secret|encryption_key|access_token|auth0ClientSecret|auth0ClientId)"\s*:\s*"((?:\\[tnr]|[ \t\r\n])+)"`)

// wsPairValue is a credential pair's value member (name password, or an org
// variable's category secret) that is only whitespace, in either member
// order.
var wsPairValue = regexp.MustCompile(`"(name|label|category)"\s*:\s*"(password|secret)"[^{}]*"value"\s*:\s*"((?:\\[tnr]|[ \t\r\n])+)"|"value"\s*:\s*"((?:\\[tnr]|[ \t\r\n])+)"[^{}]*"(name|label|category)"\s*:\s*"(password|secret)"`)

// wsQueryValue is a password query value that is only whitespace, in a
// request preview (raw, percent- or plus-encoded).
var wsQueryValue = regexp.MustCompile(`password=(?:%20|%09|\+|[ \t])+(?:&|\s|$)`)

// unescapeOnce reads one level of JSON string escaping, so a JSON document
// carried as a JSON string (an MCP result in its JSON-RPC message) is checked
// too.
var unescapeOnce = strings.NewReplacer(`\\`, `\`, `\"`, `"`)

func whitespaceCredentialPrinted(out, v string) string {
	for _, text := range []string{out, unescapeOnce.Replace(out), unescapeOnce.Replace(unescapeOnce.Replace(out))} {
		for _, re := range []*regexp.Regexp{wsCredMember, wsPairValue, wsQueryValue} {
			if m := re.FindString(text); m != "" {
				return m
			}
		}
	}
	if strings.Count(v, "\t") >= 2 {
		esc := strings.ReplaceAll(v, "\t", `\t`)
		for _, form := range []string{v, esc, strings.ReplaceAll(esc, `\`, `\\`)} {
			if strings.Contains(out, form) {
				return fmt.Sprintf("%q", form)
			}
		}
	}
	return ""
}

// shapeReq is one request a shapeServer answers: its query and what its body
// asked (a GraphQL document, or a REST-shaped body).
type shapeReq struct {
	r *http.Request
	q url.Values
	g gqlRequest
}

// field is the first top-level field the request's GraphQL document names,
// "" for a REST-shaped request.
func (s shapeReq) field() string {
	if len(s.g.fields) == 0 {
		return ""
	}
	return s.g.fields[0].name
}

// shapeServer starts a Rewst fake (TLS) answering each request to /graphql
// with the data object docs returns ({"data": <it>}), or a not-found error
// when docs has none. A REST-shaped read that names orgId (pack configs) is
// answered gzip-encoded, and one that names visibleForOrgId (org variables)
// BOM-prefixed. It records the requests and the tokens it received.
func shapeServer(t *testing.T, docs func(s shapeReq) (data string, ok bool)) *server {
	t.Helper()
	sv := newServer()
	srv := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		g := readGQL(body)
		sv.mu.Lock()
		sv.methods = append(sv.methods, r.Method+" "+r.URL.Path+" "+g.label())
		sv.countAuth(r)
		sv.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		q := r.URL.Query()
		data, ok := "", false
		if r.URL.Path == "/graphql" {
			data, ok = docs(shapeReq{r: r, q: q, g: g})
		}
		switch {
		case !ok:
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, `{"errors":[{"message":"Not Found"}]}`)
		case r.Method == http.MethodGet && q.Has("orgId"):
			gzipped(w, `{"data":`+data+`}`)
		case r.Method == http.MethodGet && q.Has("visibleForOrgId"):
			fmt.Fprint(w, "\xef\xbb\xbf"+`{"data":`+data+`}`)
		default:
			fmt.Fprint(w, `{"data":`+data+`}`)
		}
	}))
	sv.url = srv.URL
	sv.caFile = caFileFor(t, srv)
	return sv
}

// wsResponseServer answers a user, a pack config, the org variables and a
// new API client with v as every credential.
func wsResponseServer(t *testing.T, v string) *server {
	t.Helper()
	q := fmt.Sprintf("%q", v)
	return shapeServer(t, func(s shapeReq) (string, bool) {
		switch {
		case s.field() == "createOrganizationApiClient":
			return fmt.Sprintf(`{"createOrganizationApiClient":{"id":%q,"name":"HQ-WS client","orgId":%q,"auth0ClientId":%s,"auth0ClientSecret":%s}}`,
				bbAPIClientID, bbOrgID, q, q), true
		case s.g.isDoc():
			return "", false
		case s.r.Method == http.MethodGet:
			return fmt.Sprintf(`{"getTestUserSession":{"id":%q,"username":"HQ-WS user","orgId":%q,"password":%s,"api_key":%s,`+
				`"customFields":[{"name":"badge","value":"HQ-WS badge 77"}]},`+
				`"packConfigsForOrg":[{"id":%q,"name":"HQ-WS pack","orgId":%q,"config":{"tenant_domain":"hq-ws.onmicrosoft.com","access_token":%s,"encryption_key":%s}}],`+
				`"visibleOrgVariables":[{"id":"ws-general","name":"ticket_board_url","category":"general","value":"HQ-WS general"}]}`,
				bbUserID, bbOrgID, q, q, bbPackConfigID, bbOrgID, q, q), true
		}
		return "", false
	})
}

func TestWhitespaceCredentialsNeverPrint(t *testing.T) {
	budget(t)
	for i, v := range whitespaceCredentials {
		v := v
		t.Run(fmt.Sprintf("value-%d", i), func(t *testing.T) {
			spacesOnly := strings.Trim(v, " ") == ""
			// The default run: the first value runs every shape in one output form
			// and MCP; the tab run one answer; spaces only a preview and one answer.
			// A session under a spaces-only value sits out its wait, its replies
			// withheld.
			light := !fullSweep && i > 0
			forms := func(all [][]string) [][]string {
				if fullSweep {
					return all
				}
				return all[:1]
			}
			check := func(where, out, progress string) {
				t.Helper()
				// An answer carrying a credential is withheld whole. Once a value
				// of spaces only is held, every later unit and its notice can be
				// withheld too: the run stops printing at no fixed point and ends
				// in credkit.ExitOutputLost, 74 (kit note in followups), so only
				// the requests the fake saw prove that run (end of this subtest).
				if !spacesOnly && !strings.Contains(out, progress) && !kitWithheld(out) {
					t.Fatalf("%s did not run (no %q):\n%.3000s", where, progress, out)
				}
				if m := whitespaceCredentialPrinted(out, v); m != "" {
					t.Errorf("%s prints the whitespace credential %q (%s):\n%.3000s", where, v, m, out)
				}
			}
			rs := wsResponseServer(t, v)
			r := newEnv(t, rs, credEnv).lean()
			// Requests: a credential-named query value the user sends,
			// previewed and never sent.
			if fullSweep || i != 1 {
				check("CLI sync --param --dry-run", r.run(t, "sync", "--resources", "pages", "--param", "password="+v, "--dry-run"), "dry run")
			}
			if fullSweep {
				check("MCP sync --param dry-run", r.mcpCalls(t, []map[string]any{
					tool("sync", map[string]any{"resources": []any{"pages"}, "param": []any{"password=" + v}, "dry-run": true}),
				}), "dry")
			}
			if m := rs.allMethods(); len(m) != 0 {
				t.Errorf("a preview sent %v", m)
			}
			// Responses: what Rewst returns, in every output format.
			for _, args := range forms([][]string{
				{"users", "get", "--json"},
				{"users", "get", "--csv"},
				{"users", "get", "--plain"},
				{"users", "get"},
			}) {
				check(strings.Join(args, " "), r.run(t, args...), "HQ-WS")
			}
			if !light {
				for _, args := range forms([][]string{
					{"pack-configs", "get", "--pack-ids", bbPackID, "--org-id", bbOrgID, "--json"},
					{"pack-configs", "get", "--pack-ids", bbPackID, "--org-id", bbOrgID, "--csv"},
					{"pack-configs", "get", "--pack-ids", bbPackID, "--org-id", bbOrgID},
				}) {
					check(strings.Join(args, " "), r.run(t, args...), "HQ-WS pack")
				}
				for _, args := range forms([][]string{
					{"org-variables", "get", "--visible-for-org-id", bbOrgID, "--json"},
					{"org-variables", "get", "--visible-for-org-id", bbOrgID, "--csv"},
					{"org-variables", "get", "--visible-for-org-id", bbOrgID},
				}) {
					check(strings.Join(args, " "), r.run(t, args...), "HQ-WS general")
				}
				check("createOrganizationApiClient", r.runIn(t, gqlBody(mutCreateClient), "organizations", "create", "--stdin", "--yes", "--json"), "HQ-WS client")
				check("MCP reads", r.mcpCalls(t, []map[string]any{
					read("users.get", map[string]any{}),
					read("pack-configs.get", map[string]any{"orgId": bbOrgID}),
					read("org-variables.get", map[string]any{"visibleForOrgId": bbOrgID}),
					confirmed(gqlExecute(mutCreateClient), true),
				}), "HQ-WS client")
			}
			if len(rs.allMethods()) == 0 {
				t.Error("no read reached the fake; the test did not run")
			}
		})
	}
}
