// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"threecx-pp-cli/internal/cliutil/testenv"
	"threecx-pp-cli/internal/fleet"
)

func status(version, lastBackup string, trunksUp int) string {
	return `{"@odata.context":"x","Version":"` + version + `","LicenseActive":true,"LicenseKey":"AAAA-BBBB",
	"ExpirationDate":"2099-01-01T00:00:00Z","MaintenanceExpiresAt":"2099-01-01T00:00:00Z","LastBackupDateTime":"` + lastBackup + `",
	"BackupScheduled":true,"TrunksRegistered":` + string(rune('0'+trunksUp)) + `,"ExtensionsRegistered":8,"ExtensionsTotal":10,"TrunksTotal":2,"TotalDiskSpace":100,"FreeDiskSpace":60,
	"HasNotRunningServices":false,"HasUnregisteredSystemExtensions":false,"RecordingStopped":false,"RecordingQuotaReached":false,
	"VoicemailStopped":false,"VoicemailQuotaReached":false,"Firmware3CXOutdated":false,"MaxSimCalls":8,"CallsActive":1}`
}

// setupFleet registers acme, globex, and an unreachable "down" PBX and points
// the fleet runner at the fake child.
func setupFleet(t *testing.T) string {
	t.Helper()
	testenv.Isolate(t)
	dir := t.TempDir()
	t.Setenv(fleet.DirEnv, dir)
	t.Setenv("FIXTURES", dir)
	for _, k := range []string{fleet.SelectEnv, "THREECX_FQDN", "THREECX_CLIENT_ID", "THREECX_CLIENT_SECRET", "THREECX_BASE_URL",
		"THREECX_TOKEN_URL", "THREECX_CONFIG", "THREECX_HOME", "THREECX_USER_AGENT", "THREECX_ACME_CLIENT_SECRET", "THREECX_GLOBEX_CLIENT_SECRET"} {
		t.Setenv(k, "")
		_ = os.Unsetenv(k)
	}
	prevPin, prevSel := pinnedPBX, pbxSelection
	t.Cleanup(func() { pinnedPBX, pbxSelection = prevPin, prevSel })
	pinnedPBX, pbxSelection = "", ""
	reg := &fleet.Registry{}
	for _, n := range []string{"acme", "globex", "down"} {
		reg.Upsert(fleet.PBX{Name: n, FQDN: n + ".3cx.us", ClientID: "900", EnvPrefix: "THREECX_" + strings.ToUpper(n)})
	}
	if err := reg.Save(); err != nil {
		t.Fatal(err)
	}
	recent := time.Now().UTC().Add(-2 * time.Hour).Format(time.RFC3339)
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("acme_SystemStatus.json", status("20.0.10.1479", recent, 2))
	write("globex_SystemStatus.json", status("20.0.8.1109", "2020-01-01T00:00:00Z", 1))
	write("acme_Users.json", `{"value":[{"Number":"100","FirstName":"Ada","VMPIN":"1234","AccessPassword":"pw-acme"}]}`)
	write("globex_Users.json", `{"value":[{"Number":"200","FirstName":"Grace","VMPIN":"9999"}]}`)
	write("acme_Trunks.json", `{"value":[{"Number":"10000","IsOnline":true,"ConfigurationIssue":"","AuthPassword":"pw-trunk"}]}`)
	write("globex_Trunks.json", `{"value":[{"Number":"20000","IsOnline":false,"ConfigurationIssue":"Registration failed"}]}`)
	// The child is this test binary, which TestMain turns into fakeChildMain:
	// no shell, so the fleet tests run on Windows too.
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(fakeChildEnv, "1")
	prev := fleet.SelfExecutable
	fleet.SelfExecutable = func() (string, error) { return exe, nil }
	t.Cleanup(func() { fleet.SelfExecutable = prev })
	return dir
}

func runCLI(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := RootCmd()
	cmd.SetArgs(args)
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	err := cmd.Execute()
	if err != nil {
		// The rows carry each PBX's reason (redacted); keep it in the error so
		// a failure on another platform says why (exitCode still unwraps).
		err = fmt.Errorf("%w\nstdout: %.800s\nstderr: %.800s", err, out.String(), errOut.String())
	}
	return out.String(), err
}

