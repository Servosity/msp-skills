// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"eset-protect-pp-cli/internal/cliutil"
	"eset-protect-pp-cli/internal/cliutil/testenv"
)

// mintFallbackEnv points the password grant at tokenURL and the API at
// apiURL, and returns a home whose default mirror (scoped to that API user)
// holds three devices when seed is true.
func mintFallbackEnv(t *testing.T, tokenURL, apiURL string, seed bool) string {
	t.Helper()
	esetTokenURLOverride = tokenURL
	t.Cleanup(func() { esetTokenURLOverride = "" })
	respondClearHarness(t)
	t.Setenv("ESET_PROTECT_BASE_URL", apiURL)
	t.Setenv("ESET_PROTECT_REGION", "eu")
	t.Setenv("ESET_PROTECT_TOKEN", "")
	t.Setenv("ESET_PROTECT_USERNAME", "api@msp.example")
	t.Setenv("ESET_PROTECT_PASSWORD", "right-password")
	t.Setenv(cliutil.DogfoodEnvVar, "")
	if seed {
		return localQuerySeed(t)
	}
	return testenv.Isolate(t)
}

func mintFallbackRun(t *testing.T, home string, args ...string) (string, string, error) {
	t.Helper()
	root := RootCmd()
	var out, errOut bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errOut)
	root.SetArgs(append(append([]string(nil), args...), "--home", home))
	err := root.Execute()
	return out.String(), errOut.String(), err
}

