// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"dropsuite-pp-cli/internal/client"
	"dropsuite-pp-cli/internal/cliutil/testenv"
	"dropsuite-pp-cli/internal/config"
)

// These tests prove doctor-verdict-tracks-reality (handfixes.json) on the
// report doctor builds, against a stub partner API that checks both Dropsuite
// headers the way the real one does: GET /api/users answers 200 only to the
// reseller Authentication Token (X-Access-Token) together with the Reseller
// Token (X-Reseller-Token), and {"errors":{"user":["Unauthorized"]}} with 401
// otherwise (the live dropsuite.us answer to a wrong token).
// tools/maintainer/check_doctor_truth.py runs the same invariant on the built
// binary.

// Fixture tokens alternate a digit and a letter, so every 8-character piece
// holds both and none is a word or appears in anything doctor prints. They are
// split so a scanner sees no whole synthetic secret.
var (
	doctorTestAccess   = "4h7j2k9m" + "5n1p3q8r"
	doctorTestReseller = "6t3w8y1b" + "2c5d9f4g"
)

// doctorVerdictPositive mirrors is_positive in check_doctor_truth.py: a
// verdict is negative when it opens with a marker or contains a negative
// phrase, and anything else is a claim of health.
func doctorVerdictPositive(v any) bool {
	s, ok := v.(string)
	if !ok {
		return false
	}
	s = strings.TrimSpace(s)
	for _, p := range []string{"ERROR", "WARN", "INFO", "FAIL"} {
		if strings.HasPrefix(s, p) {
			return false
		}
	}
	low := strings.ToLower(s)
	for _, p := range []string{"not verified", "not configured", "unreachable", "invalid", "rejected",
		"scope-limited", "skipped", "placeholder", "not there", "error", "refused", "not loaded",
		"transport level only"} {
		if strings.Contains(low, p) {
			return false
		}
	}
	return true
}

// doctorStub is a Dropsuite partner API, answering per mode:
//
//	healthy     GET /api/users: 200 to both fixture tokens, 401 otherwise; 404 elsewhere
//	expired     401 to every request (the tokens are dead)
//	wrong-base  200 HTML at /, 404 at every other path (a web UI, not the API)
type doctorStub struct {
	mode     string
	mu       sync.Mutex
	requests []string
	authed   []string
}

func (s *doctorStub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	authed := r.Header.Get("X-Access-Token") == doctorTestAccess && r.Header.Get("X-Reseller-Token") == doctorTestReseller
	s.mu.Lock()
	s.requests = append(s.requests, r.Method+" "+r.URL.Path)
	if r.Header.Get("X-Access-Token") != "" || r.Header.Get("X-Reseller-Token") != "" {
		s.authed = append(s.authed, r.Method+" "+r.URL.Path)
	}
	s.mu.Unlock()
	unauthorized := func() {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"errors":{"user":["Unauthorized"]}}`)
	}
	switch s.mode {
	case "healthy":
		if r.URL.Path != "/api/users" {
			http.NotFound(w, r)
			return
		}
		if !authed {
			unauthorized()
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[]`)
	case "wrong-base":
		if r.URL.Path == "/" {
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprint(w, "<!doctype html><html><body>partner portal</body></html>")
			return
		}
		http.NotFound(w, r)
	default:
		unauthorized()
	}
}

// seen returns every request, and those that carried either Dropsuite header.
func (s *doctorStub) seen() (requests, authed []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.requests...), append([]string(nil), s.authed...)
}

