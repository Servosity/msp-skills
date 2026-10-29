// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"cisco-umbrella-pp-cli/internal/cliutil/testenv"
)

// These tests prove doctor-verdict-tracks-reality (handfixes.json) on the
// report doctor BUILDS, before the credential kit sees it. Execute installs
// that filter on os.Stdout; RootCmd with SetOut never reaches it. The filter
// redacts every JSON member named like a credential, doctor's `credentials`
// row included, so tools/maintainer/check_doctor_truth.py reads "[redacted]"
// in every state from the binary. The polarity of the verdict itself is
// asserted here, against a stub Umbrella that mints client-credentials tokens the
// way a real Umbrella API does.

// Fixture literals are split so a scanner sees no whole synthetic secret.
var (
	doctorTestClientID     = "doctor-truth-" + "key-id-5e1a"
	doctorTestClientSecret = "doctor-truth-" + "client-value-7c2e"
	doctorTestAccessToken  = "doctor-truth-" + "minted-4b9d"
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

// doctorStub is a Umbrella API: /auth/v2/token mints a bearer from the
// client ID and secret, and the API answers per mode.
//
//	healthy        200 to a request carrying the minted bearer, 401 otherwise
//	expired        401 to every API request (the credential is dead)
//	wrong-base     200 HTML at /, 404 at every other path (a web UI, not the API)
//	mint-rejected  the token endpoint refuses the client ID and secret
type doctorStub struct {
	mode     string
	mu       sync.Mutex
	requests []string
	bearer   []string
}

func (s *doctorStub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	authed := r.Header.Get("Authorization") == "Bearer "+doctorTestAccessToken
	s.mu.Lock()
	s.requests = append(s.requests, r.Method+" "+r.URL.Path)
	if authed && r.URL.Path != "/" {
		s.bearer = append(s.bearer, r.Method+" "+r.URL.Path)
	}
	s.mu.Unlock()
	if r.Method == http.MethodPost && r.URL.Path == "/auth/v2/token" {
		w.Header().Set("Content-Type", "application/json")
		if s.mode == "mint-rejected" {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `{"error":"invalid_client"}`)
			return
		}
		fmt.Fprint(w, `{"access_token":"`+doctorTestAccessToken+`","token_type":"Bearer","expires_in":3600}`)
		return
	}
	switch s.mode {
	case "healthy":
		if !authed {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[]`)
	case "wrong-base":
		if r.URL.Path == "/" {
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprint(w, "<!doctype html><html><body>Umbrella</body></html>")
			return
		}
		w.WriteHeader(http.StatusNotFound)
	default:
		w.WriteHeader(http.StatusUnauthorized)
	}
}

func (s *doctorStub) seen() (requests, bearer []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.requests...), append([]string(nil), s.bearer...)
}

// doctorTestInstall points the CLI at the stub the way an operator does:
// client credentials in the environment, the token endpoint on the phone
// system, and (unless withBase is false) CISCO_UMBRELLA_BASE_URL at its API root.
func doctorTestInstall(t *testing.T, mode string, withBase, withCredential bool) *doctorStub {
	t.Helper()
	testenv.Isolate(t)
	stub := &doctorStub{mode: mode}
	srv := httptest.NewServer(stub)
	t.Cleanup(srv.Close)
	for _, name := range []string{"PRINTING_PRESS_VERIFY", "PRINTING_PRESS_VERIFY_LIVE_HTTP",
		"CISCO_UMBRELLA_ORG_ID", "CISCO_UMBRELLA_BASE_URL", "CISCO_UMBRELLA_API_KEY", "CISCO_UMBRELLA_API_SECRET", "CISCO_UMBRELLA_OAUTH_SCOPE"} {
		t.Setenv(name, "")
	}
	t.Setenv("CISCO_UMBRELLA_TOKEN_URL", srv.URL+"/auth/v2/token")
	if withBase {
		t.Setenv("CISCO_UMBRELLA_BASE_URL", srv.URL)
	}
	if withCredential {
		t.Setenv("CISCO_UMBRELLA_API_KEY", doctorTestClientID)
		t.Setenv("CISCO_UMBRELLA_API_SECRET", doctorTestClientSecret)
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

// TestDoctorVerdictTracksReality is the four-state invariant of
// check_doctor_truth.py, on the report itself: `credentials` is positive only
// when the credential actually worked, `api` only when the connector reached
// this vendor's API, and the three dialled states never share a verdict.
func TestDoctorVerdictTracksReality(t *testing.T) {
	verdicts := map[string]any{}
	for _, mode := range []string{"healthy", "expired", "wrong-base"} {
		t.Run(mode, func(t *testing.T) {
			stub := doctorTestInstall(t, mode, true, true)
			report := runDoctorReport(t)
			creds, api := report["credentials"], report["api"]
			verdicts[mode] = creds
			if creds == nil {
				t.Fatalf("doctor emitted no credentials row; report: %v", report)
			}
			_, bearer := stub.seen()
			switch mode {
			case "healthy":
				if !doctorVerdictPositive(creds) {
					t.Errorf("credentials should verify against a working API, got %q", creds)
				}
				if !doctorVerdictPositive(api) {
					t.Errorf("api should be healthy against a working API, got %q", api)
				}
				// The probe must carry the minted bearer to a real endpoint, and
				// the endpoint must be /deployments/v2/networks: the read this connector's
				// own `fleet health` treats as the Umbrella API's liveness call.
				if !containsString(bearer, "GET /deployments/v2/networks") {
					t.Errorf("the credential probe never sent the minted bearer to GET /deployments/v2/networks; bearer requests: %v", bearer)
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
				if s, _ := api.(string); !strings.Contains(s, "CISCO_UMBRELLA_BASE_URL") {
					t.Errorf("the wrong-base api verdict does not name CISCO_UMBRELLA_BASE_URL: %q", api)
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

// TestDoctorPlaceholderNamesTheFixAndDialsNothing: a base URL left at a reserved
// example host must not be dialled (or a token minted for it), must say neither
// row is healthy, and must name the variable that fixes it.
func TestDoctorPlaceholderNamesTheFixAndDialsNothing(t *testing.T) {
	stub := doctorTestInstall(t, "healthy", false, true)
	t.Setenv("CISCO_UMBRELLA_BASE_URL", "https://umbrella.example.com")
	report := runDoctorReport(t)
	creds, api := report["credentials"], report["api"]
	if creds == nil {
		t.Fatalf("doctor emitted no credentials row; report: %v", report)
	}
	if doctorVerdictPositive(creds) {
		t.Errorf("base_url is a placeholder and credentials read healthy: %q", creds)
	}
	if doctorVerdictPositive(api) {
		t.Errorf("base_url is a placeholder and the api reads healthy: %q", api)
	}
	if s, _ := api.(string); !strings.Contains(s, "CISCO_UMBRELLA_BASE_URL") {
		t.Errorf("the placeholder api verdict does not name CISCO_UMBRELLA_BASE_URL: %q", s)
	}
	if requests, _ := stub.seen(); len(requests) != 0 {
		t.Errorf("doctor dialled with a placeholder base URL: %v", requests)
	}
}

// TestDoctorCountsClientCredentialsAsConfigured: a Umbrella install authenticates
// with a client ID and secret and holds no bearer until the first request
// mints one. Doctor used to read that as "auth: not configured" and tell the
// operator to set the two variables they had already set.
func TestDoctorCountsClientCredentialsAsConfigured(t *testing.T) {
	doctorTestInstall(t, "healthy", true, true)
	report := runDoctorReport(t)
	if report["auth"] != "configured" {
		t.Errorf("client ID and secret are set but auth reads %q (hint %q)", report["auth"], report["auth_hint"])
	}
	if _, ok := report["auth_hint"]; ok {
		t.Errorf("doctor tells a configured operator to set credentials: %q", report["auth_hint"])
	}
}

// TestDoctorWithoutACredentialSaysSo: a reachable API and no credential at
// all must produce a negative credentials row, not silence.
func TestDoctorWithoutACredentialSaysSo(t *testing.T) {
	stub := doctorTestInstall(t, "healthy", true, false)
	report := runDoctorReport(t)
	creds := report["credentials"]
	if creds == nil {
		t.Fatalf("no credential is configured and doctor emitted no credentials row; report: %v", report)
	}
	if doctorVerdictPositive(creds) {
		t.Errorf("no credential is configured and credentials read healthy: %q", creds)
	}
	if _, bearer := stub.seen(); len(bearer) != 0 {
		t.Errorf("no credential is configured, yet a bearer reached the API: %v", bearer)
	}
}

// TestDoctorRejectedClientSecretIsNotHealthy: the commonest Umbrella failure, an
// API key that was rotated or mistyped, must leave a negative credentials row.
func TestDoctorRejectedClientSecretIsNotHealthy(t *testing.T) {
	doctorTestInstall(t, "mint-rejected", true, true)
	report := runDoctorReport(t)
	creds := report["credentials"]
	if creds == nil {
		t.Fatalf("the token endpoint refused the client secret and doctor emitted no credentials row; report: %v", report)
	}
	if doctorVerdictPositive(creds) {
		t.Errorf("the token endpoint refused the client secret and credentials read healthy: %q", creds)
	}
}

func containsString(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}
