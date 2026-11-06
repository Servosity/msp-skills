// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"rewst-pp-cli/internal/cliutil/testenv"
)

// rewst handfix doctor-graphql-probe: api.rewst.io is Rewst's real US API
// host (the EU and AU regions are their own hosts, set with REWST_BASE_URL).
// Matching it as the "shipped placeholder" told every healthy default install
// to change its base URL and never probed the credential.
func TestDoctorTreatsTheShippedHostAsReal(t *testing.T) {
	for _, real := range []string{"https://api.rewst.io", "https://api.rewst.io/", "https://api.rewst.eu", "https://api.rewst.com.au"} {
		if doctorBaseURLIsShippedPlaceholder(real) {
			t.Errorf("%s read as a placeholder", real)
		}
	}
	for _, p := range []string{"https://{region}.rewst.io", "https://acme.example/graphql", "https://x.example.com"} {
		if !doctorBaseURLIsShippedPlaceholder(p) {
			t.Errorf("%s not read as a placeholder", p)
		}
	}
}

// Rewst serves one endpoint, POST /graphql, and answers an authorization
// failure with HTTP 200 and an errors array. The probe must be a GraphQL query
// there (the token's own organization), the verdict must follow the errors
// array, and a GraphQL error's message (which can echo what was sent) is never
// printed.
func TestDoctorProbesTheGraphQLEndpoint(t *testing.T) {
	const token = "rwst-doctor-token-5c81e3d7a9"
	cases := []struct {
		name   string
		status int
		body   string
		want   string
	}{
		{"healthy", 200, `{"data":{"userOrganization":{"id":"0f6c1d2e-org"}}}`, "valid (verified with POST /graphql userOrganization)"},
		{"not authorized", 200, `{"data":{"userOrganization":null},"errors":[{"message":"Not authorized: ` + token + `"}]}`, "ERROR rejected (GraphQL errors from /graphql)"},
		{"rewst AUTH_ERR", 200, `{"errors":[{"message":"Not Authorized - User not found - userOrganization","locations":[{"line":1,"column":21}],"path":["userOrganization"],"extensions":{"code":"AUTH_ERR","stacktrace":["GraphQLError: Not Authorized"]}}],"data":{"userOrganization":null}}`, "ERROR rejected (GraphQL AUTH_ERR from /graphql)"},
		{"denied code", 200, `{"errors":[{"message":"x","extensions":{"code":"UNAUTHENTICATED"}}]}`, "ERROR rejected (GraphQL UNAUTHENTICATED from /graphql)"},
		{"other error", 200, `{"errors":[{"message":"echo ` + token + `","extensions":{"code":"<b>x</b>"}}]}`, "WARN not verified (POST /graphql returned GraphQL errors)"},
		{"no organization", 200, `{"data":{"userOrganization":null}}`, "WARN not verified (POST /graphql answered no organization"},
		{"data not an object", 200, `{"data":[],"items":[],"results":[]}`, "valid (POST /graphql answered with data)"},
		{"not graphql", 200, `<html>login</html>`, "WARN not verified (POST /graphql answered with a body that is not a GraphQL response)"},
		{"expired", 401, `{"error":"unauthorized"}`, "ERROR rejected (HTTP 401"},
		{"wrong root", 404, ``, "ERROR not verified (HTTP 404 from /graphql)"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var sawQuery, sawAuth bool
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.Path != "/graphql" {
					http.NotFound(w, r)
					return
				}
				b, _ := io.ReadAll(r.Body)
				var req struct {
					Query string `json:"query"`
				}
				if json.Unmarshal(b, &req) == nil && strings.Contains(req.Query, "userOrganization") && !strings.Contains(req.Query, "mutation") {
					sawQuery = true
				}
				sawAuth = r.Header.Get("Authorization") == "Bearer "+token
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()
			testenv.Isolate(t)
			t.Setenv("REWST_BASE_URL", srv.URL)
			t.Setenv("REWST_API_TOKEN", token)
			out, errOut, _ := runRootArgs(t, "doctor", "--json")
			start := strings.Index(out, "{")
			if start < 0 {
				t.Fatalf("doctor printed no JSON:\n%s\n%s", out, errOut)
			}
			var report map[string]any
			if err := json.NewDecoder(strings.NewReader(out[start:])).Decode(&report); err != nil {
				t.Fatalf("doctor JSON: %v\n%s", err, out)
			}
			creds, _ := report["credentials"].(string)
			if !strings.Contains(creds, tc.want) {
				t.Fatalf("credentials = %q, want %q", creds, tc.want)
			}
			if tc.status == 200 && (!sawQuery || !sawAuth) {
				t.Fatalf("doctor never sent the userOrganization query with the Bearer token to POST /graphql (query %v, auth %v)", sawQuery, sawAuth)
			}
			if strings.Contains(out+errOut, token) {
				t.Fatalf("doctor printed the credential:\n%s\n%s", out, errOut)
			}
		})
	}
}
