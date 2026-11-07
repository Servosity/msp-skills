// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated). Recorded in handfixes.json (server-profiles).

package cli

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"windows-server-pp-cli/internal/client"
	"windows-server-pp-cli/internal/cliutil"
	"windows-server-pp-cli/internal/cliutil/testenv"
	"windows-server-pp-cli/internal/config"
	"windows-server-pp-cli/internal/store"
	"windows-server-pp-cli/internal/winsrv"
)

const testPassword = "Sh4red-WinRM-Pa55!"

const pin = "4bef9135fefa8b33948d5a136a86e64d77ca648d06130cf6d900827156dc35c2"

// encodings is every form of a secret the scans look for.
func encodings(s string) []string {
	var esc strings.Builder
	for _, r := range s {
		fmt.Fprintf(&esc, `\u%04x`, r)
	}
	// One numeric code per character or byte (round-1 review): quoted-
	// printable, CLIXML, a decimal code list and a 0x byte list.
	var qp, clixml, dec, hex0x []string
	for i := 0; i < len(s); i++ {
		qp = append(qp, fmt.Sprintf("=%02X", s[i]))
		hex0x = append(hex0x, fmt.Sprintf("0x%02x", s[i]))
	}
	for _, r := range s {
		clixml = append(clixml, fmt.Sprintf("_x%04X_", r))
		dec = append(dec, fmt.Sprint(int(r)))
	}
	return []string{s, base64.StdEncoding.EncodeToString([]byte(s)), base64.RawURLEncoding.EncodeToString([]byte(s)),
		hex.EncodeToString([]byte(s)), strings.ToUpper(hex.EncodeToString([]byte(s))), url.QueryEscape(s), esc.String(),
		strings.Join(qp, ""), strings.Join(clixml, ""), strings.Join(dec, " "), strings.Join(hex0x, ",")}
}

// leakyFixtures echo the password back in every encoding, the way a service
// description, task argument or event message could.
func leakyFixtures(secret string) *winsrv.FixtureRunner {
	enc := encodings(secret)
	svc := []map[string]any{{"name": "Spooler", "display_name": "Print Spooler", "state": "Stopped", "start_mode": "Auto", "exit_code": 1067}}
	for i, e := range enc {
		svc = append(svc, map[string]any{"name": fmt.Sprintf("Leak%d", i), "display_name": "runs as " + e, "state": "Stopped", "start_mode": "Auto", "exit_code": 1})
	}
	envelope := func(v any) string { // ok first, as Out-Envelope writes it
		b, _ := json.Marshal(v)
		return `{"ok":true,"data":` + string(b) + `}`
	}
	events := []map[string]any{{"log": "System", "record_id": 1, "level": 2, "provider": "Service Control Manager", "event_id": 7034, "message": "pw " + enc[1]}}
	return &winsrv.FixtureRunner{Outputs: map[string]string{
		"system":              envelope(map[string]any{"computer_name": "FS01", "pending_reboot": false, "domain": enc[3]}),
		"volumes":             envelope([]map[string]any{{"drive_letter": "C", "percent_free": 42.0, "health_status": "Healthy", "label": enc[0]}}),
		"services":            envelope(svc),
		"scm_events":          envelope([]map[string]any{{"event_id": 7034, "record_id": 9, "time_created": "2026-10-28T01:00:00Z", "service_key": "spooler", "params": []string{"Print Spooler"}, "message": "terminated " + enc[2]}}),
		"eventlog":            envelope(events),
		"installed_updates":   envelope([]map[string]any{{"hotfix_id": "KB5031364", "installed_on": "2026-10-20T00:00:00Z", "installed_by": enc[5]}}),
		"pending_updates":     envelope([]map[string]any{}),
		"features":            envelope([]map[string]any{{"name": "FS-FileServer", "display_name": "File Server", "install_state": "Installed"}}),
		"local_admins":        envelope([]map[string]any{{"name": "CORP\\" + enc[0], "sid": "S-1-5-21-1", "object_class": "User"}}),
		"local_admin_changes": envelope(map[string]any{"audit_policy": "Success", "window_hours": 168, "member_sids": []string{"S-1-5-21-1"}, "changes": []map[string]any{{"record_id": 5, "action": "added", "member_sid": "S-1-5-21-1", "member_name": enc[6], "changed_by": "CORP\\admin"}}}),
		"shares":              envelope([]map[string]any{{"name": "data", "path": "D:\\data", "description": enc[4]}}),
		"scheduled_tasks":     envelope([]map[string]any{{"task_path": "\\", "task_name": "Backup", "state": "Ready", "last_result": 1, "last_run_time": "2026-10-27T00:00:00Z", "run_as": enc[0]}}),
		"certificates":        envelope([]map[string]any{{"store": "My", "thumbprint": "AB", "subject": "CN=" + enc[1], "not_after": "2027-01-01T00:00:00Z", "has_private_key": true}}),
		"defender":            `{"ok":false,"error":{"kind":"role_absent","message":"not installed ` + enc[0] + `"}}`,
		"time_sync":           envelope(map[string]any{"service_state": "Running", "lines": []string{"Source: " + enc[0], "Leap Indicator: 0(no warning)"}}),
		"hyperv_vms":          `{"ok":false,"error":{"kind":"role_absent","message":"no role"}}`,
		"eventlog_groups":     envelope(map[string]any{"capped": false, "baseline_start": "2026-10-20T00:00:00Z", "oldest_record": map[string]any{"System": "2026-01-01T00:00:00Z"}, "groups": []map[string]any{{"log": "System", "provider": "Disk", "event_id": 153, "window_count": 3, "baseline_count": 0, "sample_message": enc[3]}}}),
		"service_get":         envelope(map[string]any{"name": "Spooler", "display_name": enc[1], "state": "Running", "start_mode": "Auto"}),
		"service_plan":        envelope(map[string]any{"name": "Spooler", "action": "restart", "state_before": "Running", "allowed": true, "display_name": enc[0]}),
		"service_action":      envelope(map[string]any{"name": "Spooler", "action": "restart", "state_before": "Running", "state_after": "Running", "allowed": true, "changed": true, "display_name": enc[2]}),
	}}
}