// doctorTestInstall points the CLI at the stub the way an operator does: both
// tokens in the environment and DROPSUITE_BASE_URL at the partner API root
// (https://<host>/api; the wrong-base mode takes the bare host, as a web UI
// address would be pasted).
func doctorTestInstall(t *testing.T, mode string, withCredential bool) *doctorStub {
	t.Helper()
	testenv.Isolate(t)
	stub := &doctorStub{mode: mode}
	srv := httptest.NewServer(stub)
	t.Cleanup(srv.Close)
	for _, name := range []string{"PRINTING_PRESS_VERIFY", "PRINTING_PRESS_VERIFY_LIVE_HTTP",
		"DROPSUITE_BASE_URL", "DROPSUITE_HOST", "DROPSUITE_ACCESS_TOKEN", "DROPSUITE_RESELLER_TOKEN",
		"DROPSUITE_ORG", "DROPSUITE_CA_CERT", "DROPSUITE_TLS_SERVER_NAME", "PRINTING_PRESS_CLIENT_PROFILE"} {
		t.Setenv(name, "")
	}
	base := srv.URL + "/api"
	if mode == "wrong-base" {
		base = srv.URL
	}
	t.Setenv("DROPSUITE_BASE_URL", base)
	if withCredential {
		t.Setenv("DROPSUITE_ACCESS_TOKEN", doctorTestAccess)
		t.Setenv("DROPSUITE_RESELLER_TOKEN", doctorTestReseller)
	}
	return stub
}

// runDoctorReport runs `doctor --json` through the real command tree and
// returns the report as doctor built it.
func runDoctorReport(t *testing.T) map[string]any {
	t.Helper()
	root := RootCmd()
	var out, errOut bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errOut)
	root.SetArgs([]string{"doctor", "--json"})
	if err := root.Execute(); err != nil {
		t.Fatalf("doctor --json failed: %v\nstderr: %s", err, errOut.String())
	}
	var report map[string]any
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatalf("doctor --json emitted no JSON report: %v\n%s", err, out.String())
	}
	return report
}

// TestDoctorVerdictTracksReality is the invariant of check_doctor_truth.py on
// the report itself: `credentials` is positive only when the tokens actually
// worked, `api` only when the connector reached the partner API, and the three
// dialled states never share a credentials verdict.
func TestDoctorVerdictTracksReality(t *testing.T) {
	verdicts := map[string]any{}
	for _, mode := range []string{"healthy", "expired", "wrong-base"} {
		t.Run(mode, func(t *testing.T) {
			stub := doctorTestInstall(t, mode, true)
			report := runDoctorReport(t)
			creds, api := report["credentials"], report["api"]
			verdicts[mode] = creds
			if creds == nil {
				t.Fatalf("doctor emitted no credentials row; report: %v", report)
			}
			_, authed := stub.seen()
			switch mode {
			case "healthy":
				if !doctorVerdictPositive(creds) {
					t.Errorf("credentials should verify against a working API, got %q", creds)
				}
				if !doctorVerdictPositive(api) {
					t.Errorf("api should be healthy against a working API, got %q", api)
				}
				// The probe must carry the reseller tokens to GET /users, the
				// reseller-level read that lists organizations.
				if !doctorContains(authed, "GET /api/users") {
					t.Errorf("the credential probe never sent the tokens to GET /api/users; requests carrying them: %v", authed)
				}
			case "expired":
				if doctorVerdictPositive(creds) {
					t.Errorf("every request answered 401 and doctor still reports credentials as healthy: %q", creds)
				}
			case "wrong-base":
				if doctorVerdictPositive(creds) {
					t.Errorf("every API path 404s and doctor still reports credentials as healthy: %q", creds)
				}
				if doctorVerdictPositive(api) {
					t.Errorf("base_url is not the API root and doctor still reports the API as healthy: %q", api)
				}
				if s, _ := api.(string); !strings.Contains(s, "DROPSUITE_BASE_URL") || !strings.Contains(s, "DROPSUITE_HOST") {
					t.Errorf("the wrong-base api verdict does not name DROPSUITE_BASE_URL and DROPSUITE_HOST: %q", api)
				}
			}
		})
	}
	for _, broken := range []string{"expired", "wrong-base"} {
		if verdicts["healthy"] != nil && verdicts[broken] == verdicts["healthy"] {
			t.Errorf("doctor reports the identical credentials verdict (%q) whether the credential works or the install is %s",
				verdicts["healthy"], broken)
		}
	}
}

