// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/dnsfilter/handfixes.json
// under the "doctor-verdict-tracks-reality",
// "explicit-config-logout-never-falls-back" and
// "read-commands-open-store-read-only" entries.

package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"dnsfilter-pp-cli/internal/client"
	"dnsfilter-pp-cli/internal/cliutil"
	"dnsfilter-pp-cli/internal/cliutil/testenv"
	"dnsfilter-pp-cli/internal/learn"
)

// runR17 executes the command tree and returns stdout and the error.
func runR17(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := RootCmd()
	cmd.SetArgs(args)
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	err := cmd.Execute()
	return out.String(), err
}

func runR17JSON(t *testing.T, args ...string) (map[string]any, error) {
	t.Helper()
	out, err := runR17(t, args...)
	report := map[string]any{}
	if jerr := json.Unmarshal([]byte(out), &report); jerr != nil {
		t.Fatalf("%v: output is not JSON (%v): %s (command error: %v)", args, jerr, out, err)
	}
	return report, err
}

// r17Transport answers doctor's probes in memory for any host, so a
// cross-host redirect can be followed without DNS: api.dnsfilter.test
// redirects /v1/current_user to target, and every other request is a 200 {}.
type r17Transport struct{ target string }

func (rt r17Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{}`)), Request: req}
	if req.URL.Scheme == "http" || (req.URL.Host == "api.dnsfilter.test" && req.URL.Path == "/v1/current_user" && rt.target != "") {
		resp.StatusCode = http.StatusFound
		target := rt.target
		if req.URL.Scheme == "http" {
			target = "https://" + req.URL.Host + req.URL.Path
		}
		resp.Header.Set("Location", target)
	}
	return resp, nil
}

func useR17Transport(t *testing.T, rt http.RoundTripper) {
	t.Helper()
	saved := clientHooks
	t.Cleanup(func() { clientHooks = saved })
	clientHooks = append(append([]func(*client.Client) error(nil), saved...), func(c *client.Client) error {
		c.HTTPClient.Transport = rt
		return nil
	})
}

// A probe answered by a redirect destination proves nothing: when
// /v1/current_user redirects to another host that answers 200 {}, doctor
// reports neither a reachable API nor valid credentials. A same-host
// http->https upgrade of the same path is still the API.
func TestDoctorRedirectedProbeIsNotProof(t *testing.T) {
	testenv.Isolate(t)
	t.Setenv("DNSFILTER_API_KEY", "raw-key-r17")
	t.Setenv("DNSFILTER_BASE_URL", "https://api.dnsfilter.test")
	useR17Transport(t, r17Transport{target: "https://public-cdn.test/landing"})
	report, err := runR17JSON(t, "doctor", "--json")
	if err != nil {
		t.Fatalf("doctor: %v", err)
	}
	if api, _ := report["api"].(string); !strings.HasPrefix(api, "WARN") || !strings.Contains(api, "redirected to https://public-cdn.test/landing") {
		t.Fatalf("api verdict = %q, want a WARN naming the redirect destination", api)
	}
	if creds, _ := report["credentials"].(string); creds == "valid" || !strings.HasPrefix(creds, "WARN") {
		t.Fatalf("credentials verdict = %q, want a WARN, not valid", creds)
	}

	t.Setenv("DNSFILTER_BASE_URL", "http://api.dnsfilter.test")
	useR17Transport(t, r17Transport{})
	report, err = runR17JSON(t, "doctor", "--json")
	if err != nil {
		t.Fatalf("doctor: %v", err)
	}
	if report["api"] != "reachable" || report["credentials"] != "valid" {
		t.Fatalf("http->https upgrade: api=%v credentials=%v, want reachable/valid", report["api"], report["credentials"])
	}
}

func r17DoctorServer(t *testing.T, status int, body string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		fmt.Fprint(w, body)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("DNSFILTER_BASE_URL", srv.URL)
}

// A 403 is never "credentials valid": a permission refusal is a WARN that
// --fail-on warn trips on, and a 403 that says the request was not
// authenticated is an auth failure. A 5xx gives a remedy that exists.
func TestDoctorForbiddenAndServerErrorVerdicts(t *testing.T) {
	testenv.Isolate(t)
	t.Setenv("DNSFILTER_API_KEY", "raw-key-r17")
	t.Setenv(cliutil.DogfoodEnvVar, "1") // no 5xx retry backoff

	r17DoctorServer(t, http.StatusForbidden, `{"errors":[{"title":"Forbidden","detail":"You do not have permission to view this resource"}]}`)
	report, err := runR17JSON(t, "doctor", "--json", "--fail-on", "warn")
	if creds, _ := report["credentials"].(string); !strings.HasPrefix(creds, "WARN scope-limited") || strings.Contains(creds, "credentials are valid") {
		t.Fatalf("403 credentials verdict = %q, want a WARN that does not call the key valid", creds)
	}
	if err == nil || doctorExitForFailOn("warn", map[string]any{"credentials": report["credentials"]}) == nil {
		t.Fatalf("doctor --fail-on warn exited 0 on a 403 (%v)", err)
	}

	r17DoctorServer(t, http.StatusForbidden, `{"errors":[{"detail":"Invalid API token"}]}`)
	report, _ = runR17JSON(t, "doctor", "--json")
	if creds, _ := report["credentials"].(string); !strings.HasPrefix(creds, "invalid") {
		t.Fatalf("403 'Invalid API token' verdict = %q, want an auth failure", creds)
	}

	r17DoctorServer(t, http.StatusInternalServerError, `{"error":"boom"}`)
	report, _ = runR17JSON(t, "doctor", "--json")
	if creds, _ := report["credentials"].(string); !strings.HasPrefix(creds, "WARN not verified") || strings.Contains(creds, "verify_path") || !strings.Contains(creds, "server error") {
		t.Fatalf("500 credentials verdict = %q, want a WARN with a real remedy (no auth.verify_path)", creds)
	}
}

// r17Tenant saves "default-key" in the default credentials file and
// "tenant-key" in an explicit config's sibling, and returns the config path,
// the sibling and the default file.
func r17Tenant(t *testing.T) (configPath, sibling, defaultPath string) {
	t.Helper()
	home := testenv.Isolate(t, cliutil.ConfigDir, cliutil.DataDir, cliutil.StateDir, cliutil.CacheDir)
	t.Setenv("DNSFILTER_API_KEY", "")
	r17DoctorServer(t, http.StatusOK, `{"data":{"id":"1"}}`)
	var err error
	if defaultPath, err = cliutil.CredentialsFilePath(); err != nil {
		t.Fatal(err)
	}
	if err := cliutil.SaveCredentialsAt(defaultPath, &cliutil.Credentials{DnsfilterApiKey: "default-key"}); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(home, "tenant")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	configPath = filepath.Join(dir, "config.toml")
	if err := os.WriteFile(configPath, []byte("base_url = \"https://api.dnsfilter.com\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if sibling, err = cliutil.CredentialsFilePathForConfig(configPath); err != nil {
		t.Fatal(err)
	}
	if err := cliutil.SaveCredentialsAt(sibling, &cliutil.Credentials{DnsfilterApiKey: "tenant-key"}); err != nil {
		t.Fatal(err)
	}
	return configPath, sibling, defaultPath
}

// Under an explicit config doctor names the credentials file in use (the
// sibling) and lists it, not the default file the config never reads.
func TestDoctorReportsTheCredentialsFileInUse(t *testing.T) {
	configPath, sibling, defaultPath := r17Tenant(t)
	report, err := runR17JSON(t, "--config", configPath, "doctor", "--json")
	if err != nil {
		t.Fatalf("doctor: %v", err)
	}
	if report["credentials_file"] != sibling {
		t.Fatalf("credentials_file = %v, want the sibling %s", report["credentials_file"], sibling)
	}
	locations := fmt.Sprint(report["credentials_locations"])
	if !strings.Contains(locations, sibling) || strings.Contains(locations, defaultPath) {
		t.Fatalf("credentials_locations = %s, want the sibling and not the default file", locations)
	}
	report, err = runR17JSON(t, "doctor", "--json")
	if err != nil {
		t.Fatalf("doctor: %v", err)
	}
	if report["credentials_file"] != defaultPath {
		t.Fatalf("default config: credentials_file = %v, want %s", report["credentials_file"], defaultPath)
	}
}

// `--config X auth logout` logs X out for good: the next run does not fall
// back to the default file, and logout says when something still
// authenticates instead of claiming the credentials are cleared.
func TestLogoutUnderExplicitConfigTellsTheTruth(t *testing.T) {
	configPath, _, defaultPath := r17Tenant(t)
	out, err := runR17JSON(t, "--config", configPath, "auth", "logout", "--json")
	if err != nil {
		t.Fatalf("logout: %v", err)
	}
	if out["cleared"] != true || out["credentials_still_in_effect"] != false {
		t.Fatalf("logout envelope = %v, want cleared and nothing still in effect", out)
	}
	status, _ := runR17JSON(t, "--config", configPath, "auth", "status", "--json")
	if status["authenticated"] != false {
		t.Fatalf("after logout --config X still authenticates (source %v): it fell back to the default file", status["source"])
	}
	if has, err := cliutil.CredentialsFileHasValuesAt(defaultPath); err != nil || !has {
		t.Fatalf("logout under --config X touched the default credentials file (%v)", err)
	}

	t.Setenv("DNSFILTER_API_KEY", "env-key-r17")
	text, err := runR17(t, "--config", configPath, "auth", "logout")
	if err != nil {
		t.Fatalf("logout: %v", err)
	}
	if strings.Contains(text, "Logged out. Credentials cleared.") || !strings.Contains(text, "DNSFILTER_API_KEY") {
		t.Fatalf("logout with the key still in the environment printed %q", text)
	}
	out, _ = runR17JSON(t, "--config", configPath, "auth", "logout", "--json")
	if out["credentials_still_in_effect"] != true || !strings.Contains(fmt.Sprint(out["still_in_effect_from"]), "DNSFILTER_API_KEY") {
		t.Fatalf("logout envelope with the key in the environment = %v", out)
	}
}

// The post-run flag-correction pass never creates or migrates the default
// mirror after a read-only command; a command that may write the store still
// runs it.
func TestDeriveFlagCorrectionsSkipsReadOnlyCommands(t *testing.T) {
	testenv.Isolate(t, cliutil.DataDir, cliutil.StateDir)
	for _, name := range append([]string{"DNSFILTER_NO_LEARN", "DNSFILTER_LEARN_NO_CAPTURE", "DNSFILTER_LEARN_SESSION", cliutil.VerifyEnvVar, cliutil.DogfoodEnvVar}, learn.JournalHarnessSessionEnvVars...) {
		t.Setenv(name, "")
	}
	ts := func(ago time.Duration) string { return time.Now().UTC().Add(-ago).Format(time.RFC3339) }
	for _, e := range []learn.JournalEntry{
		{TS: ts(2 * time.Minute), SessionKey: "sess-r17", Cmd: []string{"search"}, ArgvShape: map[string]string{"limitt": "str"}, ExitCode: 2, ErrorClass: "usage", FailedFlag: "--limitt", SuggestedFlag: "--limit"},
		{TS: ts(time.Minute), SessionKey: "sess-r17", Cmd: []string{"search"}, ArgvShape: map[string]string{"limit": "str"}},
	} {
		if err := learn.AppendJournalEntry(e); err != nil {
			t.Fatal(err)
		}
	}
	root := RootCmd()
	dbPath := defaultDBPath("dnsfilter-cli")
	for _, path := range [][]string{{"search"}, {"doctor"}, {"current-user", "user-current-show"}} {
		executed, _, err := root.Find(path)
		if err != nil {
			t.Fatal(err)
		}
		deriveFlagCorrections(&rootFlags{}, root, executed)
		if _, err := os.Stat(dbPath); err == nil {
			t.Fatalf("the derivation pass after %v created the default mirror %s", path, dbPath)
		}
	}
	writer, _, err := root.Find([]string{"agent-local-user-bulk-deletes", "create"})
	if err != nil {
		t.Fatal(err)
	}
	deriveFlagCorrections(&rootFlags{}, root, writer)
	if _, err := os.Stat(dbPath); err != nil {
		t.Fatalf("the derivation pass after a write-capable command did not run (no %s): the test pairs nothing", dbPath)
	}
}