func exitCode(err error) int {
	var ce *cliError
	if errors.As(err, &ce) {
		return ce.code
	}
	return -1
}

func TestFleetHealthFanOut(t *testing.T) {
	setupFleet(t)
	out, err := runCLI(t, "fleet", "health", "--json", "--no-save")
	if err != nil {
		t.Fatalf("fleet health: %v\n%s", err, out)
	}
	var rows []fleet.HealthRow
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("not JSON rows: %v\n%s", err, out)
	}
	if len(rows) != 3 || rows[0].PBX != "down" || rows[1].PBX != "globex" || rows[2].PBX != "acme" {
		t.Fatalf("rows = %+v", rows)
	}
	if rows[0].Status != fleet.Error || !strings.Contains(rows[0].Error, "HTTP 403") {
		t.Fatalf("down row = %+v", rows[0])
	}
	if rows[1].Status != fleet.Fail || !strings.Contains(rows[1].Problems, "backup:fail") || !strings.Contains(rows[1].Problems, "trunks:fail") {
		t.Fatalf("globex row = %+v", rows[1])
	}
	if rows[2].Status != fleet.Pass {
		t.Fatalf("acme row = %+v", rows[2])
	}
	if strings.Contains(out, "AAAA-BBBB") {
		t.Fatal("licence key leaked into health output")
	}

	_, err = runCLI(t, "fleet", "health", "--json", "--no-save", "--only", "acme", "--fail-on", "warn")
	if err != nil {
		t.Fatalf("healthy PBX must not trip --fail-on: %v", err)
	}
	_, err = runCLI(t, "fleet", "health", "--json", "--no-save", "--fail-on", "fail")
	if exitCode(err) != exitFleetProblems {
		t.Fatalf("--fail-on fail exit = %d (%v), want %d", exitCode(err), err, exitFleetProblems)
	}
	if _, err := runCLI(t, "fleet", "health", "--only", "nope"); exitCode(err) != 2 {
		t.Fatalf("unknown --only must be a usage error, got %v", err)
	}
}

func TestFleetExecMergesAndRedacts(t *testing.T) {
	dir := setupFleet(t)
	// A parent shell pointed at one PBX must not bleed into the children.
	t.Setenv("THREECX_CLIENT_SECRET", "parent-secret")
	t.Setenv("THREECX_BASE_URL", "http://parent.example/xapi/v1")
	t.Setenv("THREECX_USER_AGENT", "ua-from-parent")
	t.Setenv("THREECX_ACME_CLIENT_SECRET", "acme-key")
	t.Setenv("THREECX_GLOBEX_CLIENT_SECRET", "globex-key")
	out, err := runCLI(t, "fleet", "exec", "/Users", "--only", "acme,globex", "--param", "$select=Number,FirstName", "--json")
	if err != nil {
		t.Fatalf("fleet exec: %v\n%s", err, out)
	}
	var rows []map[string]any
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out)
	}
	if len(rows) != 2 || rows[0]["pbx"] != "acme" || rows[1]["pbx"] != "globex" || rows[1]["Number"] != "200" {
		t.Fatalf("rows = %v", rows)
	}
	if strings.Contains(out, "1234") || strings.Contains(out, "pw-acme") {
		t.Fatalf("secrets leaked: %s", out)
	}
	calls, _ := os.ReadFile(filepath.Join(dir, "calls.log"))
	if !strings.Contains(string(calls), "acme fleet exec --raw --timeout 1m0s --req /Users?%24select=Number%2CFirstName --json --no-input") {
		t.Fatalf("child call shape = %s", calls)
	}
	for _, bad := range []string{"https://evil/x", "/Users?$top=1", "/../connect/token"} {
		if _, err := runCLI(t, "fleet", "exec", bad); exitCode(err) != 2 {
			t.Errorf("path %q must be a usage error, got %v", bad, err)
		}
	}
}