// TestAutoReadsFallBackWhenTheTokenServiceIsDown (adversarial round 5, P2):
// the token is minted while the client is built, so a token service outage
// (5xx, 429, no answer) failed every --data-source auto read with exit 5
// before the auto logic could answer from the local mirror; the documented
// fallback worked only while a cached token was still valid. A mint failure
// that is not a credential rejection now reads like an unreachable API: the
// auto read answers from the mirror with the same meta (source local, reason
// api_unreachable) the transport-failure fallback gives, and sends nothing to
// ESET. A credential rejection still fails
// (exit 4) and never falls back. With no mirror the auto read fails exit 5,
// naming both the missing local data and the token failure. A
// --data-source local read, which never needed the token, answers too.
func TestAutoReadsFallBackWhenTheTokenServiceIsDown(t *testing.T) {
	var apiHits int32
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&apiHits, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"devices":[],"nextPageToken":""}`)
	}))
	t.Cleanup(api.Close)
	dead, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	deadURL := "http://" + dead.Addr().String() + "/oauth/token"
	_ = dead.Close()

	cases := append([]mintAnswer(nil), mintAnswers...)
	cases = append(cases, mintAnswer{name: "connection refused"},
		mintAnswer{name: "202 without response-id", status: http.StatusAccepted},
		mintAnswer{name: "200 HTML", status: http.StatusOK, body: "<html><title>Sign in</title></html>"},
		mintAnswer{name: "400 access_denied", status: http.StatusBadRequest, body: `{"error":"access_denied","error_description":"User account is disabled"}`},
		mintAnswer{name: "400 invalid_client", status: http.StatusBadRequest, body: `{"error":"invalid_client"}`})
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
			atomic.StoreInt32(&apiHits, 0)
			home := mintFallbackEnv(t, tokenURL, api.URL, true)
			out, errOut, err := mintFallbackRun(t, home, "devices", "list", "--json")
			if tc.rejected {
				if err == nil || ExitCode(err) != 4 {
					t.Fatalf("a credential rejection must fail exit 4, got %v (exit %d)\n%s", err, ExitCode(err), out)
				}
				if strings.Contains(out, "KIOSK-03") {
					t.Fatalf("a credential rejection fell back to the mirror: %s", out)
				}
				out, _, err = mintFallbackRun(t, home, "devices", "list", "--data-source", "local", "--json")
				if err == nil || ExitCode(err) != 4 || strings.Contains(out, "KIOSK-03") {
					t.Fatalf("a credential rejection must fail a local read too (exit 4), got %v (exit %d)\n%s", err, ExitCode(err), out)
				}
			} else if outage := tc.status == 0 || tc.status >= 500 || tc.status == http.StatusTooManyRequests; !outage {
				// Adversarial round 6: only positive evidence of an outage
				// (no answer, 5xx, 429) defers to the mirror. Any other
				// answer (another 4xx, a 200 without a token, HTML, a
				// redirect) fails exit 5 at construction, as before round 5.
				if err == nil || ExitCode(err) != 5 || strings.Contains(out, "KIOSK-03") {
					t.Fatalf("%s is not an outage: want exit 5 and no mirror rows, got %v (exit %d)\n%s", tc.name, err, ExitCode(err), out)
				}
			} else {
				if err != nil {
					t.Fatalf("auto read with the token service down: %v (exit %d)\nstdout %s\nstderr %s", err, ExitCode(err), out, errOut)
				}
				if !strings.Contains(out, "KIOSK-03") || !strings.Contains(out, `"api_unreachable"`) || !strings.Contains(out, `"local"`) {
					t.Fatalf("auto read did not answer from the mirror with api_unreachable: %s", out)
				}
				// The fallback says why, in every output mode.
				if n := strings.Count(errOut, "no ESET Connect token could be minted"); n != 1 || !strings.Contains(errOut, "token request") {
					t.Fatalf("auto fallback must print one stderr line naming the token failure (got %d):\n%s", n, errOut)
				}
				for _, mode := range []string{"--csv", "--plain", "--quiet", "--agent"} {
					_, modeErr, merr := mintFallbackRun(t, home, "devices", "list", mode)
					if merr != nil || !strings.Contains(modeErr, "no ESET Connect token could be minted") {
						t.Fatalf("devices list %s fell back without naming the token failure (%v):\n%s", mode, merr, modeErr)
					}
				}
				// A single get takes the other fallback path and says so too.
				getOut, getErr, gerr := mintFallbackRun(t, home, "devices", "get", "00000000-0000-4000-8000-000000000002", "--json")
				if gerr != nil || !strings.Contains(getOut, "KIOSK-03") || !strings.Contains(getErr, "no ESET Connect token could be minted") {
					t.Fatalf("devices get fell back without naming the token failure (%v):\nstdout %s\nstderr %s", gerr, getOut, getErr)
				}
				// A local read never needed the token.
				out, _, err = mintFallbackRun(t, home, "devices", "list", "--data-source", "local", "--json")
				if err != nil || !strings.Contains(out, "KIOSK-03") || !strings.Contains(out, `"user_requested"`) {
					t.Fatalf("local read with the token service down: %v\n%s", err, out)
				}
				// Live stays live: exit 5 with the token failure's own words
				// and nothing on stdout, exactly as before.
				out, _, err = mintFallbackRun(t, home, "devices", "list", "--data-source", "live", "--json")
				if err == nil || ExitCode(err) != 5 || !strings.Contains(err.Error(), "token request") || out != "" {
					t.Fatalf("live read: %v (exit %d), stdout %q, want the token failure, exit 5, empty stdout", err, ExitCode(err), out)
				}
				// No mirror: exit 5, naming both.
				empty := mintFallbackEnv(t, tokenURL, api.URL, false)
				_, _, err = mintFallbackRun(t, empty, "devices", "list", "--json")
				if err == nil || ExitCode(err) != 5 || !strings.Contains(err.Error(), "API unreachable and no local data") || !strings.Contains(err.Error(), "token request") {
					t.Fatalf("auto read with no mirror: %v (exit %d)", err, ExitCode(err))
				}
			}
			if n := atomic.LoadInt32(&apiHits); n != 0 {
				t.Fatalf("%d request(s) reached ESET without a token", n)
			}
		})
	}
}

// TestTokenOutageKeepsEveryOtherExitStatus: deferring the mint failure to the
// first request must not change what any other command reports. sync, a
// live novel view and a write still exit 5 for an outage and 4 for a
// rejection, with the token failure's words, and send nothing.
func TestTokenOutageKeepsEveryOtherExitStatus(t *testing.T) {
	var apiHits int32
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&apiHits, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{}`)
	}))
	t.Cleanup(api.Close)
	for _, tc := range []mintAnswer{{name: "503", status: 503}, {name: "429", status: 429, body: `{"error":"too_many_requests"}`}, mintAnswers[0], {name: "401", status: 401, rejected: true}} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			}))
			t.Cleanup(srv.Close)
			want, word := 5, "failed"
			if tc.rejected {
				want, word = 4, "rejected"
			}
			for _, args := range [][]string{
				{"sync", "--resources", "devices", "--json"},
				{"detections", "latest", "--since", "24h", "--json"},
				{"incidents", "close", "00000000-0000-4000-8000-000000000001", "--json"},
				{"exposure", "--cve", "CVE-2024-0001", "--json"},
			} {
				atomic.StoreInt32(&apiHits, 0)
				home := mintFallbackEnv(t, srv.URL+"/oauth/token", api.URL, true)
				out, errOut, err := mintFallbackRun(t, home, args...)
				if err == nil || ExitCode(err) != want {
					t.Fatalf("%v: %v (exit %d), want exit %d\nstdout %s\nstderr %s", args, err, ExitCode(err), want, out, errOut)
				}
				if !strings.Contains(err.Error(), "token request") || !strings.Contains(err.Error(), word) {
					t.Fatalf("%v: the error lost the token failure's words: %v", args, err)
				}
				// The client still fails at construction: nothing on stdout.
				if out != "" {
					t.Fatalf("%v: stdout %q, want nothing (the command must fail before it runs)", args, out)
				}
				if n := atomic.LoadInt32(&apiHits); n != 0 {
					t.Fatalf("%v: %d request(s) reached ESET without a token", args, n)
				}
			}
		})
	}
}