// TestDoctorMissingResellerTokenIsNamed: the Authentication Token alone is
// refused by GET /users; the verdict must be negative and name the variable.
func TestDoctorMissingResellerTokenIsNamed(t *testing.T) {
	doctorTestInstall(t, "healthy", true)
	t.Setenv("DROPSUITE_RESELLER_TOKEN", "")
	report := runDoctorReport(t)
	creds := report["credentials"]
	if doctorVerdictPositive(creds) {
		t.Errorf("GET /users answered 401 and credentials read healthy: %q", creds)
	}
	if s, _ := creds.(string); !strings.Contains(s, "DROPSUITE_RESELLER_TOKEN") {
		t.Errorf("the reseller token is unset and the credentials verdict does not name DROPSUITE_RESELLER_TOKEN: %q", creds)
	}
}

// TestDoctorPlaceholderNamesTheFixAndDialsNothing: a base URL left at a
// reserved example host is never dialled, neither row reads healthy, and the
// api row names the variable that fixes it.
func TestDoctorPlaceholderNamesTheFixAndDialsNothing(t *testing.T) {
	stub := doctorTestInstall(t, "healthy", true)
	t.Setenv("DROPSUITE_BASE_URL", "https://partner.example.com/api")
	report := runDoctorReport(t)
	creds, api := report["credentials"], report["api"]
	if creds == nil {
		t.Fatalf("doctor emitted no credentials row; report: %v", report)
	}
	if doctorVerdictPositive(creds) || doctorVerdictPositive(api) {
		t.Errorf("base_url is a placeholder and a row reads healthy: api=%q credentials=%q", api, creds)
	}
	if s, _ := api.(string); !strings.Contains(s, "DROPSUITE_BASE_URL") {
		t.Errorf("the placeholder api verdict does not name DROPSUITE_BASE_URL: %q", api)
	}
	if requests, _ := stub.seen(); len(requests) != 0 {
		t.Errorf("doctor dialled with a placeholder base URL: %v", requests)
	}
}

// TestDoctorWithoutACredentialSaysSo: a reachable API and no credential at all
// must produce a negative credentials row, not silence, and send no token.
func TestDoctorWithoutACredentialSaysSo(t *testing.T) {
	stub := doctorTestInstall(t, "healthy", false)
	report := runDoctorReport(t)
	creds := report["credentials"]
	if creds == nil {
		t.Fatalf("no credential is configured and doctor emitted no credentials row; report: %v", report)
	}
	if doctorVerdictPositive(creds) {
		t.Errorf("no credential is configured and credentials read healthy: %q", creds)
	}
	if _, authed := stub.seen(); len(authed) != 0 {
		t.Errorf("no credential is configured, yet a token reached the API: %v", authed)
	}
}

// doctorDefaultHostConfig loads the configuration an operator has when they set
// both tokens and nothing else: the shipped https://{host}/api with the default
// host dropsuite.us. The harness's placeholder state is this configuration.
func doctorDefaultHostConfig(t *testing.T) *config.Config {
	t.Helper()
	doctorTestInstall(t, "healthy", true)
	t.Setenv("DROPSUITE_BASE_URL", "")
	cfg, err := config.Load("")
	if err != nil {
		t.Fatal(err)
	}
	if got := client.ResolvedBaseURL(cfg.BaseURL, cfg.TemplateVars); got != "https://dropsuite.us/api" {
		t.Fatalf("with nothing set the CLI should dial https://dropsuite.us/api, resolved %q", got)
	}
	return cfg
}

// TestDoctorDefaultHostInForce: only the shipped template with the default
// host, chosen by nobody, counts; naming any host or URL is a choice.
func TestDoctorDefaultHostInForce(t *testing.T) {
	cfg := doctorDefaultHostConfig(t)
	if !doctorDefaultHostInForce(cfg) {
		t.Fatalf("nothing is set, so the default host is in force; got false for base_url %q vars %v", cfg.BaseURL, cfg.TemplateVars)
	}
	for _, tc := range []struct{ env, value string }{
		{"DROPSUITE_HOST", "dropsuite.us"},
		{"DROPSUITE_HOST", "euw.dropmyemail.com"},
		{"DROPSUITE_BASE_URL", "https://dropsuite.us/api"},
	} {
		t.Run(tc.env+"="+tc.value, func(t *testing.T) {
			t.Setenv(tc.env, tc.value)
			cfg, err := config.Load("")
			if err != nil {
				t.Fatal(err)
			}
			if doctorDefaultHostInForce(cfg) {
				t.Errorf("%s=%s names the host, yet the default host reads as in force", tc.env, tc.value)
			}
		})
	}
}