func TestFleetTrunksSelectsStatusFieldsOnly(t *testing.T) {
	dir := setupFleet(t)
	out, err := runCLI(t, "fleet", "trunks", "--offline", "--json", "--no-save")
	if err != nil {
		t.Fatalf("fleet trunks: %v\n%s", err, out)
	}
	var rows []fleet.TrunkRow
	_ = json.Unmarshal([]byte(out), &rows)
	if len(rows) != 2 || rows[0].PBX != "down" || rows[1].Trunk != "20000" {
		t.Fatalf("--offline rows = %+v", rows)
	}
	if strings.Contains(out, "pw-trunk") {
		t.Fatal("trunk password leaked")
	}
	calls, _ := os.ReadFile(filepath.Join(dir, "calls.log"))
	if !strings.Contains(string(calls), "--req /Trunks?%24select="+url.QueryEscape(trunkSelect)) || strings.Contains(trunkSelect, "Auth") {
		t.Fatalf("trunk request must select status fields only: %s", calls)
	}
	if _, err := runCLI(t, "fleet", "trunks", "--no-save", "--fail-on-offline", "--json"); exitCode(err) != exitFleetProblems {
		t.Fatalf("--fail-on-offline exit = %v", err)
	}
}

func TestFleetChangesAfterTwoRuns(t *testing.T) {
	dir := setupFleet(t)
	out, err := runCLI(t, "fleet", "changes", "--json")
	if err != nil || strings.TrimSpace(out) != "[]" {
		t.Fatalf("no history must be an empty list: %q %v", out, err)
	}
	if _, err := runCLI(t, "fleet", "health", "--json", "--only", "acme,globex"); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(dir, "acme_SystemStatus.json"), []byte(status("20.0.11.200", time.Now().UTC().Format(time.RFC3339), 1)), 0o600)
	if _, err := runCLI(t, "fleet", "health", "--json", "--only", "acme,globex"); err != nil {
		t.Fatal(err)
	}
	out, err = runCLI(t, "fleet", "changes", "--since", "0s", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"version_changed"`) || !strings.Contains(out, `"trunks_worsened"`) || strings.Contains(out, `"globex"`) {
		t.Fatalf("changes = %s", out)
	}
}

func TestApplyPBXSelection(t *testing.T) {
	dir := setupFleet(t)
	t.Setenv("THREECX_ACME_CLIENT_SECRET", "s3cret")
	t.Setenv("THREECX_BASE_URL", "http://evil.example")
	prev := pbxSelection
	t.Cleanup(func() { pbxSelection = prev })

	pbxSelection = "acme"
	flags := &rootFlags{}
	if err := applyPBXSelection(&cobra.Command{Use: "x"}, flags); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("THREECX_FQDN") != "acme.3cx.us" || os.Getenv("THREECX_CLIENT_SECRET") != "s3cret" || os.Getenv("THREECX_CLIENT_ID") != "900" {
		t.Fatal("selection did not set the PBX's host and credential")
	}
	if os.Getenv("THREECX_BASE_URL") != "https://acme.3cx.us/xapi/v1" || os.Getenv("THREECX_TOKEN_URL") != "https://acme.3cx.us/connect/token" {
		t.Fatalf("endpoints must be pinned from the registry, got %q %q", os.Getenv("THREECX_BASE_URL"), os.Getenv("THREECX_TOKEN_URL"))
	}
	acme := fleet.PBX{Name: "acme", FQDN: "acme.3cx.us", ClientID: "900"}
	if flags.homePath != acme.Home(dir) || os.Getenv(fleet.DirEnv) != dir {
		t.Fatalf("home = %q, fleet dir = %q", flags.homePath, os.Getenv(fleet.DirEnv))
	}

	pbxSelection = "globex" // THREECX_GLOBEX_CLIENT_SECRET unset
	if err := applyPBXSelection(&cobra.Command{Use: "x"}, &rootFlags{}); exitCode(err) != 4 {
		t.Fatalf("missing secret must be an auth error, got %v", err)
	}
	if err := applyPBXSelection(&cobra.Command{Use: "x"}, &rootFlags{dryRun: true}); err != nil {
		t.Fatalf("--dry-run must not need the secret: %v", err)
	}
	pbxSelection = "nope"
	if err := applyPBXSelection(&cobra.Command{Use: "x"}, &rootFlags{}); exitCode(err) != 2 {
		t.Fatalf("unknown PBX must be a usage error, got %v", err)
	}
	pbxSelection = "acme"
	if err := applyPBXSelection(&cobra.Command{Use: "x"}, &rootFlags{homePath: "/tmp/x"}); exitCode(err) != 2 {
		t.Fatalf("--pbx with --home must be a usage error, got %v", err)
	}
}

// TestRawGetChild drives the real child primitive against a mock PBX:
// client-credentials mint, the GET with its OData query, and redaction.
func TestRawGetChild(t *testing.T) {
	testenv.Isolate(t)
	var gotQuery, gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/connect/token":
			if id, secret, ok := r.BasicAuth(); !ok || id != "900" || secret != "s3" {
				w.WriteHeader(400)
				_, _ = w.Write([]byte(`{"error":"invalid_client"}`))
				return
			}
			_, _ = w.Write([]byte(`{"access_token":"tok","expires_in":3600}`))
		case "/xapi/v1/Users":
			gotQuery, gotAuth = r.URL.RawQuery, r.Header.Get("Authorization")
			_, _ = w.Write([]byte(`{"value":[{"Number":"100","VMPIN":"1234","Enabled":true}]}`))
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	prevPin := pinnedPBX
	t.Cleanup(func() { pinnedPBX = prevPin })
	pinnedPBX = "acme"
	t.Setenv("THREECX_BASE_URL", srv.URL+"/xapi/v1")
	t.Setenv("THREECX_TOKEN_URL", srv.URL+"/connect/token")
	t.Setenv("THREECX_CLIENT_ID", "900")
	t.Setenv("THREECX_CLIENT_SECRET", "s3")
	cmd := &cobra.Command{Use: "x"}
	cmd.SetContext(context.Background())
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := runRawGets(cmd, &rootFlags{timeout: 10 * time.Second}, []string{"/Users?%24top=1"}); err != nil {
		t.Fatal(err)
	}
	if gotAuth != "Bearer tok" || gotQuery != "%24top=1" {
		t.Fatalf("auth=%q query=%q", gotAuth, gotQuery)
	}
	if strings.Contains(out.String(), "1234") || !strings.Contains(out.String(), `"Number":"100"`) {
		t.Fatalf("raw output = %s", out.String())
	}
	pinnedPBX = ""
	if err := runRawGets(cmd, &rootFlags{}, []string{"/Users"}); exitCode(err) != 2 {
		t.Fatalf("--raw without a PBX must be a usage error, got %v", err)
	}
}

func TestFleetExecRefusesUnsafePaths(t *testing.T) {
	setupFleet(t)
	for _, bad := range []string{
		"/%2e%2e/%2e%2e/connect/token", "/ConferenceSettings/Pbx.GenerateApiKey()", "/SystemStatus/Pbx.APIToken()",
		"/ConferenceSettings/Pbx.GenerateApiKey/", "/ConferenceSettings/Pbx.GenerateApiKey;a=1", "/Trunks(1)/AuthPassword/", "/Users//Pbx.GenerateProvLink",
		"/MyTokens", "/SecurityTokens", "/Users(1)/Pbx.GenerateProvLink()", "/SystemStatus/Pbx.GetRequestHelpLink(grantPeriodDays=7)",
		"/Trunks/Pbx.InitTrunk(template='x')", "/Users/Pbx.ExportExtensions()", "/Microsoft365Integration/Pbx.GetMicrosoftAccessToken()",
		"/Parameters(1)/Value", "/Trunks(1)/AuthPassword", "/Users(1)/$value", "/BackupSettings/Contents",
	} {
		if _, err := runCLI(t, "fleet", "exec", bad); exitCode(err) != 2 {
			t.Errorf("path %q must be refused, got %v", bad, err)
		}
	}
	for _, ok := range []string{"/Users", "/Trunks(10)", "/SystemStatus/Pbx.SystemHealthStatus()", "/ReportCallLogData/Pbx.GetCallLogData(periodFrom=2026-01-01)"} {
		if _, err := cleanXAPIPath(ok); err != nil {
			t.Errorf("path %q should be allowed: %v", ok, err)
		}
	}
}

func TestFleetAllFailedAndAgentKeepsErrors(t *testing.T) {
	setupFleet(t)
	out, err := runCLI(t, "fleet", "health", "--only", "down", "--json", "--no-save")
	if exitCode(err) != 5 || !strings.Contains(out, "HTTP 403") {
		t.Fatalf("every PBX failing must exit 5 after printing rows; got %v\n%s", err, out)
	}
	out, err = runCLI(t, "fleet", "trunks", "--agent", "--no-save", "--offline")
	if err != nil {
		t.Fatalf("partial failure must stay exit 0: %v", err)
	}
	for _, want := range []string{`"error"`, `"configuration_issue"`, `"state"`} {
		if !strings.Contains(out, want) {
			t.Errorf("--agent compaction dropped %s:\n%s", want, out)
		}
	}
}

func TestFleetRegistryWrites(t *testing.T) {
	dir := setupFleet(t)
	if _, err := runCLI(t, "fleet", "add", "acme-two", "--fqdn", "two.3cx.us", "--client-id", "1", "--env-prefix", "THREECX_ACME", "--json"); exitCode(err) != 2 {
		t.Fatalf("a secret env already used by another PBX must be refused, got %v", err)
	}
	t.Setenv("PRINTING_PRESS_VERIFY", "1")
	out, err := runCLI(t, "fleet", "remove", "acme", "--json")
	if err != nil || !strings.Contains(out, "refused") {
		t.Fatalf("harness must refuse registry writes: %v %s", err, out)
	}
	reg, _ := fleet.Load()
	if _, ok := reg.Get("acme"); !ok {
		t.Fatal("harness remove deleted a registry entry")
	}
	_ = dir
	root := RootCmd()
	for _, path := range [][]string{{"fleet", "add"}, {"fleet", "remove"}} {
		c, _, err := root.Find(path)
		if err != nil || c.Annotations["mcp:hidden"] != "true" {
			t.Errorf("%v must be hidden from MCP", path)
		}
	}
	for _, path := range [][]string{{"fleet", "health"}, {"fleet", "trunks"}} {
		c, _, _ := root.Find(path)
		if c.Annotations["mcp:read-only"] == "true" {
			t.Errorf("%v writes history and must not claim read-only", path)
		}
	}
}

func TestPBXSelectionHardening(t *testing.T) {
	dir := setupFleet(t)
	t.Setenv("THREECX_ACME_CLIENT_SECRET", "s3cret")
	prev := pbxSelection
	t.Cleanup(func() { pbxSelection = prev })
	pbxSelection = "acme"
	if err := applyPBXSelection(&cobra.Command{Use: "x"}, &rootFlags{configPath: "/tmp/other.toml"}); exitCode(err) != 2 {
		t.Fatalf("--pbx with --config must be a usage error, got %v", err)
	}
	flags := &rootFlags{}
	if err := applyPBXSelection(&cobra.Command{Use: "x"}, flags); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("THREECX_CONFIG") != filepath.Join(flags.homePath, "config", "config.toml") {
		t.Fatalf("THREECX_CONFIG = %q; the legacy-config fallback must be disabled", os.Getenv("THREECX_CONFIG"))
	}
	firstHome := flags.homePath
	token := filepath.Join(firstHome, "data", "credentials.toml")
	_ = os.MkdirAll(filepath.Dir(token), 0o700)
	_ = os.WriteFile(token, []byte("access_token = \"old-host-token\"\n"), 0o600)

	// Re-point the PBX at another host: selection lands in a different home
	// and deletes nothing itself.
	reg, _ := fleet.Load()
	p, _ := reg.Get("acme")
	p.FQDN = "elsewhere.3cx.us"
	reg.Upsert(p)
	if err := reg.Save(); err != nil {
		t.Fatal(err)
	}
	t.Setenv(fleet.DirEnv, dir)
	flags = &rootFlags{}
	if err := applyPBXSelection(&cobra.Command{Use: "x"}, flags); err != nil {
		t.Fatal(err)
	}
	if flags.homePath == firstHome {
		t.Fatal("a PBX re-pointed to another host must not reuse the old home or its token")
	}
	if _, err := os.Stat(token); err != nil {
		t.Fatal("selection must not delete anything (old home is pruned only by fleet add/remove)")
	}

	// fleet add of the new identity prunes the old home under the registry lock.
	t.Setenv("PRINTING_PRESS_VERIFY", "")
	t.Setenv(fleet.SelectEnv, "")
	pbxSelection, pinnedPBX = "", ""
	if _, err := runCLI(t, "fleet", "add", "acme", "--fqdn", "elsewhere.3cx.us", "--client-id", "900", "--env-prefix", "THREECX_ACME", "--json"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(firstHome); !os.IsNotExist(err) {
		t.Fatal("fleet add must prune the previous identity's home")
	}
	if _, err := runCLI(t, "fleet", "remove", "acme", "--json"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "pbx", "acme")); !os.IsNotExist(err) {
		t.Fatal("fleet remove must delete every home for the name")
	}

	// --pbx stays authoritative over --tag.
	pinnedPBX = "globex"
	s := &fleetScope{tags: []string{"gold"}}
	got, full, err := s.resolve()
	if err != nil || len(got) != 1 || got[0].Name != "globex" || full {
		t.Fatalf("resolve with --pbx = %+v %v full=%v", got, err, full)
	}
}

func TestRawGetFollowsNextLinkOnSameHostOnly(t *testing.T) {
	testenv.Isolate(t)
	var srvURL string
	evil := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/connect/token":
			_, _ = w.Write([]byte(`{"access_token":"tok","expires_in":3600}`))
		case r.URL.Query().Get("$skip") == "1":
			_, _ = w.Write([]byte(`{"value":[{"Number":"101"}]}`))
		default:
			next := srvURL + "/xapi/v1/Users?$skip=1"
			if evil {
				next = "https://evil.example/xapi/v1/Users?$skip=1"
			}
			_, _ = w.Write([]byte(`{"value":[{"Number":"100"}],"@odata.nextLink":"` + next + `"}`))
		}
	}))
	defer srv.Close()
	srvURL = srv.URL
	prevPin := pinnedPBX
	t.Cleanup(func() { pinnedPBX = prevPin })
	pinnedPBX = "acme"
	t.Setenv("THREECX_BASE_URL", srv.URL+"/xapi/v1")
	t.Setenv("THREECX_TOKEN_URL", srv.URL+"/connect/token")
	t.Setenv("THREECX_CLIENT_ID", "900")
	t.Setenv("THREECX_CLIENT_SECRET", "s3")
	run := func() (string, error) {
		cmd := &cobra.Command{Use: "x"}
		cmd.SetContext(context.Background())
		var out bytes.Buffer
		cmd.SetOut(&out)
		err := runRawGets(cmd, &rootFlags{timeout: 10 * time.Second}, []string{"/Users"})
		return out.String(), err
	}
	out, err := run()
	if err != nil || !strings.Contains(out, `"100"`) || !strings.Contains(out, `"101"`) {
		t.Fatalf("pages not merged: %v %s", err, out)
	}
	evil = true
	if _, err := run(); err == nil || !strings.Contains(err.Error(), "outside this PBX") {
		t.Fatalf("a nextLink to another host must be refused, got %v", err)
	}
	for _, next := range []string{"/xapi/v1/%252e%252e/%252e%252e/connect/token", "/xapi/v1/ConferenceSettings/Pbx.GenerateApiKey()"} {
		if _, _, err := sameHostNextLink(nil, next); err == nil {
			t.Errorf("nextLink %q must be refused", next)
		}
	}
}

func TestHarnessDoesNotWriteHistory(t *testing.T) {
	setupFleet(t)
	t.Setenv("PRINTING_PRESS_VERIFY", "1")
	if _, err := runCLI(t, "fleet", "health", "--only", "acme", "--json"); err != nil {
		t.Fatal(err)
	}
	if h, _ := fleet.LoadHistory(); len(h) != 0 {
		t.Fatalf("verify harness wrote %d history runs", len(h))
	}
}

func TestRawGetRefusesRedirects(t *testing.T) {
	testenv.Isolate(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/connect/token" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"tok","expires_in":3600}`))
			return
		}
		if r.URL.Path == "/xapi/v1/Users" {
			http.Redirect(w, r, "/xapi/v1/ConferenceSettings/Pbx.GenerateApiKey()", http.StatusFound)
			return
		}
		t.Errorf("redirect was followed to %s", r.URL.Path)
		_, _ = w.Write([]byte(`{"value":"NEW-KEY"}`))
	}))
	defer srv.Close()
	prevPin := pinnedPBX
	t.Cleanup(func() { pinnedPBX = prevPin })
	pinnedPBX = "acme"
	t.Setenv("THREECX_BASE_URL", srv.URL+"/xapi/v1")
	t.Setenv("THREECX_TOKEN_URL", srv.URL+"/connect/token")
	t.Setenv("THREECX_CLIENT_ID", "900")
	t.Setenv("THREECX_CLIENT_SECRET", "s3")
	cmd := &cobra.Command{Use: "x"}
	cmd.SetContext(context.Background())
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := runRawGets(cmd, &rootFlags{timeout: 10 * time.Second}, []string{"/Users"}); err == nil || strings.Contains(out.String(), "NEW-KEY") {
		t.Fatalf("redirect must be refused: err=%v out=%s", err, out.String())
	}
}

