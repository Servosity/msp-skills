// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"eset-protect-pp-cli/internal/client"
	"eset-protect-pp-cli/internal/cliutil/testenv"
)

// runDoctorJSON runs `doctor --json` (plus extra args) and decodes the report.
func runDoctorJSON(t *testing.T, extra ...string) (map[string]any, error) {
	t.Helper()
	cmd := RootCmd()
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs(append([]string{"doctor", "--json"}, extra...))
	err := cmd.Execute()
	var report map[string]any
	if jerr := json.Unmarshal(out.Bytes(), &report); jerr != nil {
		t.Fatalf("doctor --json is not one JSON object: %v\n%s\n%s", jerr, out.String(), errOut.String())
	}
	return report, err
}

// doctorFakeESET serves the device-management probe with the given status
// and body (JSON unless it starts with "<") and points the password grant at
// the shared fake token server.
func doctorFakeESET(t *testing.T, probeStatus int, probeBody string) {
	t.Helper()
	withTokenServer(t)
	if probeBody == "" {
		probeBody = `{"deviceGroups":[],"nextPageToken":""}`
	}
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/device_groups" {
			if strings.HasPrefix(probeBody, "<") {
				w.Header().Set("Content-Type", "text/html")
			}
			w.WriteHeader(probeStatus)
			_, _ = io.WriteString(w, probeBody)
			return
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"code":5,"message":"Not Found"}`)
	}))
	t.Cleanup(api.Close)
	t.Setenv("ESET_PROTECT_BASE_URL", api.URL)
	t.Setenv("ESET_PROTECT_REGION", "eu")
	t.Setenv("ESET_PROTECT_TOKEN", "")
}

// TestDoctorVerdictTracksReality (handfixes.json: doctor-verdict-tracks-reality):
// the Auth row must see the ESET password grant, a rejected mint must read as
// a credential failure (not "not configured" plus a client init error), the
// Credentials row must come from a real authenticated read, and a base URL
// that answers that read with HTTP 404 or an HTML page is not the ESET API:
// the API row must be negative and name ESET_PROTECT_BASE_URL, however "/"
// answered.
func TestDoctorVerdictTracksReality(t *testing.T) {
	const wrongBase = "ERROR wrong base URL"
	cases := []struct {
		name, user, password string
		probe                int
		probeBody            string
		auth, api            string
		credentials          string
		failOnError          bool
	}{
		{"rejected password", "api@msp.example", "wrong", http.StatusOK, "", "ERROR ESET Connect token request (eu region) rejected", "reachable", "ERROR rejected", true},
		{"valid password", "api@msp.example", "s3cret", http.StatusOK, "", "configured", "reachable", "valid", false},
		{"token refused by API", "api@msp.example", "s3cret", http.StatusUnauthorized, "", "configured", "reachable", "ERROR rejected", true},
		{"no API scope", "api@msp.example", "s3cret", http.StatusForbidden, "", "configured", "reachable", "WARN scope-limited", false},
		// "FAIL Auth: not configured" must trip --fail-on error (doctor-fail-on-matches-rows).
		{"no credentials", "", "", http.StatusOK, "", "not configured", "reachable", "", true},
		{"wrong base: probe 404", "api@msp.example", "s3cret", http.StatusNotFound, `{"error":"not found"}`, "configured", wrongBase, "ERROR not verified", true},
		{"wrong base: probe HTML", "api@msp.example", "s3cret", http.StatusOK, "<!doctype html><html><body>app</body></html>", "configured", wrongBase, "ERROR not verified", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			testenv.Isolate(t)
			respondClearHarness(t)
			doctorFakeESET(t, tc.probe, tc.probeBody)
			t.Setenv("ESET_PROTECT_USERNAME", tc.user)
			t.Setenv("ESET_PROTECT_PASSWORD", tc.password)

			report, err := runDoctorJSON(t)
			if err != nil {
				t.Fatalf("doctor without --fail-on must exit 0: %v", err)
			}
			auth, _ := report["auth"].(string)
			creds, _ := report["credentials"].(string)
			api, _ := report["api"].(string)
			if !strings.HasPrefix(auth, tc.auth) {
				t.Errorf("auth = %q, want prefix %q", auth, tc.auth)
			}
			if tc.credentials == "" && creds != "" {
				t.Errorf("credentials = %q, want no row without credentials", creds)
			}
			if tc.credentials != "" && !strings.HasPrefix(creds, tc.credentials) {
				t.Errorf("credentials = %q, want prefix %q", creds, tc.credentials)
			}
			if !strings.HasPrefix(api, tc.api) {
				t.Errorf("api = %q, want prefix %q", api, tc.api)
			}
			if tc.api == wrongBase && (!strings.Contains(api, "ESET_PROTECT_BASE_URL") || !strings.Contains(api, "https://eu.device-management.eset.systems")) {
				t.Errorf("api = %q, want the remedy: unset ESET_PROTECT_BASE_URL or point it at https://eu.device-management.eset.systems", api)
			}
			_, err = runDoctorJSON(t, "--fail-on", "error")
			if tc.failOnError != (err != nil) {
				t.Errorf("doctor --fail-on error returned %v, want failure=%v", err, tc.failOnError)
			}
		})
	}
}

// doctorRegionRT answers ESET Connect locally so the default-region path can
// be exercised without dialing a real ESET host.
type doctorRegionRT struct {
	mu    sync.Mutex
	hosts []string
}

func (r *doctorRegionRT) RoundTrip(req *http.Request) (*http.Response, error) {
	r.mu.Lock()
	r.hosts = append(r.hosts, req.URL.Host)
	r.mu.Unlock()
	status, body := http.StatusNotFound, `{"code":5,"message":"Not Found"}`
	if req.URL.Path == "/v1/device_groups" {
		status, body = http.StatusOK, `{"deviceGroups":[],"nextPageToken":""}`
	}
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{"Content-Type": {"application/json"}}, Request: req}, nil
}

// TestDoctorDefaultRegionIsInfo (handfixes.json: doctor-verdict-tracks-reality,
// doctor-fail-on-matches-rows): with neither ESET_PROTECT_REGION nor a base URL
// override, the CLI uses the documented eu default. That is a valid
// configuration, so the API row is an INFO (not a WARN that would fail every
// such install under --fail-on warn) that says so and names both
// ESET_PROTECT_REGION and ESET_PROTECT_BASE_URL, and the credential probe must
// still run. A chosen region gets no note.
func TestDoctorDefaultRegionIsInfo(t *testing.T) {
	for _, tc := range []struct {
		region, host string
		warn         bool
	}{{"", "eu.device-management.eset.systems", true}, {"us", "us.device-management.eset.systems", false}} {
		t.Run("region="+tc.region, func(t *testing.T) {
			testenv.Isolate(t)
			respondClearHarness(t)
			t.Setenv("ESET_PROTECT_BASE_URL", "")
			t.Setenv("ESET_PROTECT_REGION", tc.region)
			t.Setenv("ESET_PROTECT_TOKEN", "test-token")
			t.Setenv("ESET_PROTECT_USERNAME", "")
			t.Setenv("ESET_PROTECT_PASSWORD", "")
			rec := &doctorRegionRT{}
			saved := clientHooks
			t.Cleanup(func() { clientHooks = saved })
			clientHooks = append(append([]func(*client.Client) error{}, saved...), func(c *client.Client) error {
				if tr, ok := c.HTTPClient.Transport.(*client.EsetTransport); ok {
					tr.Base = rec
				}
				return nil
			})

			report, err := runDoctorJSON(t)
			if err != nil {
				t.Fatalf("doctor without --fail-on must exit 0: %v", err)
			}
			api, _ := report["api"].(string)
			creds, _ := report["credentials"].(string)
			if !strings.HasPrefix(creds, "valid") {
				t.Errorf("credentials = %q: the probe must still run and verify the token", creds)
			}
			if tc.warn {
				for _, want := range []string{"INFO ", "default eu region", "ESET_PROTECT_REGION", "eu, de, us, ca or jpn", "ESET_PROTECT_BASE_URL"} {
					if !strings.Contains(api, want) {
						t.Errorf("api = %q, want it to contain %q", api, want)
					}
				}
				if !strings.HasPrefix(api, "INFO ") || strings.Contains(api, "placeholder") {
					t.Errorf("api = %q: a defaulted region is an INFO row, never a WARN or a placeholder claim", api)
				}
			} else if strings.HasPrefix(api, "WARN") || strings.HasPrefix(api, "INFO") || strings.Contains(api, "ESET_PROTECT_REGION is not set") {
				t.Errorf("api = %q: a chosen region must not carry the note", api)
			} else if !strings.Contains(api, "(region "+tc.region+" from ESET_PROTECT_REGION)") {
				t.Errorf("api = %q: the row must name the region and that ESET_PROTECT_REGION chose it", api)
			}
			// The documented default must pass the strictest gate a script uses.
			if _, err := runDoctorJSON(t, "--fail-on", "warn"); err != nil {
				t.Errorf("doctor --fail-on warn failed on a healthy install (region %q): %v", tc.region, err)
			}
			rec.mu.Lock()
			defer rec.mu.Unlock()
			if len(rec.hosts) == 0 {
				t.Fatal("doctor sent nothing through the ESET transport")
			}
			for _, h := range rec.hosts {
				if h != tc.host {
					t.Errorf("doctor dialed %q, want %q", h, tc.host)
				}
			}
		})
	}
}

// TestDoctorFailOnMatchesRenderedRows (handfixes.json: doctor-fail-on-matches-rows):
// --fail-on gates on the verdict each row is printed with. Every FAIL row trips
// --fail-on error, every WARN row trips --fail-on warn and not error, and INFO
// and OK rows trip neither, for every row doctor renders.
func TestDoctorFailOnMatchesRenderedRows(t *testing.T) {
	samples := map[string]string{
		"not configured": doctorVerdictFail,
		"not configured (set base_url in config file)": doctorVerdictFail,
		"ERROR token expired at x":                     doctorVerdictFail,
		"refused: credential present but not loaded":   doctorVerdictFail,
		"unreachable: dial tcp":                        doctorVerdictFail,
		"client init error: boom":                      doctorVerdictFail,
		"WARN scope-limited":                           doctorVerdictWarn,
		"skipped (API unreachable)":                    doctorVerdictFail,
		"skipped (base_url is a placeholder)":          doctorVerdictWarn,
		"INFO ESET_PROTECT_REGION is not set":          doctorVerdictInfo,
		"present, not verified":                        doctorVerdictInfo,
		"optional":                                     doctorVerdictInfo,
		"reachable":                                    doctorVerdictOK,
		"valid":                                        doctorVerdictOK,
		"configured":                                   doctorVerdictOK,
		"not required":                                 doctorVerdictOK,
	}
	for s, want := range samples {
		if got := doctorRowVerdict(s); got != want {
			t.Errorf("doctorRowVerdict(%q) = %s, want %s", s, got, want)
		}
		for _, ck := range doctorCheckKeys {
			report := map[string]any{ck.key: s}
			errGate := doctorExitForFailOn("error", report) != nil
			warnGate := doctorExitForFailOn("warn", report) != nil
			switch want {
			case doctorVerdictFail:
				if !errGate || !warnGate {
					t.Errorf("%s row %q prints FAIL but --fail-on error=%v warn=%v", ck.label, s, errGate, warnGate)
				}
			case doctorVerdictWarn:
				if errGate || !warnGate {
					t.Errorf("%s row %q prints WARN but --fail-on error=%v warn=%v", ck.label, s, errGate, warnGate)
				}
			default:
				if errGate || warnGate {
					t.Errorf("%s row %q prints %s but --fail-on error=%v warn=%v", ck.label, s, want, errGate, warnGate)
				}
			}
		}
	}
}

// TestDoctorRegionSourceAfterATokenSave (adversarial round 4, P2): a token
// saved under ESET_PROTECT_REGION=ca used to write region ca into
// config.toml, so a later run with the variable unset went to ca while the
// docs promise eu, and doctor said the region came from the config file. The
// save no longer writes it: the next run is the eu default, its requests go
// to eu, and doctor's region note says the default was used.
func TestDoctorRegionSourceAfterATokenSave(t *testing.T) {
	home := testenv.Isolate(t)
	respondClearHarness(t)
	for _, k := range []string{"ESET_PROTECT_BASE_URL", "ESET_PROTECT_TOKEN", "ESET_PROTECT_USERNAME", "ESET_PROTECT_PASSWORD"} {
		t.Setenv(k, "")
	}
	t.Setenv("ESET_PROTECT_REGION", "ca")
	cmd := RootCmd()
	cmd.SetIn(strings.NewReader("test-token-from-stdin\n"))
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{"auth", "set-token", "--home", home})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("auth set-token: %v\n%s", err, errOut.String())
	}
	t.Setenv("ESET_PROTECT_REGION", "")
	rec := &doctorRegionRT{}
	saved := clientHooks
	t.Cleanup(func() { clientHooks = saved })
	clientHooks = append(append([]func(*client.Client) error{}, saved...), func(c *client.Client) error {
		if tr, ok := c.HTTPClient.Transport.(*client.EsetTransport); ok {
			tr.Base = rec
		}
		return nil
	})
	report, err := runDoctorJSON(t, "--home", home)
	if err != nil {
		t.Fatalf("doctor: %v", err)
	}
	api, _ := report["api"].(string)
	if !strings.Contains(api, "default eu region") || strings.Contains(api, "config file") {
		t.Fatalf("api = %q: after a save under ESET_PROTECT_REGION=ca the unset variable must mean the eu default", api)
	}
	rec.mu.Lock()
	defer rec.mu.Unlock()
	for _, h := range rec.hosts {
		if !strings.HasPrefix(h, "eu.") {
			t.Fatalf("doctor dialed %q after the variable was unset; want the eu default", h)
		}
	}
}
