// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"eset-protect-pp-cli/internal/cliutil/testenv"
)

// mintAnswer is one way ESET's token endpoint (or whatever stands between
// the CLI and it) can answer the password grant.
type mintAnswer struct {
	name     string
	status   int
	body     string
	header   map[string]string
	rejected bool
}

var mintAnswers = []mintAnswer{
	{"400 unauthorized_client / invalid_username_or_password", 400, `{"error":"unauthorized_client","error_description":"invalid_username_or_password"}`, nil, true},
	{"400 invalid_grant", 400, `{"error":"invalid_grant"}`, nil, true},
	{"400 INVALID_GRANT spelled loudly", 400, `{"error":" INVALID_GRANT "}`, nil, true},
	{"400 description only", 400, `{"error_description":"invalid_username_or_password"}`, nil, true},
	{"401", 401, `{"error":"unauthorized"}`, nil, true},
	{"403", 403, ``, nil, true},
	{"400 invalid_request", 400, `{"error":"invalid_request"}`, nil, false},
	{"400 not JSON", 400, `<html>Bad Request</html>`, nil, false},
	{"429", 429, `{"error":"too_many_requests"}`, map[string]string{"Retry-After": "1"}, false},
	{"500", 500, `{"error":"server_error"}`, nil, false},
	{"502 from a proxy", 502, `<html>Bad Gateway</html>`, nil, false},
	{"503", 503, ``, nil, false},
	{"504", 504, `{"code":4,"message":"timeout"}`, nil, false},
	{"307 redirect (never followed)", 307, ``, map[string]string{"Location": "https://attacker.example/oauth/token"}, false},
	{"404", 404, `{"code":5}`, nil, false},
	{"200 without a token", 200, `{"token_type":"Bearer"}`, nil, false},
}

// mintRun runs args with the password grant against the given token URL and
// returns the error and stderr.
func mintRun(t *testing.T, tokenURL, apiURL string, args ...string) (error, string) {
	t.Helper()
	esetTokenURLOverride = tokenURL
	t.Cleanup(func() { esetTokenURLOverride = "" })
	t.Setenv("ESET_PROTECT_BASE_URL", apiURL)
	t.Setenv("ESET_PROTECT_REGION", "eu")
	t.Setenv("ESET_PROTECT_TOKEN", "")
	t.Setenv("ESET_PROTECT_USERNAME", "api@msp.example")
	t.Setenv("ESET_PROTECT_PASSWORD", "right-password")
	cmd := RootCmd()
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return err, errOut.String()
}

// TestMintExitCodeTracksRejection (adversarial round 4, P2): `auth login`,
// and every command's token mint, exited 4 ("rejected") for any token
// request that did not return a token: a transport error, a proxy refusal, a
// 5xx or a 429 from the IAM host. A script keying on 4 = wrong password then
// tells the MSP to reset a password during an outage. Only an explicit ESET
// credential rejection (400 invalid_grant / invalid_username_or_password /
// unauthorized_client, 401, 403) is 4; everything else is 5, and the message
// never says "rejected" for it.
func TestMintExitCodeTracksRejection(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"deviceGroups":[],"devices":[],"nextPageToken":""}`)
	}))
	t.Cleanup(api.Close)
	dead, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	deadURL := "http://" + dead.Addr().String() + "/oauth/token"
	_ = dead.Close()

	cases := append([]mintAnswer(nil), mintAnswers...)
	cases = append(cases, mintAnswer{name: "connection refused", rejected: false})
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			tokenURL := deadURL
			if tc.status != 0 {
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					for k, v := range tc.header {
						w.Header().Set(k, v)
					}
					w.WriteHeader(tc.status)
					_, _ = io.WriteString(w, tc.body)
				}))
				t.Cleanup(srv.Close)
				tokenURL = srv.URL + "/oauth/token"
			}
			want := 5
			if tc.rejected {
				want = 4
			}
			for _, args := range [][]string{{"auth", "login"}, {"auth", "login", "--json"}, {"devices", "list", "--data-source", "live", "--json"}} {
				testenv.Isolate(t)
				respondClearHarness(t)
				err, stderr := mintRun(t, tokenURL, api.URL, args...)
				if err == nil {
					t.Fatalf("%v: no error for a token answer of %s", args, tc.name)
				}
				if got := ExitCode(err); got != want {
					t.Fatalf("%v: exit %d, want %d for %s: %v\n%s", args, got, want, tc.name, err, stderr)
				}
				if !tc.rejected && strings.Contains(err.Error(), "rejected") {
					t.Fatalf("%v: %s is not a credential rejection, but the error says so: %v", args, tc.name, err)
				}
			}
		})
	}
}

// TestDoctorMintFailureIsNotARejection: doctor's Credentials row says
// "rejected" only for an explicit credential rejection.
func TestDoctorMintFailureIsNotARejection(t *testing.T) {
	for _, tc := range []mintAnswer{mintAnswers[0], {name: "503", status: 503}, {name: "429", status: 429}} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			testenv.Isolate(t)
			respondClearHarness(t)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			}))
			t.Cleanup(srv.Close)
			api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"deviceGroups":[],"nextPageToken":""}`)
			}))
			t.Cleanup(api.Close)
			esetTokenURLOverride = srv.URL
			t.Cleanup(func() { esetTokenURLOverride = "" })
			t.Setenv("ESET_PROTECT_BASE_URL", api.URL)
			t.Setenv("ESET_PROTECT_REGION", "eu")
			t.Setenv("ESET_PROTECT_TOKEN", "")
			t.Setenv("ESET_PROTECT_USERNAME", "api@msp.example")
			t.Setenv("ESET_PROTECT_PASSWORD", "right-password")
			report, _ := runDoctorJSON(t)
			creds, _ := report["credentials"].(string)
			if tc.rejected != strings.HasPrefix(creds, "ERROR rejected") {
				t.Fatalf("credentials = %q for a token answer of %s", creds, tc.name)
			}
			if !strings.HasPrefix(creds, "ERROR") {
				t.Fatalf("credentials = %q: a failed mint must stay negative", creds)
			}
		})
	}
}