func TestFleetExecKeepsScalarCollections(t *testing.T) {
	dir := setupFleet(t)
	_ = os.WriteFile(filepath.Join(dir, "acme_Trunks_10__DidNumbers.json"), []byte(`{"value":["+15550100","+15550101"]}`), 0o600)
	out, err := runCLI(t, "fleet", "exec", "/Trunks(10)/DidNumbers", "--only", "acme", "--json")
	if err != nil || !strings.Contains(out, "+15550101") {
		t.Fatalf("scalar collection over-redacted: %v %s", err, out)
	}
}

func TestLocalCommandSkipsSecretCheck(t *testing.T) {
	setupFleet(t)
	prev := pbxSelection
	t.Cleanup(func() { pbxSelection = prev })
	pbxSelection = "globex" // THREECX_GLOBEX_CLIENT_SECRET unset
	local := &cobra.Command{Use: "changes", Annotations: map[string]string{"pp:data-source": "local"}}
	if err := applyPBXSelection(local, &rootFlags{}); err != nil {
		t.Fatalf("a local-only command must not need the PBX secret: %v", err)
	}
}

func TestFleetUnknownTagIsUsageError(t *testing.T) {
	setupFleet(t) // no PBX carries a tag
	_, err := runCLI(t, "fleet", "health", "--tag", "gold", "--no-save")
	if exitCode(err) != 2 || !strings.Contains(err.Error(), `unknown tag "gold"`) {
		t.Fatalf("a tag no PBX carries must be a usage error, got %v", err)
	}
}

