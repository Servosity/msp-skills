// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"rewst-pp-cli/internal/cliutil/testenv"
)

// rewst handfix graphql-errors-are-failures: a GraphQL answer that carries an
// errors (or error) member is a failure for the typed commands, whatever its
// HTTP status and whatever data rides beside it; a clean answer still prints.
func TestGraphQLErrorsAnswerIsAFailure(t *testing.T) {
	cases := []struct {
		name, body string
		fail       bool
	}{
		{"errors only", `{"errors":[{"message":"Not authorized"}]}`, true},
		{"partial data", `{"data":{"visibleOrgVariables":[{"id":"ov1","name":"x"}]},"errors":[{"message":"field denied"}]}`, true},
		{"errors object", `{"errors":{"message":"bad"}}`, true},
		{"error string", `{"error":"upstream timeout"}`, true},
		{"errors before data", `{"errors":[{}],"data":null}`, true},
		{"clean", `{"data":{"visibleOrgVariables":[{"id":"ov1","name":"x","category":"general","value":"v"}]}}`, false},
		{"empty errors", `{"data":{"visibleOrgVariables":[]},"errors":[]}`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()
			testenv.Isolate(t)
			t.Setenv("REWST_BASE_URL", srv.URL)
			t.Setenv("REWST_API_TOKEN", "rwst-gql-errors-token-7b2e")
			out, errOut, err := runRootArgs(t, "org-variables", "get", "--visible-for-org-id", "org-1", "--json", "--no-cache", "--data-source", "live")
			if tc.fail && err == nil {
				t.Fatalf("a GraphQL answer with errors exited 0:\n%s\n%s", out, errOut)
			}
			if !tc.fail && err != nil {
				t.Fatalf("a clean GraphQL answer failed: %v\n%s\n%s", err, out, errOut)
			}
			if !tc.fail && !strings.Contains(out, `"ov1"`) && tc.name == "clean" {
				t.Fatalf("the clean answer's data was not printed:\n%s", out)
			}
		})
	}
}

func TestGraphQLAnswerFailedReadsEveryShape(t *testing.T) {
	for body, want := range map[string]bool{
		`{"errors":[{"message":"x"}]}`: true, `{"errors":"x"}`: true, `{"errors":1}`: true, `{"errors":true}`: true,
		`{"error":{"code":"X"}}`: true, `{"data":{},"errors":[null]}`: true,
		`{"data":{"a":1}}`: false, `{"errors":null}`: false, `{"errors":[]}`: false, `{"error":""}`: false, `[1,2]`: false, `not json`: false,
	} {
		if got := graphQLAnswerFailed([]byte(body)); got != want {
			t.Errorf("graphQLAnswerFailed(%s) = %v, want %v", body, got, want)
		}
	}
}

// The strongest error code decides the status: an authentication failure is a
// 401 (not a retried 502), whatever other errors ride beside it.
func TestGraphQLFailureStatusFollowsTheCode(t *testing.T) {
	for body, want := range map[string]int{
		`{"errors":[{"message":"Not Authorized - User not found","extensions":{"code":"AUTH_ERR"}}]}`:          401,
		`{"errors":[{"extensions":{"code":"BAD_USER_INPUT"}},{"extensions":{"code":"UNAUTHENTICATED"}}]}`:      401,
		`{"errors":[{"extensions":{"code":"FORBIDDEN"}},{"extensions":{"code":"GRAPHQL_VALIDATION_FAILED"}}]}`: 403,
		`{"errors":[{"message":"x","extensions":{"code":"BAD_REQUEST"}}]}`:                                     400,
		`{"errors":[{"message":"x","extensions":{"code":"INTERNAL_SERVER_ERROR"}}]}`:                           502,
		`{"errors":[{"message":"no code"}]}`:                                                                   502,
		`{"errors":"a string"}`:                                                                                502,
		`{"error":"proxy"}`:                                                                                    502,
	} {
		if got := graphQLFailureStatus([]byte(body)); got != want {
			t.Errorf("graphQLFailureStatus(%s) = %d, want %d", body, got, want)
		}
	}
}