// TestTokenOutageNeverServesTheResponseCache: a GET answered live a minute ago
// sits in the response cache. Once the token cannot be minted, that cached
// answer is not passed off as live: the auto read answers from the mirror,
// labelled api_unreachable, and the root search falls back to local search.
func TestTokenOutageNeverServesTheResponseCache(t *testing.T) {
	var mintDown int32
	mint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.LoadInt32(&mintDown) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		// Inside the refresh margin, so the next run must mint again.
		_, _ = io.WriteString(w, `{"access_token":"tok-1","token_type":"Bearer","expires_in":60}`)
	}))
	t.Cleanup(mint.Close)
	var apiHits int32
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&apiHits, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"devices":[{"uuid":"00000000-0000-4000-8000-0000000000aa","displayName":"LIVEONLY-01"}],"nextPageToken":""}`)
	}))
	t.Cleanup(api.Close)
	home := mintFallbackEnv(t, mint.URL+"/oauth/token", api.URL, true)
	out, _, err := mintFallbackRun(t, home, "devices", "list", "--json")
	if err != nil || !strings.Contains(out, "LIVEONLY-01") {
		t.Fatalf("live read with a working token: %v\n%s", err, out)
	}
	atomic.StoreInt32(&mintDown, 1)
	atomic.StoreInt32(&apiHits, 0)
	out, _, err = mintFallbackRun(t, home, "devices", "list", "--json")
	if err != nil {
		t.Fatalf("auto read with the token service down: %v", err)
	}
	// The first read wrote LIVEONLY-01 through to the mirror, so it may be
	// listed; what must not happen is the cached response labelled live.
	if strings.Contains(out, `"source": "live"`) || !strings.Contains(out, `"source": "local"`) || !strings.Contains(out, "KIOSK-03") || !strings.Contains(out, `"api_unreachable"`) {
		t.Fatalf("the cached live answer was served without a token (or no mirror fallback): %s", out)
	}
	// search-is-local-only: search never calls ESET, so a token outage
	// cannot affect it; it answers from the mirror.
	out, errOut, err := mintFallbackRun(t, home, "search", "KIOSK", "--json")
	if err != nil || !strings.Contains(out, "KIOSK") {
		t.Fatalf("search with the token service down: %v\n%s%s", err, out, errOut)
	}
	if n := atomic.LoadInt32(&apiHits); n != 0 {
		t.Fatalf("%d request(s) reached ESET without a token", n)
	}
}

// TestRemintFailureKeepsItsClass (adversarial round 6, P2): ESET answered 401
// to a token and the re-mint then failed because the token service was down
// (503); the transport dropped the re-mint error and returned the 401, so the
// command exited 4 "ESET Connect rejected the token" during an outage. The
// re-mint's own error now decides (handfixes.json:
// reauth-failure-keeps-its-class): an outage exits 5 naming the token
// failure (and an auto read falls back to the mirror, saying why), a
// rejection exits 4, any other answer exits 5.
func TestRemintFailureKeepsItsClass(t *testing.T) {
	var apiHits int32
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&apiHits, 1)
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"message":"token expired"}`)
	}))
	t.Cleanup(api.Close)
	for _, tc := range []struct {
		name   string
		status int
		body   string
		want   int
		word   string
	}{
		{"503", http.StatusServiceUnavailable, ``, 5, "failed"},
		{"429", http.StatusTooManyRequests, `{"error":"too_many_requests"}`, 5, "failed"},
		{"400 invalid_grant", http.StatusBadRequest, `{"error":"invalid_grant"}`, 4, "rejected"},
		{"404", http.StatusNotFound, `{"code":5}`, 5, "failed"},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			var mints int32
			tok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if atomic.AddInt32(&mints, 1) == 1 {
					w.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(w, `{"access_token":"tok-first","refresh_token":"rt-first","token_type":"Bearer","expires_in":3600}`)
					return
				}
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			}))
			t.Cleanup(tok.Close)
			home := mintFallbackEnv(t, tok.URL+"/oauth/token", api.URL, true)
			out, errOut, err := mintFallbackRun(t, home, "devices", "list", "--data-source", "live", "--json")
			if err == nil || ExitCode(err) != tc.want || !strings.Contains(err.Error(), "token request") || !strings.Contains(err.Error(), tc.word) {
				t.Fatalf("401 then a %s re-mint: %v (exit %d), want exit %d naming the token failure\nstdout %s\nstderr %s", tc.name, err, ExitCode(err), tc.want, out, errOut)
			}
			if strings.Contains(err.Error(), "rejected the token; check the API user") {
				t.Fatalf("the 401 hint replaced the re-mint's own error: %v", err)
			}
			if atomic.LoadInt32(&mints) < 2 {
				t.Fatalf("no re-mint was attempted (%d mints)", mints)
			}
			if tc.status == http.StatusServiceUnavailable {
				// The cached token from the first run is used, refused, and
				// its re-mint meets the outage again.
				out, errOut, err = mintFallbackRun(t, home, "devices", "list", "--json")
				if err != nil || !strings.Contains(out, "KIOSK-03") || !strings.Contains(errOut, "no ESET Connect token could be minted") {
					t.Fatalf("auto read after a 401 and a re-mint outage must fall back and say why: %v\nstdout %s\nstderr %s", err, out, errOut)
				}
			}
		})
	}
}