func TestRenderEmptyCSVPrintsHeader(t *testing.T) {
	cmd := &cobra.Command{}
	var out bytes.Buffer
	cmd.SetOut(&out)
	err := render(cmd, &rootFlags{csv: true}, []fleet.LicenceRow{}, []string{"pbx", "error"}, "none",
		func(fleet.LicenceRow) map[string]any { return nil })
	if err != nil {
		t.Fatal(err)
	}
	header := strings.TrimSpace(out.String())
	if strings.Contains(header, "(no rows)") || !strings.Contains(header, "pbx") || !strings.Contains(header, "license_days_left") {
		t.Fatalf("empty CSV must print the column header, got %q", header)
	}
}

// Budget 3 round 1: a fact one row reports as unknown survives --agent. Frequency
// compaction drops a field most rows leave out, and the row then read as healthy.
func TestFleetRowsKeepEverySparseFieldUnderAgent(t *testing.T) {
	var out bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&out)
	rows := []fleet.VersionRow{{PBX: "a", Version: "20"}, {PBX: "b", Version: "20"}, {PBX: "c", Version: "20", UnknownUpdates: 1, Unknown: []string{"update_list"}}}
	if err := render(cmd, &rootFlags{asJSON: true, compact: true}, rows, []string{"pbx", "version"}, "", func(fleet.VersionRow) map[string]any { return nil }); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"unknown_updates"`, `"unknown"`, "update_list"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("--agent lost %s:\n%s", want, out.String())
		}
	}
}