// TestDoctorDefaultHostIsProbedAndNamesTheFix: the default host is a real
// partner API, so doctor probes it with the tokens rather than refusing it; a
// working credential there reads healthy, and one that does not verify there
// leaves the api row negative and naming DROPSUITE_HOST and DROPSUITE_BASE_URL.
// The client is aimed at the stub in place of dropsuite.us, so no test
// touches the network, while its configuration is the defaulted one doctor sees.
func TestDoctorDefaultHostIsProbedAndNamesTheFix(t *testing.T) {
	for _, mode := range []string{"healthy", "expired"} {
		t.Run(mode, func(t *testing.T) {
			cfg := doctorDefaultHostConfig(t)
			stub := &doctorStub{mode: mode}
			srv := httptest.NewServer(stub)
			t.Cleanup(srv.Close)
			c := client.New(cfg, 10*time.Second, 0)
			c.BaseURL = srv.URL + "/api"
			if err := ApplyClientHooks(c); err != nil {
				t.Fatal(err)
			}
			const reached = "reachable (HTTP 404 at /)"
			report := map[string]any{"api": reached}
			doctorProbeCredentials(context.Background(), c, RootCmd(), "dropsuite-cli", report)
			creds, api := report["credentials"], report["api"]
			if _, authed := stub.seen(); !doctorContains(authed, "GET /api/users") {
				t.Fatalf("the default host was not probed with the tokens; requests carrying them: %v", authed)
			}
			if mode == "healthy" {
				if !doctorVerdictPositive(creds) || api != reached {
					t.Errorf("a working credential on the default host must read healthy and leave the api row alone: api=%q credentials=%q", api, creds)
				}
				return
			}
			if doctorVerdictPositive(creds) || doctorVerdictPositive(api) {
				t.Errorf("the default host rejected the credential and a row reads healthy: api=%q credentials=%q", api, creds)
			}
			if s, _ := api.(string); !strings.Contains(s, "DROPSUITE_HOST") || !strings.Contains(s, "DROPSUITE_BASE_URL") {
				t.Errorf("the default-host api verdict does not name DROPSUITE_HOST and DROPSUITE_BASE_URL: %q", api)
			}
		})
	}
}

// TestDoctorUnreachableDefaultHostNamesTheFix: the harness's placeholder state
// on Linux. The default host does not answer (no network, or a trust store that
// does not know it), and the api row must still name the host variables.
func TestDoctorUnreachableDefaultHostNamesTheFix(t *testing.T) {
	cfg := doctorDefaultHostConfig(t)
	report := map[string]any{"api": "unreachable: dial tcp: lookup dropsuite.us: no such host", "credentials": "skipped (API unreachable)"}
	doctorNameDefaultHost(cfg, report)
	api, _ := report["api"].(string)
	if doctorVerdictPositive(api) || !strings.Contains(api, "DROPSUITE_HOST") || !strings.Contains(api, "DROPSUITE_BASE_URL") {
		t.Errorf("an unreachable default host must stay negative and name DROPSUITE_HOST and DROPSUITE_BASE_URL: %q", api)
	}
	t.Setenv("DROPSUITE_HOST", "euw.dropmyemail.com")
	named, err := config.Load("")
	if err != nil {
		t.Fatal(err)
	}
	report = map[string]any{"api": "unreachable: dial tcp: lookup euw.dropmyemail.com: no such host"}
	doctorNameDefaultHost(named, report)
	if report["api"] != "unreachable: dial tcp: lookup euw.dropmyemail.com: no such host" {
		t.Errorf("a host the operator named is not the default host, yet the verdict was rewritten: %q", report["api"])
	}
}

func doctorContains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}