// winsrvTestEnv isolates the home directory, writes a two-server config and
// installs the fixture runner. It returns the isolated home.
func winsrvTestEnv(t *testing.T, f *winsrv.FixtureRunner) string {
	t.Helper()
	home := testenv.Isolate(t)
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config.toml")
	body := "[[servers]]\nname = \"fs01\"\nhost = \"fs01.corp.example\"\ncert_sha256 = \"" + pin + "\"\ndefault = true\n\n[[servers]]\nname = \"fs02\"\nhost = \"10.0.0.2\"\ncert_sha256 = \"" + pin + "\"\n"
	if err := os.WriteFile(cfg, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WINDOWS_SERVER_CONFIG", cfg)
	t.Setenv("WINDOWS_SERVER_USERNAME", `CORP\svc-monitor`)
	t.Setenv("WINDOWS_SERVER_PASSWORD", testPassword)
	winsrvServer, winsrvAllowHTTP = "", false
	prev := newWinsrvRunner
	newWinsrvRunner = func(*rootFlags) winsrv.Runner { return f }
	t.Cleanup(func() { newWinsrvRunner = prev })
	return home
}

// storedLogin swaps the shared WINDOWS_SERVER_USERNAME/PASSWORD pair for a
// stored credential per server (servers login). The generated client turns the
// shared pair into a Basic header for the virtual base URL; the kit binds that
// header to the base URL credkit.Start reads, and these in-process tests run
// no Start, so the kit's transport would withhold every typed read. A stored
// credential reaches only the virtual transport, which resolves it itself.
func storedLogin(t *testing.T, servers ...string) {
	t.Helper()
	t.Setenv("WINDOWS_SERVER_USERNAME", "")
	t.Setenv("WINDOWS_SERVER_PASSWORD", "")
	for _, name := range servers {
		cmd := RootCmd()
		cmd.SetArgs([]string{"servers", "login", name, "--username", `CORP\svc-monitor`})
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetIn(strings.NewReader(testPassword + "\n"))
		if err := cmd.Execute(); err != nil {
			t.Fatalf("servers login %s: %v\n%s", name, err, out.String())
		}
	}
}

func runCLI(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := RootCmd()
	cmd.SetArgs(args)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetIn(strings.NewReader(""))
	err := cmd.Execute()
	if err != nil {
		out.WriteString("\nERROR: " + err.Error())
	}
	return out.String(), err
}

func assertNoSecret(t *testing.T, where, text string) {
	t.Helper()
	for _, e := range encodings(testPassword) {
		if strings.Contains(text, e) {
			t.Errorf("%s carries the password (as %q):\n%s", where, e, text)
		}
	}
}

func TestVirtualTransportRefusesOtherHosts(t *testing.T) {
	winsrvTestEnv(t, leakyFixtures(testPassword))
	vt := &virtualTransport{}
	for _, raw := range []string{"https://evil.example/system", "http://winrm.invalid/system", "https://127.0.0.1/system"} {
		req, _ := httpNewRequest(raw)
		if _, err := vt.RoundTrip(req); err == nil || !strings.Contains(err.Error(), "refused") {
			t.Errorf("%s: want refused, got %v", raw, err)
		}
	}
}

func TestServiceActionNeedsConfirmation(t *testing.T) {
	f := leakyFixtures(testPassword)
	winsrvTestEnv(t, f)
	out, err := runCLI(t, "services", "restart", "Spooler", "--agent")
	if err == nil || !strings.Contains(out, "nothing was changed") {
		t.Fatalf("want a preview and refusal, got %v:\n%s", err, out)
	}
	for _, c := range f.Calls {
		if c.Script == "service_action" {
			t.Fatal("the change ran without --yes or --confirm")
		}
	}
	if _, err := runCLI(t, "services", "start", "Spooler", "--dry-run"); err != nil {
		t.Fatal(err)
	}
	if len(f.Calls) != 1 {
		t.Fatalf("dry-run contacted the server: %+v", f.Calls)
	}
	if _, err := runCLI(t, "services", "restart", "Spooler", "--confirm", "--agent"); err != nil {
		t.Fatal(err)
	}
	if last := f.Calls[len(f.Calls)-1]; last.Script != "service_action" || last.Server != "fs01" {
		t.Fatalf("confirmed change did not run on the default server: %+v", last)
	}
	for _, bad := range []string{"Spooler;whoami", "$(whoami)", "Spoo*", "a b"} {
		before := len(f.Calls)
		if _, err := runCLI(t, "services", "restart", bad, "--yes"); err == nil || len(f.Calls) != before {
			t.Errorf("%q: want refusal before any call", bad)
		}
	}
}

func TestServerFlagNeverTakesAHost(t *testing.T) {
	winsrvTestEnv(t, leakyFixtures(testPassword))
	for _, bad := range []string{"fs01.corp.example", "10.0.0.2", "https://fs01", "fs01:5986", "FS01"} {
		if _, err := runCLI(t, "--server", bad, "volumes"); err == nil {
			t.Errorf("--server %q accepted", bad)
		}
	}
}

func TestAllowHTTPRefusedUnderMCP(t *testing.T) {
	winsrvTestEnv(t, leakyFixtures(testPassword))
	t.Setenv(cliutil.MCPChildEnvVar, "1")
	if _, err := runCLI(t, "--allow-http", "volumes"); err == nil {
		t.Fatal("--allow-http accepted in an MCP child")
	}
	root := RootCmd()
	if f := root.PersistentFlags().Lookup("allow-http"); f == nil || !f.Hidden {
		t.Fatal("--allow-http must exist and be hidden from the MCP tool schema")
	}
}

func TestShortPasswordIsRefused(t *testing.T) {
	// Padding does not count: the filter trims before it matches.
	for _, pw := range []string{"short", "Passw0r ", "  abc   "} {
		winsrvTestEnv(t, leakyFixtures(testPassword))
		t.Setenv("WINDOWS_SERVER_PASSWORD", pw)
		out, err := runCLI(t, "health")
		if err == nil || !strings.Contains(out, "8 characters") {
			t.Fatalf("%q: want refusal, got %v %s", pw, err, out)
		}
	}
}

func TestHealthWritesEventsOnChange(t *testing.T) {
	f := leakyFixtures(testPassword)
	winsrvTestEnv(t, f)
	if _, err := runCLI(t, "health", "--server", "fs01", "--agent"); err != nil {
		t.Fatal(err)
	}
	out, _ := runCLI(t, "events", "list", "--json")
	var first struct {
		Events []map[string]any `json:"events"`
	}
	if err := json.Unmarshal([]byte(out), &first); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if len(first.Events) == 0 {
		t.Fatalf("a first run must report the checks already failing:\n%s", out)
	}
	for _, e := range first.Events {
		d, _ := e["data"].(map[string]any)
		if d["from"] != nil || !winsrv.Failing(fmt.Sprint(d["to"])) && e["name"] == EventCheckChanged {
			t.Fatalf("a first run reports only failing state, from null: %v", e)
		}
	}
	if strings.Contains(out, `"to":"ok"`) || strings.Contains(out, `"disks"`) {
		t.Fatalf("a first run must not report a passing check:\n%s", out)
	}
	f.Outputs["volumes"] = `{"ok":true,"data":[{"drive_letter":"C","percent_free":3.0,"health_status":"Healthy"}]}`
	if _, err := runCLI(t, "health", "--server", "fs01", "--agent"); err != nil {
		t.Fatal(err)
	}
	out, _ = runCLI(t, "events", "list", "--json")
	var got struct {
		Events []map[string]any `json:"events"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	found := false
	for _, e := range got.Events {
		if e["name"] == EventCheckChanged && strings.Contains(fmt.Sprint(e["data"]), "disks") {
			found = true
		}
	}
	if !found {
		t.Fatalf("no disks check_changed event:\n%s", out)
	}
}

func httpNewRequest(raw string) (*http.Request, error) {
	return http.NewRequest(http.MethodGet, raw, nil)
}

// TestStoredCredentialIsBoundToItsHost: a password saved for "fs01" under one
// config is never sent to a server also named "fs01" at another host.
func TestStoredCredentialIsBoundToItsHost(t *testing.T) {
	winsrvTestEnv(t, leakyFixtures(testPassword))
	t.Setenv("WINDOWS_SERVER_USERNAME", "")
	t.Setenv("WINDOWS_SERVER_PASSWORD", "")
	const acme = "AcmeOnlyPassw0rd!"
	cmd := RootCmd()
	cmd.SetArgs([]string{"servers", "login", "fs01", "--username", "acme-admin"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetIn(strings.NewReader(acme + "\n"))
	if err := cmd.Execute(); err != nil {
		t.Fatalf("login: %v\n%s", err, out.String())
	}
	resolve := func(path string) winsrv.Profile {
		t.Helper()
		cfg, err := config.Load(path)
		if err != nil {
			t.Fatal(err)
		}
		profiles, _, err := serversFromConfig(cfg, "fs01", false)
		if err != nil {
			t.Fatal(err)
		}
		return profiles[0]
	}
	if p := resolve(os.Getenv("WINDOWS_SERVER_CONFIG")); p.Password != acme {
		t.Fatalf("the server the credential was saved for does not get it: %q", p.CredentialFrom)
	}
	other := filepath.Join(t.TempDir(), "globex.toml")
	body := "[[servers]]\nname = \"fs01\"\nhost = \"fs01.globex.example\"\ncert_sha256 = \"" + pin + "\"\n"
	if err := os.WriteFile(other, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if p := resolve(other); p.Password == acme {
		t.Fatalf("acme's fs01 password was resolved for %s", p.Host)
	}
	// Same name at the same address under another client's config (RFC1918
	// overlap, or a short name each client's DNS resolves): still not shared.
	same := filepath.Join(t.TempDir(), "globex.toml")
	body = "[[servers]]\nname = \"fs01\"\nhost = \"fs01.corp.example\"\ncert_sha256 = \"" + pin + "\"\n"
	if err := os.WriteFile(same, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if p := resolve(same); p.Password == acme {
		t.Fatalf("acme's fs01 password was resolved for the same address under another config")
	}
	// Logging out globex's fs01 leaves acme's alone.
	if out, err := runCLI(t, "servers", "logout", "fs01", "--config", same); err != nil {
		t.Fatalf("logout: %v %s", err, out)
	}
	if p := resolve(os.Getenv("WINDOWS_SERVER_CONFIG")); p.Password != acme {
		t.Fatalf("another config's logout removed acme's credential")
	}
}

// TestRotatingThePasswordKeepsTheStore: the health history survives a new
// shared password; another config gets its own store.
func TestRotatingThePasswordKeepsTheStore(t *testing.T) {
	winsrvTestEnv(t, leakyFixtures(testPassword))
	t.Cleanup(func() { setDefaultDBScopeIdentity("", "", "") })
	path := func() string { configureDefaultDBScope(""); return defaultDBPath("windows-server-cli") }
	before := path()
	if filepath.Base(before) == "data.db" {
		t.Fatalf("the store is not scoped at all: %s", before)
	}
	t.Setenv("WINDOWS_SERVER_PASSWORD", "R0tated-WinRM-Pa55!")
	if after := path(); after != before {
		t.Fatalf("a new password moved the store: %s -> %s", before, after)
	}
	other := filepath.Join(t.TempDir(), "other.toml")
	if err := os.WriteFile(other, []byte("[[servers]]\nname = \"fs01\"\nhost = \"fs01.other.example\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WINDOWS_SERVER_CONFIG", other)
	if got := path(); got == before {
		t.Fatalf("two configs share one store: %s", got)
	}
}

// TestVirtualTransportDoesNotCutScriptBudgets: the generated client's
// whole-call timeout (60s) would end a 5-minute Windows Update search early.
func TestVirtualTransportDoesNotCutScriptBudgets(t *testing.T) {
	c := &client.Client{HTTPClient: &http.Client{Timeout: time.Minute}}
	if err := installVirtualTransport(c); err != nil {
		t.Fatal(err)
	}
	if c.HTTPClient.Timeout != 0 {
		t.Fatalf("client timeout %s still bounds every script run", c.HTTPClient.Timeout)
	}
}

// TestExplicitTimeoutReachesTypedReads: --timeout bounds a single-resource
// read's script run (the virtual transport has no whole-call timeout).
func TestExplicitTimeoutReachesTypedReads(t *testing.T) {
	f := leakyFixtures(testPassword)
	winsrvTestEnv(t, f)
	storedLogin(t, "fs01", "fs02")
	var got []time.Duration
	newWinsrvRunner = func(fl *rootFlags) winsrv.Runner {
		if fl != nil && fl.timeoutExplicit {
			got = append(got, fl.timeout)
		} else {
			got = append(got, 0)
		}
		return f
	}
	if out, err := runCLI(t, "volumes", "--timeout", "3s", "--json"); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	if len(got) == 0 || got[len(got)-1] != 3*time.Second {
		t.Fatalf("the script run did not get --timeout 3s: %v", got)
	}
	got = nil
	if out, err := runCLI(t, "volumes", "--json"); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	if len(got) == 0 || got[len(got)-1] != 0 {
		t.Fatalf("an earlier --timeout leaked into the next run: %v", got)
	}
}

// TestNoLocalDataRecordsNothing: WINDOWS_SERVER_NO_LOCAL_DATA=1 lets health
// print its verdicts but records no run and no event; a snapshot is refused.
func TestNoLocalDataRecordsNothing(t *testing.T) {
	winsrvTestEnv(t, leakyFixtures(testPassword))
	t.Setenv("WINDOWS_SERVER_NO_LOCAL_DATA", "1")
	out, err := runCLI(t, "health", "--server", "fs01", "--json")
	if err != nil || !strings.Contains(out, `"verdict"`) {
		t.Fatalf("health under no-local-data: %v %s", err, out)
	}
	if out, _ := runCLI(t, "events", "list", "--json"); strings.Contains(out, "check_changed") {
		t.Fatalf("health recorded events with local data off:\n%s", out)
	}
	if _, err := runCLI(t, "inventory", "snapshot", "--server", "fs01"); err == nil {
		t.Fatal("inventory snapshot ran with local data off")
	}
}

// TestFanOutExitsNonZeroWhenNoServerAnswered: a run that read nothing from
// any server does not exit 0.
func TestFanOutExitsNonZeroWhenNoServerAnswered(t *testing.T) {
	winsrvTestEnv(t, &winsrv.FixtureRunner{Outputs: map[string]string{}})
	for _, args := range [][]string{{"health"}, {"inventory", "snapshot"}, {"services", "down"}, {"eventlog", "new"}, {"local-admins", "changes"}} {
		if out, err := runCLI(t, append(args, "--json")...); err == nil {
			t.Errorf("%v exited 0 with no server answering:\n%s", args, out)
		}
	}
}

// TestPaddedPasswordIsRefusedAtLogin: spaces do not count toward the minimum.
func TestPaddedPasswordIsRefusedAtLogin(t *testing.T) {
	winsrvTestEnv(t, leakyFixtures(testPassword))
	cmd := RootCmd()
	cmd.SetArgs([]string{"servers", "login", "fs01", "--username", "u"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetIn(strings.NewReader("Passw0r \n"))
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error()+out.String(), "8 characters") {
		t.Fatalf("a 7-character password padded to 8 was not refused for its length: %v\n%s", err, out.String())
	}
}

// doctorReport runs doctor --json and decodes its report.
func doctorReport(t *testing.T) map[string]any {
	t.Helper()
	out, _ := runCLI(t, "doctor", "--json")
	var report map[string]any
	if err := json.Unmarshal([]byte(out[strings.Index(out, "{"):strings.LastIndex(out, "}")+1]), &report); err != nil {
		t.Fatalf("doctor --json is not JSON: %v\n%s", err, out)
	}
	return report
}

// TestDoctorNeverCallsTheVirtualHostReachable: the virtual host is answered
// in process, so doctor reports the configured servers and points at health
// instead of reading the in-process reply to GET / as "reachable".
func TestDoctorNeverCallsTheVirtualHostReachable(t *testing.T) {
	winsrvTestEnv(t, leakyFixtures(testPassword))
	report := doctorReport(t)
	api, _ := report["api"].(string)
	if strings.Contains(api, "reachable") || !strings.Contains(api, "2 servers configured, not verified here") || !strings.Contains(api, "health") {
		t.Fatalf("api row: %q", api)
	}
	if creds, _ := report["credentials"].(string); !strings.HasPrefix(creds, "present, not verified") {
		t.Fatalf("credentials row: %q", creds)
	}

	t.Setenv("WINDOWS_SERVER_PASSWORD", "")
	report = doctorReport(t)
	if creds, _ := report["credentials"].(string); !strings.HasPrefix(creds, "missing for fs01, fs02") {
		t.Fatalf("credentials row without a password: %q", creds)
	}

	empty := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(empty, []byte("# no servers\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WINDOWS_SERVER_CONFIG", empty)
	report = doctorReport(t)
	if api, _ := report["api"].(string); !strings.HasPrefix(api, "not configured (add a [[servers]] entry") {
		t.Fatalf("api row with no servers: %q", api)
	}
}

// TestDoctorCountsPerServerCredentials: a credential stored with servers
// login for every server is configured auth, not a missing shared pair.
func TestDoctorCountsPerServerCredentials(t *testing.T) {
	winsrvTestEnv(t, leakyFixtures(testPassword))
	storedLogin(t, "fs01", "fs02")
	report := doctorReport(t)
	if report["auth"] != "configured" || report["auth_source"] != "per-server credential store" {
		t.Fatalf("auth %v (source %v)", report["auth"], report["auth_source"])
	}
	if env, _ := report["env_vars"].(string); strings.Contains(env, "missing") {
		t.Fatalf("env_vars: %q", env)
	}
	if out, _ := runCLI(t, "doctor", "--json"); strings.Contains(out, testPassword) {
		t.Fatal("doctor printed the stored password")
	}
}

// TestInventoryDiffWithNothingToCompare: a store with no snapshot and no
// configured server answers "no inventory recorded yet" (exit 3), the same
// as a missing store, never an empty "no drift" list; a configured server
// with no snapshot says so in its note.
func TestInventoryDiffWithNothingToCompare(t *testing.T) {
	winsrvTestEnv(t, leakyFixtures(testPassword))
	_, _ = runCLI(t, "inventory", "diff", "--json") // scopes the store to this config
	db, err := store.Open(defaultDBPath("windows-server-cli"))
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	out, err := runCLI(t, "inventory", "diff", "--json")
	if err != nil {
		t.Fatalf("inventory diff with configured servers: %v\n%s", err, out)
	}
	if !strings.Contains(out, "no snapshot recorded yet; diff needs two") || strings.Contains(out, "snapshot(s)") {
		t.Fatalf("zero-snapshot note:\n%s", out)
	}

	empty := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(empty, []byte("# no servers\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WINDOWS_SERVER_CONFIG", empty)
	_, _ = runCLI(t, "inventory", "diff", "--json") // scopes the store to this config
	db, err = store.Open(defaultDBPath("windows-server-cli"))
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	out, err = runCLI(t, "inventory", "diff", "--json")
	var ce *cliError
	if !errors.As(err, &ce) || ce.code != 3 || !strings.Contains(out, "no inventory recorded yet") {
		t.Fatalf("no servers and no snapshots: want exit 3 'no inventory recorded yet', got %v\n%s", err, out)
	}
}

// TestCappedReadsNeverReadAsComplete is the round-1 class for the commands:
// a newest-first event read that hit its cap is never reported as "no such
// event". services down must not claim the Service Control Manager logged
// nothing, and local-admins changes must warn that older changes are cut.
func TestCappedReadsNeverReadAsComplete(t *testing.T) {
	env := func(v any) string {
		b, _ := json.Marshal(v)
		return `{"ok":true,"data":` + string(b) + `}`
	}
	var scm []map[string]any
	for i := 0; i < 500; i++ {
		scm = append(scm, map[string]any{"event_id": 7031, "record_id": 9000 - i, "time_created": "2026-10-28T01:00:00Z", "service_key": fmt.Sprintf("other%d", i), "params": []string{fmt.Sprintf("Other %d", i)}, "message": "terminated"})
	}
	winsrvTestEnv(t, &winsrv.FixtureRunner{Outputs: map[string]string{
		"services":            env([]map[string]any{{"name": "Spooler", "display_name": "Print Spooler", "state": "Stopped", "start_mode": "Auto", "exit_code": 1067}}),
		"scm_events":          env(scm),
		"local_admin_changes": env(map[string]any{"audit_policy": "Success", "window_hours": 168, "capped": true, "member_sids": []string{}, "changes": []map[string]any{}}),
	}})
	out, err := runCLI(t, "services", "down", "--server", "fs01", "--agent")
	if err != nil || strings.Contains(out, "no Service Control Manager error") || !strings.Contains(out, "cap") {
		t.Errorf("services down over a capped read:\n%s", out)
	}
	out, err = runCLI(t, "local-admins", "changes", "--server", "fs01", "--agent")
	if err != nil || !strings.Contains(out, "cap") {
		t.Errorf("local-admins changes over a capped read:\n%s", out)
	}
}

// TestServicesDownWaitsOutTheDelayedStart is the lab's boot-time input for
// services down: a delayed automatic service with exit code 1077 (not
// started since boot) on a server up three minutes is not down; the same
// service three hours after boot is, and so is a stop with a failure code.
// When the uptime cannot be read, the stop is listed (fail toward reporting).
func TestServicesDownWaitsOutTheDelayedStart(t *testing.T) {
	env := func(v any) string {
		b, _ := json.Marshal(v)
		return `{"ok":true,"data":` + string(b) + `}`
	}
	services := env([]map[string]any{
		{"name": "DPS", "display_name": "Diagnostic Policy Service", "state": "Stopped", "start_mode": "Auto", "delayed_auto_start": true, "trigger_start": false, "exit_code": 1077},
		{"name": "Spooler", "display_name": "Print Spooler", "state": "Stopped", "start_mode": "Auto", "delayed_auto_start": false, "trigger_start": false, "exit_code": 1067},
	})
	system := func(uptime int) string {
		return env(map[string]any{"computer_name": "FS01", "last_boot": "2026-10-28T08:48:56Z", "uptime_seconds": uptime, "pending_reboot": false})
	}
	for _, tc := range []struct {
		label  string
		system string // "" = the system read fails
		dps    bool
	}{
		{"uptime 3 min", system(180), false},
		{"uptime 3 h", system(3 * 3600), true},
		{"uptime unknown", "", true},
	} {
		outputs := map[string]string{"services": services, "scm_events": env([]map[string]any{})}
		if tc.system != "" {
			outputs["system"] = tc.system
		}
		winsrvTestEnv(t, &winsrv.FixtureRunner{Outputs: outputs})
		out, err := runCLI(t, "services", "down", "--server", "fs01", "--agent")
		if err != nil || !strings.Contains(out, "Spooler") || strings.Contains(out, "DPS") != tc.dps {
			t.Errorf("%s: DPS listed = %v, want %v (Spooler always):\n%s", tc.label, strings.Contains(out, "DPS"), tc.dps, out)
		}
	}
	// No stop the uptime could explain: the system script is not read.
	f := &winsrv.FixtureRunner{Outputs: map[string]string{
		"services":   env([]map[string]any{{"name": "Spooler", "display_name": "Print Spooler", "state": "Stopped", "start_mode": "Auto", "exit_code": 1067}}),
		"scm_events": env([]map[string]any{}),
		"system":     system(180),
	}}
	winsrvTestEnv(t, f)
	if out, err := runCLI(t, "services", "down", "--server", "fs01", "--agent"); err != nil || !strings.Contains(out, "Spooler") {
		t.Fatalf("services down:\n%s", out)
	}
	for _, c := range f.Calls {
		if c.Script == "system" {
			t.Errorf("the system script was read with no delayed service waiting to start")
		}
	}
}

// TestNotFoundHintNamesOnlyRealCommands: the generated 404 hint said "Run
// the 'list' command", and this CLI has no list command. A 404 here is a
// role or product that is not installed (hyperv-vms, defender) or a name the
// server does not have (services get). Every command the hint names must
// resolve in the real command tree, read through the real virtual transport.
func TestNotFoundHintNamesOnlyRealCommands(t *testing.T) {
	quoted := regexp.MustCompile(`'([^']*)'`)
	root := RootCmd()
	check := func(label, out string) {
		t.Helper()
		_, hint, ok := strings.Cut(out, "hint:")
		if !ok {
			t.Errorf("%s: no hint:\n%s", label, out)
			return
		}
		named := quoted.FindAllStringSubmatch(hint, -1)
		if len(named) == 0 {
			t.Errorf("%s: the hint names no command: %s", label, hint)
		}
		for _, m := range named {
			words := strings.Fields(m[1])
			if len(words) < 2 || words[0] != root.Name() {
				t.Errorf("%s: the hint names %q, which is not a %s command", label, m[1], root.Name())
				continue
			}
			cmd, rest, err := root.Find(words[1:])
			if err != nil || len(rest) != 0 || cmd.CommandPath() != m[1] {
				t.Errorf("%s: the hint names %q, which does not resolve (found %q, rest %v, %v)", label, m[1], cmd.CommandPath(), rest, err)
			}
		}
	}
	env := func(v any) string {
		b, _ := json.Marshal(v)
		return `{"ok":true,"data":` + string(b) + `}`
	}
	winsrvTestEnv(t, &winsrv.FixtureRunner{Outputs: map[string]string{
		"hyperv_vms":  `{"ok":false,"error":{"kind":"role_absent","message":"The Hyper-V role is not installed"}}`,
		"defender":    `{"ok":false,"error":{"kind":"role_absent","message":"Microsoft Defender Antivirus is not installed"}}`,
		"service_get": `{"ok":false,"error":{"kind":"not_found","message":"No service with that name"}}`,
		"services":    env([]map[string]any{}),
	}})
	storedLogin(t, "fs01")
	for _, args := range [][]string{{"hyperv-vms"}, {"defender"}, {"services", "get", "NoSuchSvc"}} {
		out, err := runCLI(t, append(args, "--server", "fs01", "--data-source", "live", "--no-cache")...)
		if err == nil || ExitCode(err) != 3 {
			t.Errorf("%v: want a not-found exit (3), got %v:\n%s", args, err, out)
		}
		check(strings.Join(args, " "), out)
		if roleAbsent := args[0] != "services"; roleAbsent != strings.Contains(out, "is not installed on that server") {
			t.Errorf("%v: the role-absent hint is shown = %v, want %v:\n%s", args, !roleAbsent, roleAbsent, out)
		}
	}
	// The classifier on its own, for both 404 shapes.
	for _, msg := range []string{
		`GET /hyperv-vms returned HTTP 404: hyperv_vms on fs01: role_absent: The Hyper-V role is not installed`,
		`GET /services/NoSuchSvc returned HTTP 404: service_get on fs01: not_found: No service with that name`,
	} {
		check(msg, classifyAPIErrorOnly(errors.New(msg)).Error())
	}
}

// A base URL other than the virtual host is refused by the transport, so doctor
// must not read the refusal as "API unreachable" with a credential verdict that
// claims nothing; it says the credential is not verified over HTTP.
func TestDoctorRefusesANonVirtualBaseURL(t *testing.T) {
	t.Setenv("PRINTING_PRESS_VERIFY", "")
	for _, base := range []string{"https://127.0.0.1:8443", "https://vendor.example.com"} {
		api, creds, ok := winsrvDoctorReachability(&config.Config{BaseURL: base})
		if !ok || !strings.HasPrefix(api, "refused:") || !strings.HasPrefix(creds, "WARN not verified") {
			t.Fatalf("%s: got ok=%v api=%q creds=%q", base, ok, api, creds)
		}
	}
}
