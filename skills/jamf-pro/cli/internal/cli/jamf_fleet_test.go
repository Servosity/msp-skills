// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bufio"
	"bytes"
	"compress/flate"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"jamf-pro-pp-cli/internal/client"
	"jamf-pro-pp-cli/internal/cliutil"
	"jamf-pro-pp-cli/internal/cliutil/testenv"
	"jamf-pro-pp-cli/internal/config"
	"jamf-pro-pp-cli/internal/fleet"
	"jamf-pro-pp-cli/internal/jamfauth"
)

// setupFleet registers acme, globex, and an unreachable "down" instance and
// points the fleet runner at the fake child. fixtures maps "<instance><path>"
// to a response body.
func setupFleet(t *testing.T, fixtures map[string]string) string {
	t.Helper()
	testenv.Isolate(t)
	dir := t.TempDir()
	t.Setenv(fleet.DirEnv, dir)
	t.Setenv("FIXTURES", dir)
	for _, k := range []string{fleet.SelectEnv, jamfauth.IdentityEnv, "JAMF_PRO_SERVER", "JAMF_PRO_CLIENT_ID", "JAMF_PRO_CLIENT_SECRET", "JAMF_PRO_BASE_URL",
		"JAMF_PRO_TOKEN_URL", "JAMF_PRO_CONFIG", "JAMF_PRO_HOME", fleet.EntriesEnv} {
		t.Setenv(k, "")
		_ = os.Unsetenv(k)
	}
	prevPin, prevSel := pinnedInstance, instanceSelection
	t.Cleanup(func() {
		pinnedInstance, instanceSelection = prevPin, prevSel
		jamfauth.ResetForTest()
		fleet.ReleaseHeldForTest()
	})
	pinnedInstance, instanceSelection = "", ""
	jamfauth.ResetForTest()
	fleet.ReleaseHeldForTest()
	reg := &fleet.Registry{}
	for _, n := range []string{"acme", "globex", "down"} {
		reg.Upsert(fleet.Instance{Name: n, Server: n + ".jamfcloud.com", ClientID: "client-" + n})
	}
	if err := reg.Save(); err != nil {
		t.Fatal(err)
	}
	for key, body := range fixtures {
		name := strings.NewReplacer("/", "_", "(", "_", ")", "_", ".", "_", "&", "_", "%", "_").Replace(key) + ".json"
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// The test binary is the fake child (jamf_fleet_child_test.go), so the
	// fleet runs the same way on every platform, Windows included.
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	prev := fleetExe
	fleetExe = exe
	t.Cleanup(func() { fleetExe = prev })
	return dir
}

// freshProcess makes the next step behave like a new process: no
// established identity, no held instance locks, no inherited identity.
func freshProcess(t *testing.T) {
	t.Helper()
	jamfauth.ResetForTest()
	fleet.ReleaseHeldForTest()
	pinnedInstance = ""
	t.Setenv(jamfauth.IdentityEnv, "")
	_ = os.Unsetenv(jamfauth.IdentityEnv)
}

func runCLI(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := RootCmd()
	cmd.SetArgs(args)
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	err := cmd.Execute()
	return out.String(), err
}

func exitCode(err error) int {
	var ce *cliError
	if errors.As(err, &ce) {
		return ce.code
	}
	return -1
}

func decodeRows(t *testing.T, out string) []map[string]any {
	t.Helper()
	var rows []map[string]any
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		var env struct {
			Results []map[string]any `json:"results"`
		}
		if err2 := json.Unmarshal([]byte(out), &env); err2 != nil {
			t.Fatalf("output is not JSON rows: %v\n%s", err, out)
		}
		rows = env.Results
	}
	return rows
}

func rowsFor(rows []map[string]any, instance string) []map[string]any {
	var out []map[string]any
	for _, r := range rows {
		if r["instance"] == instance {
			out = append(out, r)
		}
	}
	return out
}

func TestFleetExecMergesAndRedacts(t *testing.T) {
	setupFleet(t, map[string]string{
		"acme/api/v1/scripts":   `{"totalCount":1,"results":[{"id":"1","name":"a.sh","clientSecret":"leak-a"}]}`,
		"globex/api/v1/scripts": `{"totalCount":1,"results":[{"id":"2","name":"b.sh"}]}`,
	})
	t.Setenv(fleet.EntriesEnv, "acme=s-acme")
	out, err := runCLI(t, "fleet", "exec", "/api/v1/scripts", "--json")
	if err != nil {
		t.Fatalf("partial failure must exit 0, got %v", err)
	}
	if strings.Contains(out, "leak-a") {
		t.Fatalf("credential field not redacted: %s", out)
	}
	rows := decodeRows(t, out)
	if len(rowsFor(rows, "acme")) != 1 || len(rowsFor(rows, "globex")) != 1 || rowsFor(rows, "down")[0]["error"] == nil {
		t.Fatalf("rows = %v", rows)
	}
}

func TestFleetExecRefusesUnsafePaths(t *testing.T) {
	setupFleet(t, nil)
	for _, bad := range []string{
		"/api/v4/computers-inventory/1/filevault", "/api/v2/local-admin-password/x/account/admin/password",
		"/api/v1/oauth/token", "/api/v1/auth", "/api/v4/computers-inventory/1/view-recovery-lock-password",
		"/api/%2e%2e/v1/scripts", "/api/v1/../v1/scripts", "/other/path", "/api/v1/scripts?x=1",
		"/api/v1/scripts/1/download", "/api/v1/device-enrollments/upload-token", "/api/v1/sc%zz",
		"/api/v2/local-admin-password/m1/account/admin/audit", "/api/v2/local-admin-password/m1/account/admin/g1/audit",
		"/api/v2/local-admin-password/m1/account/admin/%61udit",
		"/api/v2/local-admin-password/m1/account/admin/{server}", "/api/v2/local-admin-password/m1/account/admin/%7Bserver%7D",
	} {
		if _, err := runCLI(t, "fleet", "exec", bad, "--json"); exitCode(err) != 2 {
			t.Errorf("fleet exec %s must be refused as a usage error, got %v", bad, err)
		}
	}
	if p, err := cleanJamfPath("/JSSResource/computerapplications/application/Google%20Chrome.app"); err != nil || !strings.Contains(p, "%20") {
		t.Fatalf("an escaped Classic name must be accepted unchanged: %q %v", p, err)
	}
}

func TestFleetAllFailedIsNonZero(t *testing.T) {
	setupFleet(t, nil)
	if _, err := runCLI(t, "fleet", "exec", "/api/v1/jamf-pro-version", "--only", "down", "--json"); err == nil {
		t.Fatal("every instance failing must be a non-zero exit")
	}
}

func TestFleetFind(t *testing.T) {
	setupFleet(t, map[string]string{
		"acme/api/v4/computers-inventory":     `{"totalCount":1,"results":[{"id":"7","general":{"name":"Ada's MacBook","lastContact":"2026-10-14T10:00:00Z","managementId":"m-7","remoteManagement":{"managed":true}},"hardware":{"serialNumber":"C02X"},"userAndLocation":{"username":"ada"}}]}`,
		"acme/api/v2/mobile-devices/detail":   `{"totalCount":0,"results":[]}`,
		"globex/api/v4/computers-inventory":   `{"totalCount":0,"results":[]}`,
		"globex/api/v2/mobile-devices/detail": `{"totalCount":1,"results":[{"mobileDeviceId":"3","deviceType":"iOS","general":{"displayName":"Ada iPad","managed":true},"hardware":{"serialNumber":"DMP1"},"userAndLocation":{"username":"ada"}}]}`,
	})
	out, err := runCLI(t, "fleet", "find", "ada", "--json")
	if err != nil {
		t.Fatal(err)
	}
	rows := decodeRows(t, out)
	acme, globex := rowsFor(rows, "acme"), rowsFor(rows, "globex")
	if len(acme) != 1 || acme[0]["kind"] != "computer" || acme[0]["serial"] != "C02X" || !strings.Contains(fmt.Sprint(acme[0]["next"]), "--instance acme") {
		t.Fatalf("acme rows = %v", acme)
	}
	if len(globex) != 1 || globex[0]["kind"] != "mobile:iOS" {
		t.Fatalf("globex rows = %v", globex)
	}
	if _, err := runCLI(t, "fleet", "find", "ada", "--kind", "tablet"); exitCode(err) != 2 {
		t.Fatalf("bad --kind must be a usage error, got %v", err)
	}
}

func TestFleetPosture(t *testing.T) {
	stale := time.Now().UTC().Add(-40 * 24 * time.Hour).Format(time.RFC3339)
	fresh := time.Now().UTC().Add(-2 * time.Hour).Format(time.RFC3339)
	inv := `{"managedComputers":2,"unmanagedComputers":0,"managedDevices":0,"unmanagedDevices":0}`
	setupFleet(t, map[string]string{
		"acme/api/v1/inventory-information": inv,
		"acme/api/v4/computers-inventory": `{"totalCount":2,"results":[
			{"id":"1","general":{"name":"ok","lastContact":"` + fresh + `","remoteManagement":{"managed":true}},"operatingSystem":{"version":"15.1","fileVault2Status":"ALL_ENCRYPTED"},"security":{"firewallEnabled":true}},
			{"id":"2","general":{"name":"bad","lastContact":"` + stale + `","remoteManagement":{"managed":true}},"operatingSystem":{"version":"13.6","fileVault2Status":"NOT_ENCRYPTED"},"security":{"firewallEnabled":false}}]}`,
		"globex/api/v1/inventory-information": inv,
		"globex/api/v4/computers-inventory":   `{"totalCount":0,"results":[]}`,
	})
	out, err := runCLI(t, "fleet", "posture", "--min-os", "14.6", "--json")
	if err != nil {
		t.Fatal(err)
	}
	acme := rowsFor(decodeRows(t, out), "acme")
	if len(acme) != 1 || acme[0]["failing"] != float64(1) || acme[0]["stale_check_in"] != float64(1) || acme[0]["below_min_os"] != float64(1) || acme[0]["percent_compliant"] != float64(50) {
		t.Fatalf("acme posture = %v", acme)
	}
	out, err = runCLI(t, "fleet", "posture", "--failing", "--only", "acme", "--json")
	if err != nil {
		t.Fatal(err)
	}
	failing := decodeRows(t, out)
	if len(failing) != 1 || failing[0]["name"] != "bad" {
		t.Fatalf("--failing = %v", failing)
	}
}

func TestFleetPatch(t *testing.T) {
	setupFleet(t, map[string]string{
		"acme/api/v3/patch-software-title-configurations":                  `[{"id":"11","displayName":"Chrome"},{"id":"12","displayName":"Zoom"}]`,
		"acme/api/v2/patch-policies":                                       `{"totalCount":1,"results":[{"id":"1","softwareTitleConfigurationId":"11","failed":2}]}`,
		"acme/api/v1/managed-software-updates/update-statuses":             `{"totalCount":1,"results":[{"status":"INSTALL_FAILED"}]}`,
		"acme/api/v3/patch-software-title-configurations/11/patch-summary": `{"latestVersion":"130","upToDate":1,"outOfDate":3}`,
		"acme/api/v3/patch-software-title-configurations/12/patch-summary": `{"latestVersion":"6.2","upToDate":4,"outOfDate":0}`,
	})
	out, err := runCLI(t, "fleet", "patch", "--only", "acme", "--behind-only", "--json")
	if err != nil {
		t.Fatal(err)
	}
	rows := decodeRows(t, out)
	if len(rows) != 2 || rows[0]["title"] != "Chrome" || rows[0]["patch_policy_failures"] != float64(2) || rows[1]["kind"] != "os_updates" || rows[1]["failed_os_updates"] != float64(1) {
		t.Fatalf("patch rows = %v", rows)
	}
}

func TestFleetHealth(t *testing.T) {
	soon := time.Now().UTC().Add(10 * 24 * time.Hour).Format(time.RFC3339)
	far := time.Now().UTC().Add(300 * 24 * time.Hour).Format(time.RFC3339)
	setupFleet(t, map[string]string{
		"acme/api/v1/jamf-pro-version":                  `{"version":"11.10.1"}`,
		"acme/api/v1/device-enrollments":                `{"totalCount":1,"results":[{"id":"1","name":"ABM","tokenExpirationDate":"` + soon + `"}]}`,
		"acme/api/v1/volume-purchasing-locations":       `{"totalCount":1,"results":[{"id":"1","name":"VPP","tokenExpiration":"` + far + `","clientContextMismatch":true}]}`,
		"acme/api/v1/health-check":                      `null`,
		"acme/api/v1/device-enrollments/1/syncs/latest": `{"syncState":"CONNECTION_ERROR","timestamp":"2026-10-14T00:00:00Z"}`,
		"globex/api/v1/jamf-pro-version":                `{"version":"11.9.0"}`,
		"globex/api/v1/device-enrollments":              `{"totalCount":0,"results":[]}`,
		"globex/api/v1/volume-purchasing-locations":     `{"totalCount":1,"results":[{"id":"2","name":"VPP2","tokenExpiration":"` + far + `"}]}`,
		"globex/api/v1/health-check":                    `null`,
	})
	out, err := runCLI(t, "fleet", "health", "--json")
	if err != nil {
		t.Fatal(err)
	}
	rows := decodeRows(t, out)
	status := map[string]string{}
	for _, r := range rows {
		status[fmt.Sprint(r["instance"], "/", r["item"])] = fmt.Sprint(r["status"])
	}
	want := map[string]string{"acme/version": "ok", "globex/version": "warn", "acme/ade_token": "warn", "acme/ade_sync": "fail",
		"acme/vpp_token": "fail", "acme/health_check": "ok", "down/instance": "fail",
		// clientContextMismatch not reported: ownership unknown is a warning, never ok.
		"globex/vpp_token": "warn"}
	for k, v := range want {
		if status[k] != v {
			t.Errorf("%s = %q, want %q (all: %v)", k, status[k], v, status)
		}
	}
	if _, err := runCLI(t, "fleet", "health", "--only", "acme", "--fail-on", "warn", "--json"); exitCode(err) != exitFleetProblems {
		t.Fatalf("--fail-on warn must exit %d, got %v", exitFleetProblems, err)
	}
}

func TestFleetBaseline(t *testing.T) {
	scripts := func(body string) string {
		return `{"totalCount":1,"results":[{"id":"1","name":"setup.sh","scriptContents":` + strconv.Quote(body) + `}]}`
	}
	empty := `{"totalCount":0,"results":[]}`
	setupFleet(t, map[string]string{
		"acme/api/v1/scripts":                         scripts("echo v2"),
		"acme/api/v1/packages":                        `{"totalCount":1,"results":[{"id":"1","packageName":"Std.pkg"}]}`,
		"acme/api/v1/computer-extension-attributes":   empty,
		"acme/api/v3/computer-groups/smart-groups":    empty,
		"acme/JSSResource/policies/createdBy/jss":     `{"policies":[{"id":1,"name":"Std Policy"}]}`,
		"acme/JSSResource/osxconfigurationprofiles":   `{"os_x_configuration_profiles":[]}`,
		"globex/api/v1/scripts":                       scripts("echo v1"),
		"globex/api/v1/packages":                      empty,
		"globex/api/v1/computer-extension-attributes": empty,
		"globex/api/v3/computer-groups/smart-groups":  empty,
		"globex/JSSResource/policies/createdBy/jss":   `{"policies":[{"id":5,"name":"Std Policy"},{"id":6,"name":"Client Only"}]}`,
		"globex/JSSResource/osxconfigurationprofiles": `{"os_x_configuration_profiles":[]}`,
	})
	out, err := runCLI(t, "fleet", "baseline", "--golden", "acme", "--only", "globex", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, r := range decodeRows(t, out) {
		got = append(got, fmt.Sprint(r["kind"], ":", r["name"], ":", r["status"]))
	}
	want := "package:Std.pkg:missing policy:Client Only:extra script:setup.sh:differs"
	if strings.Join(got, " ") != want {
		t.Fatalf("baseline = %v, want %s", got, want)
	}
	if _, err := runCLI(t, "fleet", "baseline", "--golden", "nope"); exitCode(err) != 2 {
		t.Fatalf("unknown golden must be a usage error, got %v", err)
	}
}

func TestFleetPolicyHygiene(t *testing.T) {
	setupFleet(t, map[string]string{
		"acme/JSSResource/policies/createdBy/jss": `{"policies":[{"id":1,"name":"Broken"},{"id":2,"name":"Good"}]}`,
		"acme/api/v1/scripts":                     `{"totalCount":1,"results":[{"id":"5","name":"ok.sh"}]}`,
		"acme/api/v1/packages":                    `{"totalCount":0,"results":[]}`,
		"acme/JSSResource/policies/id/1":          `{"policy":{"general":{"id":1,"name":"Broken","enabled":false,"category":{"id":-1,"name":"No category assigned"}},"scope":{"all_computers":true},"scripts":[{"id":9,"name":"gone.sh"}]}}`,
		"acme/JSSResource/policies/id/2":          `{"policy":{"general":{"id":2,"name":"Good","enabled":true,"trigger_checkin":true,"category":{"id":3,"name":"Apps"}},"scope":{"all_computers":true},"scripts":[{"id":5,"name":"ok.sh"}]}}`,
	})
	out, err := runCLI(t, "fleet", "policy-hygiene", "--only", "acme", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var view struct {
		Rows            []map[string]any `json:"rows"`
		ScannedPolicies int              `json:"scanned_policies"`
	}
	if err := json.Unmarshal([]byte(out), &view); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if view.ScannedPolicies != 2 || len(view.Rows) != 1 || view.Rows[0]["name"] != "Broken" ||
		fmt.Sprint(view.Rows[0]["defects"]) != "[disabled uncategorized missing_script]" {
		t.Fatalf("hygiene = %+v", view)
	}
	out, err = runCLI(t, "fleet", "policy-hygiene", "--only", "acme", "--max-policies", "1", "--json")
	var capped struct {
		Skipped int    `json:"skipped_policies"`
		Note    string `json:"note"`
	}
	if err != nil || json.Unmarshal([]byte(out), &capped) != nil || capped.Skipped != 1 || capped.Note == "" {
		t.Fatalf("--max-policies must report skipped policies: %v %s", err, out)
	}
}

func TestFleetApp(t *testing.T) {
	setupFleet(t, map[string]string{
		"acme/JSSResource/computerapplications/application/Google%20Chrome.app": `{"computer_applications":{"versions":[{"number":"128.0.1","computers":[{"id":1,"name":"mac-1"}]},{"number":"131.0","computers":[{"id":2,"name":"mac-2"}]}]}}`,
	})
	out, err := runCLI(t, "fleet", "app", "Google Chrome", "--below", "130", "--only", "acme", "--json")
	if err != nil {
		t.Fatal(err)
	}
	rows := decodeRows(t, out)
	if len(rows) != 1 || rows[0]["version"] != "128.0.1" || rows[0]["app"] != "Google Chrome.app" || rows[0]["computer_names"] != "mac-1" {
		t.Fatalf("app rows = %v", rows)
	}
	if _, err := runCLI(t, "fleet", "app", "Firefox"); exitCode(err) != 2 {
		t.Fatalf("missing --below must be a usage error, got %v", err)
	}
}

func TestInstanceRegistryCommands(t *testing.T) {
	setupFleet(t, nil)
	if _, err := runCLI(t, "instance", "add", "initech", "--server", "initech.jamfcloud.com", "--client-id", "abc", "--json"); err != nil {
		t.Fatal(err)
	}
	reg, err := fleet.Load()
	if err != nil {
		t.Fatal(err)
	}
	p, ok := reg.Get("initech")
	if !ok || p.ClientID != "abc" {
		t.Fatalf("added = %+v", p)
	}
	if _, err := runCLI(t, "instance", "add", "evil", "--server", "https://x.example/api", "--client-id", "a"); exitCode(err) != 2 {
		t.Fatalf("a URL as server must be refused, got %v", err)
	}
	out, err := runCLI(t, "instance", "list", "--json")
	if err != nil || !strings.Contains(out, `"initech"`) || strings.Contains(out, "secret\":\"") {
		t.Fatalf("list = %v %s", err, out)
	}
	if _, err := runCLI(t, "instance", "remove", "initech", "--json"); err != nil {
		t.Fatal(err)
	}
	if _, err := runCLI(t, "instance", "remove", "initech"); exitCode(err) == 0 {
		t.Fatal("removing an unknown instance must fail")
	}
}

func TestApplyInstanceSelection(t *testing.T) {
	dir := setupFleet(t, nil)
	t.Setenv(fleet.EntriesEnv, "acme=s3cret,globex=other")
	t.Setenv("JAMF_PRO_BASE_URL", "http://evil.example")
	instanceSelection = "acme"
	flags := &rootFlags{}
	if err := applyInstanceSelection(&cobra.Command{Use: "x"}, flags); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("JAMF_PRO_SERVER") != "acme.jamfcloud.com" || os.Getenv("JAMF_PRO_CLIENT_SECRET") != "s3cret" || os.Getenv("JAMF_PRO_CLIENT_ID") != "client-acme" {
		t.Fatal("selection did not set the instance's server and credential")
	}
	if os.Getenv("JAMF_PRO_BASE_URL") != "https://acme.jamfcloud.com" {
		t.Fatalf("the endpoint must be pinned from the registry, got %q", os.Getenv("JAMF_PRO_BASE_URL"))
	}
	if os.Getenv(fleet.EntriesEnv) != "acme=s3cret" {
		t.Fatal("another instance's secret must be scrubbed from a pinned process")
	}
	acme := fleet.Instance{Name: "acme", Server: "acme.jamfcloud.com", ClientID: "client-acme"}
	if flags.homePath != acme.Home(dir) || os.Getenv(fleet.DirEnv) != dir {
		t.Fatalf("home = %q, fleet dir = %q", flags.homePath, os.Getenv(fleet.DirEnv))
	}
	freshProcess(t)
	instanceSelection = "globex" // its secret was scrubbed above
	if err := applyInstanceSelection(&cobra.Command{Use: "x"}, &rootFlags{}); exitCode(err) != 4 {
		t.Fatalf("missing secret must be an auth error, got %v", err)
	}
	if err := applyInstanceSelection(&cobra.Command{Use: "x"}, &rootFlags{dryRun: true}); err != nil {
		t.Fatalf("--dry-run must not need the secret: %v", err)
	}
	freshProcess(t)
	instanceSelection = "nope"
	if err := applyInstanceSelection(&cobra.Command{Use: "x"}, &rootFlags{}); exitCode(err) != 2 {
		t.Fatalf("unknown instance must be a usage error, got %v", err)
	}
	freshProcess(t)
	instanceSelection = "acme"
	if err := applyInstanceSelection(&cobra.Command{Use: "x"}, &rootFlags{homePath: "/tmp/x"}); exitCode(err) != 2 {
		t.Fatalf("--instance with --home must be a usage error, got %v", err)
	}
	authCmd := &cobra.Command{Use: "auth"}
	login := &cobra.Command{Use: "login"}
	authCmd.AddCommand(login)
	if err := applyInstanceSelection(login, &rootFlags{}); exitCode(err) != 2 {
		t.Fatalf("auth login under --instance must be refused, got %v", err)
	}
}

// mockJamf serves the OAuth token endpoint (form-body credentials only, as
// the vendor documents) and a 0-based paginated computer inventory.
func mockJamf(t *testing.T, total int, tokenForms *[]url.Values, pages *[]string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/oauth/token":
			body, _ := io.ReadAll(r.Body)
			form, _ := url.ParseQuery(string(body))
			*tokenForms = append(*tokenForms, form)
			if r.Header.Get("Authorization") != "" || form.Get("client_id") != "cid" || form.Get("client_secret") != "csecret" || form.Get("grant_type") != "client_credentials" {
				w.WriteHeader(400)
				_, _ = w.Write([]byte(`{"error":"invalid_client"}`))
				return
			}
			_, _ = w.Write([]byte(`{"access_token":"tok","expires_in":1200,"token_type":"Bearer"}`))
		case "/api/v4/computers-inventory":
			if r.Header.Get("Authorization") != "Bearer tok" {
				w.WriteHeader(401)
				_, _ = w.Write([]byte(`{"httpStatus":401,"errors":[]}`))
				return
			}
			page, _ := strconv.Atoi(r.URL.Query().Get("page"))
			size := 100
			if s := r.URL.Query().Get("page-size"); s != "" {
				size, _ = strconv.Atoi(s)
			}
			*pages = append(*pages, r.URL.Query().Get("page"))
			var items []string
			for i := page * size; i < total && i < (page+1)*size; i++ {
				items = append(items, fmt.Sprintf(`{"id":"%d","general":{"name":"mac-%d","personalRecoveryKey":"K-%d"}}`, i, i, i))
			}
			fmt.Fprintf(w, `{"totalCount":%d,"results":[%s]}`, total, strings.Join(items, ","))
		default:
			w.WriteHeader(404)
		}
	}))
}

// TestRawGetChild drives the real child primitive against a mock Jamf Pro:
// form-body token mint, every 0-based page read to totalCount, redaction.
func TestRawGetChild(t *testing.T) {
	freshProcess(t)
	testenv.Isolate(t)
	var forms []url.Values
	var pages []string
	srv := mockJamf(t, 250, &forms, &pages)
	defer srv.Close()
	prevPin := pinnedInstance
	t.Cleanup(func() { pinnedInstance = prevPin })
	pinnedInstance = "acme"
	t.Setenv("JAMF_PRO_BASE_URL", srv.URL)
	t.Setenv("JAMF_PRO_TOKEN_URL", srv.URL+"/api/v1/oauth/token")
	t.Setenv("JAMF_PRO_CLIENT_ID", "cid")
	t.Setenv("JAMF_PRO_CLIENT_SECRET", "csecret")
	cmd := &cobra.Command{Use: "x"}
	cmd.SetContext(context.Background())
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := runRawGets(cmd, &rootFlags{timeout: 10 * time.Second}, []string{"/api/v4/computers-inventory?section=GENERAL"}, false); err != nil {
		t.Fatal(err)
	}
	if len(forms) != 1 {
		t.Fatalf("expected one token mint, got %d", len(forms))
	}
	if strings.Join(pages, ",") != ",1,2" {
		t.Fatalf("pages requested = %q, want the default page then 1 and 2 (0-based)", strings.Join(pages, ","))
	}
	var bodies []struct {
		TotalCount int              `json:"totalCount"`
		Results    []map[string]any `json:"results"`
	}
	if err := json.Unmarshal(out.Bytes(), &bodies); err != nil {
		t.Fatal(err)
	}
	if bodies[0].TotalCount != 250 || len(bodies[0].Results) != 250 {
		t.Fatalf("merged %d of %d", len(bodies[0].Results), bodies[0].TotalCount)
	}
	seen := map[string]bool{}
	for _, r := range bodies[0].Results {
		seen[fmt.Sprint(r["id"])] = true
	}
	if len(seen) != 250 || !seen["100"] || !seen["199"] {
		t.Fatal("records 100-199 (Jamf page 1) must not be skipped")
	}
	if strings.Contains(out.String(), "K-1") {
		t.Fatal("personalRecoveryKey must be redacted")
	}
	pinnedInstance = ""
	if err := runRawGets(cmd, &rootFlags{}, []string{"/api/v1/scripts"}, false); exitCode(err) != 2 {
		t.Fatalf("--raw without an instance must be a usage error, got %v", err)
	}
}

func TestRawGetPinnedPageDoesNotWalk(t *testing.T) {
	freshProcess(t)
	testenv.Isolate(t)
	var forms []url.Values
	var pages []string
	srv := mockJamf(t, 250, &forms, &pages)
	defer srv.Close()
	prevPin := pinnedInstance
	t.Cleanup(func() { pinnedInstance = prevPin })
	pinnedInstance = "acme"
	t.Setenv("JAMF_PRO_BASE_URL", srv.URL)
	t.Setenv("JAMF_PRO_TOKEN_URL", srv.URL+"/api/v1/oauth/token")
	t.Setenv("JAMF_PRO_CLIENT_ID", "cid")
	t.Setenv("JAMF_PRO_CLIENT_SECRET", "csecret")
	cmd := &cobra.Command{Use: "x"}
	cmd.SetContext(context.Background())
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := runRawGets(cmd, &rootFlags{timeout: 10 * time.Second}, []string{"/api/v4/computers-inventory?page=2&page-size=100"}, false); err != nil {
		t.Fatal(err)
	}
	if strings.Join(pages, ",") != "2" || !strings.Contains(out.String(), `"mac-249"`) || strings.Contains(out.String(), `"mac-0"`) {
		t.Fatalf("an explicit page must be read alone: pages=%v", pages)
	}
}

func TestStoreScopeStableAcrossFirstMint(t *testing.T) {
	freshProcess(t)
	dir := t.TempDir()
	t.Setenv("JAMF_PRO_CLIENT_ID", "cid")
	t.Setenv("JAMF_PRO_CLIENT_SECRET", "s")
	envOnly := &config.Config{BaseURL: "https://Acme.jamfcloud.com/"}
	afterMint := &config.Config{BaseURL: "https://acme.jamfcloud.com", ClientID: "cid", ClientSecret: "s", AccessToken: "tok"}
	a, b := envOnly.StoreScopeCredential(), afterMint.StoreScopeCredential()
	if a == "" || a != b {
		t.Fatalf("store scope must not change across the first token mint: %q %q", a, b)
	}
	tmplA := &config.Config{BaseURL: "https://{server}", TemplateVars: map[string]string{"server": "acme.jamfcloud.com"}}
	tmplB := &config.Config{BaseURL: "https://{server}", TemplateVars: map[string]string{"server": "globex.jamfcloud.com"}}
	if tmplA.StoreScopeCredential() != a || tmplA.StoreScopeCredential() == tmplB.StoreScopeCredential() {
		t.Fatal("two servers behind the same {server} template must get different stores, and the resolved one must match")
	}
	// A pair saved by 'auth login' (bound) keys the same store as the env pair.
	t.Setenv("JAMF_PRO_CLIENT_ID", "")
	t.Setenv("JAMF_PRO_CLIENT_SECRET", "")
	saved := &config.Config{Path: filepath.Join(dir, "config.toml"), BaseURL: "https://acme.jamfcloud.com", ClientID: "cid", ClientSecret: "s"}
	bindSavedCredentials(saved, "", "", "cid", "s")
	if saved.StoreScopeCredential() != a {
		t.Fatal("a saved pair must key the same store as the same env pair")
	}
}

// TestRawGetCollectionChangedIsAnError: a totalCount that changes mid-walk
// must fail rather than return a short or stale list.
func TestRawGetCollectionChangedIsAnError(t *testing.T) {
	freshProcess(t)
	testenv.Isolate(t)
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/oauth/token" {
			_, _ = w.Write([]byte(`{"access_token":"tok","expires_in":1200}`))
			return
		}
		calls++
		var items []string
		for i := 0; i < 100; i++ {
			items = append(items, fmt.Sprintf(`{"id":"%d"}`, i))
		}
		total := 150
		if calls > 1 {
			total = 250 // the collection grew between pages
		}
		fmt.Fprintf(w, `{"totalCount":%d,"results":[%s]}`, total, strings.Join(items, ","))
	}))
	defer srv.Close()
	prevPin := pinnedInstance
	t.Cleanup(func() { pinnedInstance = prevPin })
	pinnedInstance = "acme"
	t.Setenv("JAMF_PRO_BASE_URL", srv.URL)
	t.Setenv("JAMF_PRO_TOKEN_URL", srv.URL+"/api/v1/oauth/token")
	t.Setenv("JAMF_PRO_CLIENT_ID", "cid")
	t.Setenv("JAMF_PRO_CLIENT_SECRET", "csecret")
	cmd := &cobra.Command{Use: "x"}
	cmd.SetContext(context.Background())
	cmd.SetOut(io.Discard)
	err := runRawGets(cmd, &rootFlags{timeout: 10 * time.Second}, []string{"/api/v1/scripts"}, false)
	if err == nil || !strings.Contains(err.Error(), "changed") {
		t.Fatalf("a changed collection must be an error, got %v", err)
	}
}

// TestTokenMintDoesNotPersistEnvSecret: an env-sourced client secret is used
// to mint a token but never written under the CLI's home.
func TestTokenMintDoesNotPersistEnvSecret(t *testing.T) {
	home := testenv.Isolate(t)
	var forms []url.Values
	var pages []string
	srv := mockJamf(t, 5, &forms, &pages)
	defer srv.Close()
	t.Setenv("JAMF_PRO_HOME", home)
	t.Setenv("JAMF_PRO_BASE_URL", srv.URL)
	t.Setenv("JAMF_PRO_TOKEN_URL", srv.URL+"/api/v1/oauth/token")
	t.Setenv("JAMF_PRO_CLIENT_ID", "cid")
	t.Setenv("JAMF_PRO_CLIENT_SECRET", "csecret")
	if _, err := runCLI(t, "computer-inventory", "list", "--json", "--data-source", "live"); err != nil {
		t.Fatal(err)
	}
	if len(forms) == 0 {
		t.Fatal("expected a token mint")
	}
	_ = filepath.Walk(home, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			if b, rerr := os.ReadFile(path); rerr == nil && strings.Contains(string(b), "csecret") {
				t.Errorf("client secret written to disk: %s", path)
			}
		}
		return nil
	})
}

// TestRawGetRepeatedRecordIsAnError: a record that appears on two pages means
// the order shifted and others were skipped.
func TestRawGetRepeatedRecordIsAnError(t *testing.T) {
	freshProcess(t)
	testenv.Isolate(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/oauth/token" {
			_, _ = w.Write([]byte(`{"access_token":"tok","expires_in":1200}`))
			return
		}
		var items []string
		for i := 1; i <= 100; i++ {
			items = append(items, fmt.Sprintf(`{"id":"%d"}`, i)) // every page repeats ids 1-100
		}
		fmt.Fprintf(w, `{"totalCount":200,"results":[%s]}`, strings.Join(items, ","))
	}))
	defer srv.Close()
	prevPin := pinnedInstance
	t.Cleanup(func() { pinnedInstance = prevPin })
	pinnedInstance = "acme"
	t.Setenv("JAMF_PRO_BASE_URL", srv.URL)
	t.Setenv("JAMF_PRO_TOKEN_URL", srv.URL+"/api/v1/oauth/token")
	t.Setenv("JAMF_PRO_CLIENT_ID", "cid")
	t.Setenv("JAMF_PRO_CLIENT_SECRET", "csecret")
	cmd := &cobra.Command{Use: "x"}
	cmd.SetContext(context.Background())
	cmd.SetOut(io.Discard)
	if err := runRawGets(cmd, &rootFlags{timeout: 10 * time.Second}, []string{"/api/v1/scripts"}, false); err == nil || !strings.Contains(err.Error(), "twice") {
		t.Fatalf("a repeated record must be an error, got %v", err)
	}
}

// TestRawGetFreshTokenMintsAgain: --fresh-token ignores a cached token.
func TestRawGetFreshTokenMintsAgain(t *testing.T) {
	freshProcess(t)
	home := testenv.Isolate(t)
	var forms []url.Values
	var pages []string
	srv := mockJamf(t, 1, &forms, &pages)
	defer srv.Close()
	prevPin := pinnedInstance
	t.Cleanup(func() { pinnedInstance = prevPin })
	pinnedInstance = "acme"
	t.Setenv("JAMF_PRO_HOME", home)
	t.Setenv("JAMF_PRO_BASE_URL", srv.URL)
	t.Setenv("JAMF_PRO_TOKEN_URL", srv.URL+"/api/v1/oauth/token")
	t.Setenv("JAMF_PRO_CLIENT_ID", "cid")
	t.Setenv("JAMF_PRO_CLIENT_SECRET", "csecret")
	cmd := &cobra.Command{Use: "x"}
	cmd.SetContext(context.Background())
	cmd.SetOut(io.Discard)
	for i := 0; i < 2; i++ {
		if err := runRawGets(cmd, &rootFlags{timeout: 10 * time.Second}, []string{"/api/v4/computers-inventory"}, false); err != nil {
			t.Fatal(err)
		}
	}
	if len(forms) != 1 {
		t.Fatalf("the second run must reuse the cached token, got %d mints", len(forms))
	}
	if err := runRawGets(cmd, &rootFlags{timeout: 10 * time.Second}, []string{"/api/v4/computers-inventory"}, true); err != nil {
		t.Fatal(err)
	}
	if len(forms) != 2 {
		t.Fatalf("--fresh-token must mint again, got %d mints", len(forms))
	}
}

// TestAuthLoginDoesNotPersistEnvSecret: auth login with env credentials
// keeps the secret out of the credentials file.
func TestAuthLoginDoesNotPersistEnvSecret(t *testing.T) {
	home := testenv.Isolate(t)
	var forms []url.Values
	var pages []string
	srv := mockJamf(t, 1, &forms, &pages)
	defer srv.Close()
	t.Setenv("JAMF_PRO_HOME", home)
	t.Setenv("JAMF_PRO_BASE_URL", srv.URL)
	t.Setenv("JAMF_PRO_TOKEN_URL", srv.URL+"/api/v1/oauth/token")
	t.Setenv("JAMF_PRO_CLIENT_ID", "cid")
	t.Setenv("JAMF_PRO_CLIENT_SECRET", "csecret")
	if _, err := runCLI(t, "auth", "login"); err != nil {
		t.Fatal(err)
	}
	if len(forms) != 1 || forms[0].Get("client_secret") != "csecret" {
		t.Fatalf("auth login must mint with form-body credentials: %v", forms)
	}
	_ = filepath.Walk(home, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			if b, rerr := os.ReadFile(path); rerr == nil && strings.Contains(string(b), "csecret") {
				t.Errorf("env-sourced client secret written to disk: %s", path)
			}
		}
		return nil
	})
}

// TestBaselineAllTargetsFailedIsNonZero: a readable golden instance must not
// hide that every compared instance failed.
func TestBaselineAllTargetsFailedIsNonZero(t *testing.T) {
	empty := `{"totalCount":0,"results":[]}`
	setupFleet(t, map[string]string{
		"acme/api/v1/scripts":                       empty,
		"acme/api/v1/packages":                      empty,
		"acme/api/v1/computer-extension-attributes": empty,
		"acme/api/v3/computer-groups/smart-groups":  empty,
		"acme/JSSResource/policies/createdBy/jss":   `{"policies":[]}`,
		"acme/JSSResource/osxconfigurationprofiles": `{"os_x_configuration_profiles":[]}`,
	})
	if _, err := runCLI(t, "fleet", "baseline", "--golden", "acme", "--only", "down", "--json"); err == nil {
		t.Fatal("every compared instance failing must be a non-zero exit")
	}
}

// TestPartialEnvPairNeverBorrowsCachedSecret: with a client id in the
// environment but no secret, the cached secret of another client is not used.
func TestPartialEnvPairNeverBorrowsCachedSecret(t *testing.T) {
	home := testenv.Isolate(t)
	var forms []url.Values
	var pages []string
	srv := mockJamf(t, 1, &forms, &pages)
	defer srv.Close()
	t.Setenv("JAMF_PRO_HOME", home)
	t.Setenv("JAMF_PRO_BASE_URL", srv.URL)
	t.Setenv("JAMF_PRO_TOKEN_URL", srv.URL+"/api/v1/oauth/token")
	if _, err := runCLI(t, "auth", "login", "--client-id", "cid", "--client-secret", "csecret"); err != nil {
		t.Fatal(err)
	}
	if _, err := runCLI(t, "auth", "logout"); err != nil {
		t.Fatal(err)
	}
	if _, err := runCLI(t, "auth", "login", "--client-id", "cid", "--client-secret", "csecret"); err != nil {
		t.Fatal(err)
	}
	forms = nil
	t.Setenv("JAMF_PRO_CLIENT_ID", "other-client")
	_, _ = runCLI(t, "computer-inventory", "list", "--json", "--data-source", "live", "--no-cache")
	for _, f := range forms {
		if f.Get("client_id") == "other-client" && f.Get("client_secret") == "csecret" {
			t.Fatal("a partial env pair borrowed the cached secret of another client")
		}
	}
}

func TestNoRepeatsUsesCollectionIdentity(t *testing.T) {
	seen := map[string]bool{}
	page1 := []json.RawMessage{json.RawMessage(`{"mobileDeviceId":"7","general":{}}`), json.RawMessage(`{"osUpdatesStatusId":"u1"}`)}
	if err := noRepeats("/x", page1, seen); err != nil {
		t.Fatal(err)
	}
	if err := noRepeats("/x", []json.RawMessage{json.RawMessage(`{"mobileDeviceId":"7","general":{}}`)}, seen); err == nil {
		t.Fatal("a repeated mobileDeviceId must be detected")
	}
	if err := noRepeats("/x", []json.RawMessage{json.RawMessage(`{"osUpdatesStatusId":"u1"}`)}, seen); err == nil {
		t.Fatal("a repeated osUpdatesStatusId must be detected")
	}
	anon := map[string]bool{}
	_ = noRepeats("/x", []json.RawMessage{json.RawMessage(`{"name":"a"}`)}, anon)
	if err := noRepeats("/x", []json.RawMessage{json.RawMessage(`{"name":"a"}`)}, anon); err == nil {
		t.Fatal("an identical record without an id must be detected by content")
	}
}

func TestFleetFindDetailRefusesTruncatedMatch(t *testing.T) {
	setupFleet(t, map[string]string{
		"acme/api/v4/computers-inventory":   `{"totalCount":2,"results":[{"id":"7","general":{"name":"mac-a"},"hardware":{"serialNumber":"S1"}}]}`,
		"acme/api/v2/mobile-devices/detail": `{"totalCount":0,"results":[]}`,
	})
	out, err := runCLI(t, "fleet", "find", "mac", "--limit", "1", "--detail", "--only", "acme", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, `"match"`) || !strings.Contains(out, `"truncated": true`) {
		t.Fatalf("a truncated single row must not be expanded as the only match: %s", out)
	}
}

// TestAuthLoginNeverPersistsMixedPair: a login whose id comes from a flag but
// whose secret comes from the environment clears the cached pair.
func TestAuthLoginNeverPersistsMixedPair(t *testing.T) {
	home := testenv.Isolate(t)
	var forms []url.Values
	var pages []string
	srv := mockJamf(t, 1, &forms, &pages)
	defer srv.Close()
	t.Setenv("JAMF_PRO_HOME", home)
	t.Setenv("JAMF_PRO_BASE_URL", srv.URL)
	t.Setenv("JAMF_PRO_TOKEN_URL", srv.URL+"/api/v1/oauth/token")
	if _, err := runCLI(t, "auth", "login", "--client-id", "cid", "--client-secret", "csecret"); err != nil {
		t.Fatal(err)
	}
	t.Setenv("JAMF_PRO_CLIENT_SECRET", "csecret")
	if _, err := runCLI(t, "auth", "login", "--client-id", "cid"); err != nil {
		t.Fatal(err)
	}
	_ = filepath.Walk(home, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			if b, rerr := os.ReadFile(path); rerr == nil && strings.Contains(string(b), "csecret") {
				t.Errorf("a login with an env-sourced secret left a stored secret: %s", path)
			}
		}
		return nil
	})
}

// identityServer mints one fixed token and records the Authorization header
// of every API request.
func identityServer(t *testing.T, token string, mints *[]string, auths *[]string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = r.ParseForm()
		// Any request that carries a client secret counts as a mint attempt,
		// whatever its path: Basic auth or a client_secret form field.
		if id, sec, ok := r.BasicAuth(); ok || r.PostForm.Get("client_secret") != "" {
			if !ok {
				id, sec = r.PostForm.Get("client_id"), r.PostForm.Get("client_secret")
			}
			*mints = append(*mints, id+":"+sec)
			if r.URL.Path != "/api/v1/oauth/token" {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			fmt.Fprintf(w, `{"access_token":%q,"expires_in":1200}`, token)
			return
		}
		*auths = append(*auths, r.Header.Get("Authorization"))
		_, _ = w.Write([]byte(`{"totalCount":0,"results":[]}`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// writeTokenURLConfig writes a 0600 config.toml naming base and tokenURL and
// returns its path.
func writeTokenURLConfig(t *testing.T, base, tokenURL string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.toml")
	body := ""
	if base != "" {
		body += fmt.Sprintf("base_url = %q\n", base)
	}
	body += fmt.Sprintf("token_url = %q\n", tokenURL)
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func pointAt(t *testing.T, srv *httptest.Server, id, secret string) {
	t.Setenv("JAMF_PRO_BASE_URL", srv.URL)
	t.Setenv("JAMF_PRO_TOKEN_URL", srv.URL+"/api/v1/oauth/token")
	t.Setenv("JAMF_PRO_CLIENT_ID", id)
	t.Setenv("JAMF_PRO_CLIENT_SECRET", secret)
}

// TestCachedTokenNotSentToAnotherIdentity: a token cached for server A and
// client a is never sent to server B or used for another client; the same
// identity still reuses it.
func TestCachedTokenNotSentToAnotherIdentity(t *testing.T) {
	home := testenv.Isolate(t)
	t.Setenv("JAMF_PRO_HOME", home)
	var mintsA, mintsB, authsA, authsB []string
	a := identityServer(t, "tok-A", &mintsA, &authsA)
	b := identityServer(t, "tok-B", &mintsB, &authsB)
	list := []string{"computer-inventory", "list", "--json", "--data-source", "live", "--no-cache"}

	pointAt(t, a, "client-a", "secret-a")
	if _, err := runCLI(t, list...); err != nil {
		t.Fatal(err)
	}
	if len(mintsA) != 1 || len(authsA) == 0 || authsA[0] != "Bearer tok-A" {
		t.Fatalf("server A: mints=%v auths=%v", mintsA, authsA)
	}
	// Control: the same identity reuses its cached token.
	if _, err := runCLI(t, list...); err != nil {
		t.Fatal(err)
	}
	if len(mintsA) != 1 {
		t.Fatalf("the same identity must reuse its token, got %d mints", len(mintsA))
	}

	pointAt(t, b, "client-b", "secret-b")
	if _, err := runCLI(t, list...); err != nil {
		t.Fatal(err)
	}
	for _, h := range authsB {
		if h != "Bearer tok-B" {
			t.Fatalf("server B received %q; only its own token may be sent", h)
		}
	}
	if len(mintsB) != 1 || len(authsB) == 0 {
		t.Fatalf("server B: mints=%v auths=%v", mintsB, authsB)
	}

	// Same server, another API client: mint again instead of borrowing.
	pointAt(t, b, "client-b2", "secret-b2")
	if _, err := runCLI(t, list...); err != nil {
		t.Fatal(err)
	}
	if len(mintsB) != 2 {
		t.Fatalf("a different client id must mint its own token, got %d mints", len(mintsB))
	}
}

// TestSetTokenIsBoundToItsServer: a pasted token works where it was saved
// and is not sent to another server.
func TestSetTokenIsBoundToItsServer(t *testing.T) {
	home := testenv.Isolate(t)
	t.Setenv("JAMF_PRO_HOME", home)
	var mintsA, mintsB, authsA, authsB []string
	a := identityServer(t, "unused", &mintsA, &authsA)
	b := identityServer(t, "unused", &mintsB, &authsB)
	pointAt(t, a, "", "")
	cmd := RootCmd()
	cmd.SetArgs([]string{"auth", "set-token"})
	cmd.SetIn(strings.NewReader("pasted-token\n"))
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	list := []string{"computer-inventory", "list", "--json", "--data-source", "live", "--no-cache"}
	if _, err := runCLI(t, list...); err != nil {
		t.Fatal(err)
	}
	if len(authsA) == 0 || authsA[0] != "Bearer pasted-token" {
		t.Fatalf("server A auths = %v", authsA)
	}
	pointAt(t, b, "", "")
	_, _ = runCLI(t, list...)
	for _, h := range authsB {
		if strings.Contains(h, "pasted-token") {
			t.Fatal("a token saved for server A was sent to server B")
		}
	}
}

// TestPinInstanceFromEnv: the MCP entry point pins the process to
// JAMF_PRO_INSTANCE, and a client built for any other server is refused.
func TestPinInstanceFromEnv(t *testing.T) {
	dir := setupFleet(t, nil)
	t.Cleanup(func() { _, _ = cliutil.SetHomeOverride("") })
	t.Setenv(fleet.EntriesEnv, "acme=s3cret,globex=other")
	t.Setenv(fleet.SelectEnv, "acme")
	t.Setenv(mcpHTTPCallerEnv, "caller-token")

	cfg := &config.Config{BaseURL: "https://evil.example"}
	if err := ApplyClientHooks(client.New(cfg, time.Second, 0)); err == nil {
		t.Fatal("an unpinned process with JAMF_PRO_INSTANCE set must refuse to build a client")
	}
	if err := EstablishMCPIdentity(); err != nil {
		t.Fatal(err)
	}
	if os.Getenv(mcpHTTPCallerEnv) != "caller-token" {
		t.Fatal("pinning must keep the MCP HTTP caller token")
	}
	if pinnedInstance != "acme" || os.Getenv("JAMF_PRO_BASE_URL") != "https://acme.jamfcloud.com" || os.Getenv(fleet.EntriesEnv) != "acme=s3cret" {
		t.Fatalf("pin = %q base = %q", pinnedInstance, os.Getenv("JAMF_PRO_BASE_URL"))
	}
	acme := fleet.Instance{Name: "acme", Server: "acme.jamfcloud.com", ClientID: "client-acme"}
	if d, err := cliutil.DataDir(); err != nil || !strings.HasPrefix(d, acme.Home(dir)) {
		t.Fatalf("data dir %q is not under the instance home %q (%v)", d, acme.Home(dir), err)
	}
	loaded, err := config.Load("")
	if err != nil {
		t.Fatal(err)
	}
	if err := ApplyClientHooks(client.New(loaded, time.Second, 0)); err != nil {
		t.Fatalf("the pinned instance's own client must build: %v", err)
	}
	if err := ApplyClientHooks(client.New(cfg, time.Second, 0)); err == nil {
		t.Fatal("a client for another server must be refused in a pinned process")
	}
	t.Setenv(fleet.SelectEnv, "nope")
	if err := EstablishMCPIdentity(); err == nil {
		t.Fatal("an unknown instance must fail")
	}
}

// TestFleetFindRejectsContradictions: an empty page that reports matches is
// an instance error, and --detail refuses while any instance errored.
func TestFleetFindRejectsContradictions(t *testing.T) {
	setupFleet(t, map[string]string{
		"acme/api/v4/computers-inventory":     `{"totalCount":3,"results":[]}`,
		"acme/api/v2/mobile-devices/detail":   `{"totalCount":0,"results":[]}`,
		"globex/api/v4/computers-inventory":   `{"totalCount":0,"results":[]}`,
		"globex/api/v2/mobile-devices/detail": `{"totalCount":1,"results":[{"mobileDeviceId":"3","deviceType":"iOS","general":{"displayName":"Ada iPad","managed":true},"hardware":{"serialNumber":"DMP1"},"userAndLocation":{"username":"ada"}}]}`,
	})
	out, err := runCLI(t, "fleet", "find", "ada", "--json", "--only", "acme,globex")
	if err != nil {
		t.Fatal(err)
	}
	acme := rowsFor(decodeRows(t, out), "acme")
	if len(acme) != 1 || !strings.Contains(fmt.Sprint(acme[0]["error"]), "totalCount") {
		t.Fatalf("acme rows = %v", acme)
	}
	out, err = runCLI(t, "fleet", "find", "ada", "--json", "--only", "acme,globex", "--detail")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, `"match"`) {
		t.Fatalf("--detail must not expand a match while an instance errored: %s", out)
	}
}

// TestNoRepeatsUsesUUID: an MDM command seen twice with a changed state is
// still the same record.
func TestNoRepeatsUsesUUID(t *testing.T) {
	seen := map[string]bool{}
	if err := noRepeats("/api/v2/mdm/commands", []json.RawMessage{json.RawMessage(`{"uuid":"u-1","commandState":"PENDING"}`)}, seen); err != nil {
		t.Fatal(err)
	}
	if err := noRepeats("/api/v2/mdm/commands", []json.RawMessage{json.RawMessage(`{"uuid":"u-1","commandState":"ACKNOWLEDGED"}`)}, seen); err == nil {
		t.Fatal("a command repeated across pages must be detected by uuid")
	}
}

// TestLoginPairNotSentToAnotherServer: a client pair saved by 'auth login'
// for server A is never posted to server B's token endpoint, and neither is
// A's token sent to B.
func TestLoginPairNotSentToAnotherServer(t *testing.T) {
	home := testenv.Isolate(t)
	t.Setenv("JAMF_PRO_HOME", home)
	var mintsA, mintsB, authsA, authsB []string
	a := identityServer(t, "tok-A", &mintsA, &authsA)
	b := identityServer(t, "tok-B", &mintsB, &authsB)
	pointAt(t, a, "", "")
	if _, err := runCLI(t, "auth", "login", "--client-id", "client-a", "--client-secret", "secret-a"); err != nil {
		t.Fatal(err)
	}
	list := []string{"computer-inventory", "list", "--json", "--data-source", "live", "--no-cache"}
	if _, err := runCLI(t, list...); err != nil {
		t.Fatal(err)
	}
	if len(authsA) == 0 || authsA[0] != "Bearer tok-A" {
		t.Fatalf("server A auths = %v (the login token must be used)", authsA)
	}
	pointAt(t, b, "", "")
	_, _ = runCLI(t, list...)
	if len(mintsB) != 0 {
		t.Fatalf("server B received a mint with %v; A's saved pair must not leave A", mintsB)
	}
	for _, h := range authsB {
		if strings.Contains(h, "tok-A") {
			t.Fatal("A's token was sent to B")
		}
	}
	// Control: back on A the saved pair still mints.
	freshProcess(t)
	pointAt(t, a, "", "")
	cfg, err := config.Load("")
	if err != nil {
		t.Fatal(err)
	}
	cfg.AccessToken = ""
	c := client.New(cfg, time.Second, 0)
	if err := ApplyClientHooks(c); err != nil {
		t.Fatal(err)
	}
	if c.Config.ClientID != "client-a" {
		t.Fatalf("the pair saved for A must stay usable on A, got %q", c.Config.ClientID)
	}
}

// TestConfigCredentialHeadersDropped: an Authorization header in the
// config's [headers] table never overrides the bound token.
func TestConfigCredentialHeadersDropped(t *testing.T) {
	home := testenv.Isolate(t)
	t.Setenv("JAMF_PRO_HOME", home)
	var mints, auths []string
	a := identityServer(t, "tok-A", &mints, &auths)
	pointAt(t, a, "client-a", "secret-a")
	cfgPath := filepath.Join(home, "explicit.toml")
	if err := os.WriteFile(cfgPath, []byte("[headers]\nauthorization = \"Bearer leaked\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("JAMF_PRO_CONFIG", cfgPath)
	if _, err := runCLI(t, "computer-inventory", "list", "--json", "--data-source", "live", "--no-cache"); err != nil {
		t.Fatal(err)
	}
	if len(auths) == 0 || auths[0] != "Bearer tok-A" {
		t.Fatalf("auths = %v; a configured Authorization header must be dropped", auths)
	}
}

// TestBindingIsForTheExactToken: a binding written for one token never
// validates another token in the same config.
func TestBindingIsForTheExactToken(t *testing.T) {
	freshProcess(t)
	dir := t.TempDir()
	cfg := &config.Config{Path: filepath.Join(dir, "config.toml"), BaseURL: "https://a.example", AccessToken: "tok-Y"}
	bindSavedCredentials(cfg, "tok-X", "", "", "")
	if _, err := identityFor(cfg, cfg.BaseURL); err != nil || cfg.AccessToken != "" {
		t.Fatalf("a binding for tok-X must not validate tok-Y (err %v)", err)
	}
	cfg.AccessToken = "tok-X"
	if _, err := identityFor(cfg, cfg.BaseURL); err != nil || cfg.AccessToken != "tok-X" {
		t.Fatal("the bound token itself must stay usable")
	}
}

// TestGenericAllWalkRejectsOverlap: the generated --all walker fails a Jamf
// walk with a repeated record or fewer records than totalCount.
func TestGenericAllWalkRejectsOverlap(t *testing.T) {
	for name, tc := range map[string]struct {
		pages [][]int
		ok    bool
	}{
		"overlap": {[][]int{{1, 2}, {2, 3}, {}}, false},
		"short":   {[][]int{{1, 2}, {}}, false},
		"clean":   {[][]int{{1, 2}, {3, 4}}, true},
	} {
		t.Run(name, func(t *testing.T) {
			home := testenv.Isolate(t)
			t.Setenv("JAMF_PRO_HOME", home)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path == "/api/v1/oauth/token" {
					_, _ = w.Write([]byte(`{"access_token":"tok","expires_in":1200}`))
					return
				}
				n, _ := strconv.Atoi(r.URL.Query().Get("page"))
				var items []string
				if n < len(tc.pages) {
					for _, id := range tc.pages[n] {
						items = append(items, fmt.Sprintf(`{"id":"%d"}`, id))
					}
				}
				fmt.Fprintf(w, `{"totalCount":4,"results":[%s]}`, strings.Join(items, ","))
			}))
			defer srv.Close()
			pointAt(t, srv, "cid", "csecret")
			out, err := runCLI(t, "computer-inventory", "list", "--all", "--json", "--data-source", "live", "--no-cache")
			if tc.ok && err != nil {
				t.Fatalf("a clean walk must pass: %v", err)
			}
			if !tc.ok && err == nil {
				t.Fatalf("a bad walk must fail, got %s", out)
			}
		})
	}
}

// TestFleetTablesScrubTerminalControls: a device name carrying escape
// sequences cannot rewrite a human or plain report.
func TestFleetTablesScrubTerminalControls(t *testing.T) {
	setupFleet(t, map[string]string{
		"acme/api/v4/computers-inventory": `{"totalCount":1,"results":[{"id":"7","general":{"name":"Ada\u001b[2J\u001b[31mFAKE"},"hardware":{"serialNumber":"C02X"},"userAndLocation":{"username":"ada"}}]}`,
	})
	for _, mode := range []string{"--plain", "--csv"} {
		out, err := runCLI(t, "fleet", "find", "ada", "--kind", "computer", "--only", "acme", mode)
		if err != nil {
			t.Fatal(err)
		}
		if strings.ContainsRune(out, 0x1b) || !strings.Contains(out, "FAKE") {
			t.Fatalf("%s output kept terminal controls: %q", mode, out)
		}
	}
}

// TestStoreScopeTokenOnlyIsPerServer: a token-only login never shares a
// local mirror across servers.
func TestStoreScopeTokenOnlyIsPerServer(t *testing.T) {
	freshProcess(t)
	dir := t.TempDir()
	t.Setenv("JAMF_PRO_CLIENT_ID", "")
	t.Setenv("JAMF_PRO_CLIENT_SECRET", "")
	path := filepath.Join(dir, "config.toml")
	a := &config.Config{Path: path, BaseURL: "https://a.example", AccessToken: "same-token"}
	bindSavedCredentials(a, "same-token", "", "", "") // auth set-token on server A
	b := &config.Config{Path: path, BaseURL: "https://b.example", AccessToken: "same-token"}
	ka, kb := a.StoreScopeCredential(), b.StoreScopeCredential()
	if ka == "" || ka == kb {
		t.Fatalf("one token on two servers must not share a store key: %q %q", ka, kb)
	}
}

// TestTokenErrorsMaskTheSecret: a token endpoint that echoes the client
// secret in its error body never gets that secret into the CLI's error, on
// the client mint path or the auth login path.
func TestTokenErrorsMaskTheSecret(t *testing.T) {
	home := testenv.Isolate(t)
	t.Setenv("JAMF_PRO_HOME", home)
	const secret = "s3cr3t+/value&x"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		w.WriteHeader(http.StatusUnauthorized)
		// Echo the secret in every form a server might, plus a marker: the
		// body must never be printed, only the status and the short code.
		fmt.Fprintf(w, `{"error":"invalid_client","echo":%q,"b64":%q,"raw64":%q,"url64":%q,"note":"BODYMARK"}`, string(raw),
			base64.StdEncoding.EncodeToString([]byte(secret)), base64.RawStdEncoding.EncodeToString([]byte(secret)), base64.RawURLEncoding.EncodeToString([]byte("cid:"+secret)))
	}))
	defer srv.Close()
	pointAt(t, srv, "cid", secret)
	for _, args := range [][]string{
		{"computer-inventory", "list", "--json", "--data-source", "live", "--no-cache"},
		{"auth", "login"},
	} {
		out, err := runCLI(t, args...)
		if err == nil {
			t.Fatalf("%v: a 401 mint must fail", args)
		}
		all := out + err.Error()
		if strings.Contains(all, "BODYMARK") || !strings.Contains(all, "invalid_client") || !strings.Contains(all, "401") {
			t.Fatalf("%v must print only the status and the short code: %s", args, all)
		}
		for _, form := range []string{secret, url.QueryEscape(secret), base64.RawStdEncoding.EncodeToString([]byte(secret)), base64.RawURLEncoding.EncodeToString([]byte("cid:" + secret))} {
			if strings.Contains(all, form) {
				t.Fatalf("%v leaked the secret: %s", args, all)
			}
		}
	}
}

// TestSyncShortWalkFails: a walk that reads fewer records than totalCount
// fails the sync instead of reporting success.
func TestSyncShortWalkFails(t *testing.T) {
	home := testenv.Isolate(t)
	t.Setenv("JAMF_PRO_HOME", home)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/oauth/token" {
			_, _ = w.Write([]byte(`{"access_token":"tok","expires_in":1200}`))
			return
		}
		if r.URL.Query().Get("page") == "0" {
			_, _ = w.Write([]byte(`{"totalCount":2,"results":[{"id":"1","general":{"name":"a"}}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"totalCount":2,"results":[]}`))
	}))
	defer srv.Close()
	pointAt(t, srv, "cid", "csecret")
	if _, err := runCLI(t, "sync", "--resources", "computer_inventory", "--json"); err == nil {
		t.Fatal("a short walk must fail the sync")
	}
}

// TestClientRefusesRetargeting: once a client's credentials were checked for
// one server, filling {server} with another (as --path-context or an MCP
// tool input does) is refused before anything is sent.
func TestClientRefusesRetargeting(t *testing.T) {
	freshProcess(t)
	home := testenv.Isolate(t)
	t.Setenv("JAMF_PRO_HOME", home)
	var mintsA, mintsB, authsA, authsB []string
	a := identityServer(t, "tok-A", &mintsA, &authsA)
	b := identityServer(t, "tok-B", &mintsB, &authsB)
	t.Setenv("JAMF_PRO_CLIENT_ID", "client-a")
	t.Setenv("JAMF_PRO_CLIENT_SECRET", "secret-a")
	cfg, err := config.Load("")
	if err != nil {
		t.Fatal(err)
	}
	cfg.BaseURL, cfg.TokenURL = "http://{server}", "http://{server}/api/v1/oauth/token"
	cfg.TemplateVars = map[string]string{"server": strings.TrimPrefix(a.URL, "http://")}
	c := client.New(cfg, 5*time.Second, 0)
	c.NoCache = true
	if err := ApplyClientHooks(c); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Get(context.Background(), "/api/v1/scripts", nil); err != nil {
		t.Fatalf("the checked server must work: %v", err)
	}
	cfg.TemplateVars["server"] = strings.TrimPrefix(b.URL, "http://")
	if _, err := c.Get(context.Background(), "/api/v1/scripts", nil); err == nil {
		t.Fatal("a retargeted request must be refused")
	}
	if len(mintsB) != 0 || len(authsB) != 0 {
		t.Fatalf("server B was contacted: mints=%v auths=%v", mintsB, authsB)
	}
	if _, err := runCLI(t, "sync", "--resources", "computer_inventory", "--path-context", "server=evil.example"); exitCode(err) != 2 {
		t.Fatalf("sync --path-context server must be a usage error, got %v", err)
	}
}

// TestSyncStuckAndResumedWalks: a stuck page fails the sync; a walk capped
// by --max-pages and its resume do not.
func TestSyncStuckAndResumedWalks(t *testing.T) {
	serve := func(stuck bool) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if r.URL.Path == "/api/v1/oauth/token" {
				_, _ = w.Write([]byte(`{"access_token":"tok","expires_in":1200}`))
				return
			}
			page, _ := strconv.Atoi(r.URL.Query().Get("page"))
			size, _ := strconv.Atoi(r.URL.Query().Get("page-size"))
			if size <= 0 {
				size = 100
			}
			if stuck {
				page = 0
			}
			var items []string
			for i := page*size + 1; i <= (page+1)*size && i <= 250; i++ {
				items = append(items, fmt.Sprintf(`{"id":"%d","general":{"name":"m%d"}}`, i, i))
			}
			fmt.Fprintf(w, `{"totalCount":250,"results":[%s]}`, strings.Join(items, ","))
		}))
	}
	t.Run("stuck", func(t *testing.T) {
		home := testenv.Isolate(t)
		t.Setenv("JAMF_PRO_HOME", home)
		srv := serve(true)
		defer srv.Close()
		pointAt(t, srv, "cid", "csecret")
		if _, err := runCLI(t, "sync", "--resources", "computer_inventory", "--json"); err == nil {
			t.Fatal("a stuck page must fail the sync")
		}
	})
	t.Run("capped-then-resumed", func(t *testing.T) {
		home := testenv.Isolate(t)
		t.Setenv("JAMF_PRO_HOME", home)
		srv := serve(false)
		defer srv.Close()
		pointAt(t, srv, "cid", "csecret")
		if _, err := runCLI(t, "sync", "--resources", "computer_inventory", "--json", "--max-pages", "1"); err != nil {
			t.Fatalf("a capped walk is partial by request, not an error: %v", err)
		}
		if _, err := runCLI(t, "sync", "--resources", "computer_inventory", "--json"); err != nil {
			t.Fatalf("resuming a capped walk must not fail: %v", err)
		}
	})
}

// TestSyncOvershootFails: more unique records than totalCount is not the
// collection either.
func TestSyncOvershootFails(t *testing.T) {
	home := testenv.Isolate(t)
	t.Setenv("JAMF_PRO_HOME", home)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/oauth/token" {
			_, _ = w.Write([]byte(`{"access_token":"tok","expires_in":1200}`))
			return
		}
		_, _ = w.Write([]byte(`{"totalCount":1,"results":[{"id":"1","general":{"name":"a"}},{"id":"2","general":{"name":"b"}}]}`))
	}))
	defer srv.Close()
	pointAt(t, srv, "cid", "csecret")
	if _, err := runCLI(t, "sync", "--resources", "computer_inventory", "--json"); err == nil {
		t.Fatal("two records under totalCount 1 must fail the sync")
	}
}

// TestChildRefusesChangedRegistryEntry: a fleet child acts as the identity
// its parent handed down; if the registry entry changed or was removed since,
// it refuses before reading any secret, and it never re-targets.
func TestChildRefusesChangedRegistryEntry(t *testing.T) {
	setupFleet(t, nil)
	t.Setenv(fleet.EntriesEnv, "acme=s3cret")
	handed, err := jamfauth.ForInstance("acme", "acme.jamfcloud.com", "client-acme")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(jamfauth.IdentityEnv, handed.Encode())
	if err := applyInstanceSelection(&cobra.Command{Use: "x"}, &rootFlags{}); err != nil {
		t.Fatalf("the unchanged entry must be accepted: %v", err)
	}
	if est, _ := jamfauth.Established(); est != handed {
		t.Fatalf("the child must act as the handed-down identity, got %+v", est)
	}
	jamfauth.ResetForTest()
	fleet.ReleaseHeldForTest()
	moved, _ := jamfauth.ForInstance("acme", "evil.example.com", "client-acme")
	t.Setenv(jamfauth.IdentityEnv, moved.Encode())
	if err := applyInstanceSelection(&cobra.Command{Use: "x"}, &rootFlags{}); err == nil {
		t.Fatal("an identity the registry no longer holds must be refused")
	}
	// The registry is re-pointed after the parent handed acme down: the child
	// keeps the handed identity and refuses, it never follows the new entry.
	freshProcess(t)
	reg, _ := fleet.Load()
	reg.Upsert(fleet.Instance{Name: "acme", Server: "evil.example.com", ClientID: "client-acme"})
	if err := reg.Save(); err != nil {
		t.Fatal(err)
	}
	t.Setenv(jamfauth.IdentityEnv, handed.Encode())
	if err := applyInstanceSelection(&cobra.Command{Use: "x"}, &rootFlags{}); err == nil {
		t.Fatal("a child must refuse when the registry entry it was handed has been re-pointed")
	}
	if os.Getenv("JAMF_PRO_SERVER") == "evil.example.com" {
		t.Fatal("the child followed the re-pointed registry entry")
	}
	freshProcess(t)
	t.Setenv(jamfauth.IdentityEnv, handed.Encode())
	instanceSelection = "globex"
	if err := applyInstanceSelection(&cobra.Command{Use: "x"}, &rootFlags{}); err == nil {
		t.Fatal("a child started for acme must refuse --instance globex")
	}
}

// TestEstablishedIdentityIsAuthoritative: once a process acts as an
// identity, a config or environment that now names another client or
// server is refused for clients and keys an isolated store.
func TestEstablishedIdentityIsAuthoritative(t *testing.T) {
	freshProcess(t)
	home := testenv.Isolate(t)
	t.Setenv("JAMF_PRO_HOME", home)
	est, err := jamfauth.New("https://a.example", "c1")
	if err != nil {
		t.Fatal(err)
	}
	if err := jamfauth.Establish(est); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { freshProcess(t) })
	t.Setenv("JAMF_PRO_CLIENT_ID", "c2")
	t.Setenv("JAMF_PRO_CLIENT_SECRET", "s2")
	cfg := &config.Config{BaseURL: "https://a.example"}
	if err := ApplyClientHooks(client.New(cfg, time.Second, 0)); err == nil {
		t.Fatal("a client for another client id must be refused in a process acting as c1")
	}
	if k := cfg.StoreScopeCredential(); !strings.HasPrefix(k, "refused") {
		t.Fatalf("the store for another identity must be the refused key, got %q", k)
	}
	t.Setenv("JAMF_PRO_CLIENT_ID", "c1")
	t.Setenv("JAMF_PRO_CLIENT_SECRET", "s1")
	other := &config.Config{BaseURL: "https://b.example"}
	if err := ApplyClientHooks(client.New(other, time.Second, 0)); err == nil {
		t.Fatal("a client for another server must be refused")
	}
	same := &config.Config{BaseURL: "https://a.example"}
	if err := ApplyClientHooks(client.New(same, time.Second, 0)); err != nil {
		t.Fatalf("the established identity itself must build: %v", err)
	}
}

// TestTokenURLVariantsRefused: a token URL that is not exactly the
// identity's (trailing slash, case, query, other path) never receives the
// client secret, and the failure masks it.
func TestTokenURLVariantsRefused(t *testing.T) {
	for _, suffix := range []string{"/api/v1/oauth/token/", "/api/v1/OAuth/token", "/api/v1/oauth/token?x=1", "/api/v2/oauth/token", "/oauth/token"} {
		t.Run(suffix, func(t *testing.T) {
			freshProcess(t)
			home := testenv.Isolate(t)
			t.Setenv("JAMF_PRO_HOME", home)
			var mints, auths []string
			srv := identityServer(t, "tok", &mints, &auths)
			pointAt(t, srv, "cid", "s3cr3t-value")
			// The token URL is a config file's: a JAMF_PRO_BASE_URL override
			// always derives the server's own endpoint.
			t.Setenv("JAMF_PRO_BASE_URL", "")
			t.Setenv("JAMF_PRO_CONFIG", writeTokenURLConfig(t, srv.URL, srv.URL+suffix))
			out, err := runCLI(t, "computer-inventory", "list", "--json", "--data-source", "live", "--no-cache")
			if err == nil {
				t.Fatal("a mint to a variant token URL must fail")
			}
			if len(mints) != 0 {
				t.Fatalf("the variant token URL received the credentials: %v", mints)
			}
			if strings.Contains(out+err.Error(), "s3cr3t-value") {
				t.Fatalf("the error leaked the secret: %v", err)
			}
		})
	}
}

// TestRemoveRefusedWhileInstanceInUse: removing an instance a command is
// acting as is refused, and nothing is removed.
func TestRemoveRefusedWhileInstanceInUse(t *testing.T) {
	setupFleet(t, nil)
	t.Setenv(fleet.EntriesEnv, "acme=s3cret")
	instanceSelection = "acme"
	if err := applyInstanceSelection(&cobra.Command{Use: "x"}, &rootFlags{}); err != nil {
		t.Fatal(err)
	}
	if _, err := fleet.RemoveInstance("acme"); err == nil {
		t.Fatal("removing an instance in use must be refused")
	}
	if reg, _ := fleet.Load(); func() bool { _, ok := reg.Get("acme"); return !ok }() {
		t.Fatal("a refused removal must leave the registry entry")
	}
	fleet.ReleaseHeldForTest()
	if found, err := fleet.RemoveInstance("acme"); err != nil || !found {
		t.Fatalf("once no command holds it, removal must work: %v %v", found, err)
	}
}

// TestFleetPostureFlagsUnevaluatedManagedComputers: managed computers that
// inventory never returned are an error for that instance, not an all-clear.
func TestFleetPostureFlagsUnevaluatedManagedComputers(t *testing.T) {
	setupFleet(t, map[string]string{
		"acme/api/v1/inventory-information": `{"managedComputers":3,"unmanagedComputers":0,"managedDevices":0,"unmanagedDevices":0}`,
		"acme/api/v4/computers-inventory":   `{"totalCount":0,"results":[]}`,
	})
	for _, extra := range [][]string{nil, {"--failing"}} {
		out, _ := runCLI(t, append([]string{"fleet", "posture", "--json", "--only", "acme"}, extra...)...)
		if !strings.Contains(out, "incomplete: evaluated 0 of 3 managed computers") {
			t.Fatalf("posture %v must report incomplete coverage: %s", extra, out)
		}
	}
}

// TestFleetPosturePartialCoverage: one healthy Mac out of three managed is
// not 100% compliance: coverage is flagged and the percentage withheld.
func TestFleetPosturePartialCoverage(t *testing.T) {
	fresh := time.Now().UTC().Add(-2 * time.Hour).Format(time.RFC3339)
	setupFleet(t, map[string]string{
		"acme/api/v1/inventory-information": `{"managedComputers":3,"unmanagedComputers":0,"managedDevices":0,"unmanagedDevices":0}`,
		"acme/api/v4/computers-inventory": `{"totalCount":1,"results":[{"id":"1","general":{"name":"m1","lastContact":"` + fresh + `","remoteManagement":{"managed":true}},` +
			`"hardware":{"serialNumber":"S1"},"operatingSystem":{"version":"15.1","fileVault2Status":"ALL_ENCRYPTED"},"security":{"firewallEnabled":true}}]}`,
	})
	out, _ := runCLI(t, "fleet", "posture", "--json", "--only", "acme")
	rows := decodeRows(t, out)
	if len(rows) != 1 || rows[0]["percent_compliant"] != nil || !strings.Contains(fmt.Sprint(rows[0]["coverage"]), "evaluated 1 of 3") {
		t.Fatalf("partial coverage must withhold the percentage and flag coverage: %s", out)
	}
	out, _ = runCLI(t, "fleet", "posture", "--json", "--only", "acme", "--failing")
	if !strings.Contains(out, "evaluated 1 of 3") {
		t.Fatalf("--failing must carry the coverage gap: %s", out)
	}
}

// TestTokenRedirectNotFollowed: a token endpoint that answers with a
// redirect is not followed (the target is never fetched) and nothing it or
// the target wrote is printed.
func TestTokenRedirectNotFollowed(t *testing.T) {
	const secret = "s3cr3t-redirect"
	for _, args := range [][]string{
		{"computer-inventory", "list", "--json", "--data-source", "live", "--no-cache"},
		{"auth", "login"},
	} {
		freshProcess(t)
		home := testenv.Isolate(t)
		t.Setenv("JAMF_PRO_HOME", home)
		var targetHits int
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/api/v1/oauth/token" {
				http.Redirect(w, r, "/error", http.StatusFound)
				return
			}
			targetHits++
			raw, _ := io.ReadAll(r.Body)
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprintf(w, `{"error":"invalid_client","echo":%q,"note":"BODYMARK %s"}`, string(raw), secret)
		}))
		pointAt(t, srv, "cid", secret)
		out, err := runCLI(t, args...)
		srv.Close()
		if err == nil {
			t.Fatalf("%v: a redirected mint must fail", args)
		}
		all := out + err.Error()
		if targetHits != 0 || strings.Contains(all, secret) || strings.Contains(all, "BODYMARK") || !strings.Contains(all, "302") {
			t.Fatalf("%v: redirect followed (%d hits) or body printed: %s", args, targetHits, all)
		}
	}
}

// TestTokenErrorPrintsOnlyStandardCodes: a token endpoint can put anything in
// "error", a secret included; only the six RFC 6749 codes are printed.
func TestTokenErrorPrintsOnlyStandardCodes(t *testing.T) {
	const secret = "abcdefghijkl"
	for _, args := range [][]string{
		{"computer-inventory", "list", "--json", "--data-source", "live", "--no-cache"},
		{"auth", "login"},
	} {
		freshProcess(t)
		home := testenv.Isolate(t)
		t.Setenv("JAMF_PRO_HOME", home)
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprintf(w, `{"error":%q}`, secret)
		}))
		pointAt(t, srv, "cid", secret)
		out, err := runCLI(t, args...)
		srv.Close()
		if err == nil {
			t.Fatalf("%v: a 401 mint must fail", args)
		}
		if all := out + err.Error(); strings.Contains(all, secret) || !strings.Contains(all, "401") {
			t.Fatalf("%v: printed a non-standard error code: %s", args, all)
		}
	}
}

// TestIdentityAndStoreFailClosed: an unreadable configuration stops the MCP
// server (no start without an identity) and never resolves the shared
// unscoped store.
func TestIdentityAndStoreFailClosed(t *testing.T) {
	freshProcess(t)
	home := testenv.Isolate(t)
	t.Setenv("JAMF_PRO_HOME", home)
	bad := filepath.Join(home, "broken.toml")
	if err := os.WriteFile(bad, []byte("this is = = not toml [[["), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("JAMF_PRO_CONFIG", bad)
	t.Setenv("JAMF_PRO_CLIENT_ID", "cid")
	t.Setenv("JAMF_PRO_CLIENT_SECRET", "s")
	if err := EstablishMCPIdentity(); err == nil {
		t.Fatal("the MCP server must not start when no identity can be established")
	}
	if _, ok := jamfauth.Established(); ok {
		t.Fatal("no identity may be established from an unreadable configuration")
	}
	path, err := MCPStorePath()
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(path) == "data.db" {
		t.Fatalf("an unreadable configuration must not resolve the shared store, got %s", path)
	}
}

// rawJamf answers every request with handler(path) written verbatim, so a
// test can send a malformed status line.
func rawJamf(t *testing.T, handler func(path string) string) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(conn net.Conn) {
				defer conn.Close()
				r := bufio.NewReader(conn)
				req, err := http.ReadRequest(r)
				if err != nil {
					return
				}
				_, _ = io.Copy(io.Discard, req.Body)
				_, _ = conn.Write([]byte(handler(req.URL.Path)))
			}(conn)
		}
	}()
	return "http://" + ln.Addr().String()
}

// TestTransportErrorsAreFixedText: a server that puts the client secret (or
// the bearer token) into a malformed status line never gets it printed:
// credential requests fail with "token request failed", others by kind.
func TestTransportErrorsAreFixedText(t *testing.T) {
	const secret = "DemoSecret42"
	base := rawJamf(t, func(path string) string {
		if path == "/api/v1/oauth/token" {
			return "HTTP/1.1 " + secret + " Bad\r\nContent-Length: 0\r\n\r\n"
		}
		return "HTTP/1.1 200 OK\r\n\r\n"
	})
	for _, args := range [][]string{
		{"computer-inventory", "list", "--json", "--data-source", "live", "--no-cache"},
		{"auth", "login"},
	} {
		freshProcess(t)
		home := testenv.Isolate(t)
		t.Setenv("JAMF_PRO_HOME", home)
		t.Setenv("JAMF_PRO_BASE_URL", base)
		t.Setenv("JAMF_PRO_TOKEN_URL", base+"/api/v1/oauth/token")
		t.Setenv("JAMF_PRO_CLIENT_ID", "cid")
		t.Setenv("JAMF_PRO_CLIENT_SECRET", secret)
		out, err := runCLI(t, args...)
		if err == nil {
			t.Fatalf("%v: a malformed token response must fail", args)
		}
		if all := out + err.Error(); strings.Contains(all, secret) || !strings.Contains(all, "token request failed") {
			t.Fatalf("%v: transport error text leaked or was not fixed: %s", args, all)
		}
	}
	// An API call whose response status line echoes the bearer token.
	const bearer = "BearerEcho77token"
	api := rawJamf(t, func(path string) string {
		if path == "/api/v1/oauth/token" {
			body := `{"access_token":"` + bearer + `","expires_in":1200}`
			return fmt.Sprintf("HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: %d\r\n\r\n%s", len(body), body)
		}
		// The literal token and an encoded copy the generated masker does not know.
		return "HTTP/1.1 " + bearer + "-" + base64.StdEncoding.EncodeToString([]byte(bearer)) + " Bad\r\nContent-Length: 0\r\n\r\n"
	})
	freshProcess(t)
	home := testenv.Isolate(t)
	t.Setenv("JAMF_PRO_HOME", home)
	t.Setenv("JAMF_PRO_BASE_URL", api)
	t.Setenv("JAMF_PRO_TOKEN_URL", api+"/api/v1/oauth/token")
	t.Setenv("JAMF_PRO_CLIENT_ID", "cid")
	t.Setenv("JAMF_PRO_CLIENT_SECRET", "s")
	out, err := runCLI(t, "computer-inventory", "list", "--json", "--data-source", "live", "--no-cache")
	if err == nil {
		t.Fatal("a malformed API response must fail")
	}
	if all := out + err.Error(); strings.Contains(all, bearer) || strings.Contains(all, base64.StdEncoding.EncodeToString([]byte(bearer))) {
		t.Fatalf("an API transport error leaked the bearer token: %s", all)
	}
}

// TestRequestErrorKeepsCause: the fixed-text errors still unwrap to the
// original, so timeout, DNS and retry handling keep working.
func TestRequestErrorKeepsCause(t *testing.T) {
	dns := &net.DNSError{Err: "no such host", Name: "x.invalid", IsNotFound: true}
	err := apiRequestError(dns)
	var got *net.DNSError
	if !errors.As(err, &got) || err.Error() != "could not resolve the server host" {
		t.Fatalf("apiRequestError = %q, unwraps to DNS error: %v", err.Error(), errors.As(err, &got))
	}
	if cerr := credentialRequestError(0, context.DeadlineExceeded); !errors.Is(cerr, context.DeadlineExceeded) || cerr.Error() != "token request failed" {
		t.Fatalf("credentialRequestError = %q", cerr.Error())
	}
}

// TestFleetPatchTitleAttribution: a patch policy without a title id is an
// error for that instance, and failures of a policy whose title is not
// listed are reported, not dropped.
func TestFleetPatchTitleAttribution(t *testing.T) {
	common := map[string]string{
		"acme/api/v3/patch-software-title-configurations":                  `[{"id":"11","displayName":"Chrome"}]`,
		"acme/api/v1/managed-software-updates/update-statuses":             `{"totalCount":0,"results":[]}`,
		"acme/api/v3/patch-software-title-configurations/11/patch-summary": `{"latestVersion":"130","upToDate":5,"outOfDate":0}`,
	}
	with := func(policies string) map[string]string {
		m := map[string]string{"acme/api/v2/patch-policies": policies}
		for k, v := range common {
			m[k] = v
		}
		return m
	}
	setupFleet(t, with(`{"totalCount":1,"results":[{"id":"1","failed":2}]}`))
	out, _ := runCLI(t, "fleet", "patch", "--only", "acme", "--json")
	if !strings.Contains(out, "softwareTitleConfigurationId") {
		t.Fatalf("a policy without a title id must be an instance error: %s", out)
	}
	setupFleet(t, with(`{"totalCount":1,"results":[{"id":"1","softwareTitleConfigurationId":"99","failed":3}]}`))
	out, err := runCLI(t, "fleet", "patch", "--only", "acme", "--behind-only", "--json")
	if err != nil {
		t.Fatal(err)
	}
	rows := decodeRows(t, out)
	found := false
	for _, r := range rows {
		if r["kind"] == "patch_policy_unmatched" && r["patch_policy_failures"] == float64(3) {
			found = true
		}
	}
	if !found {
		t.Fatalf("failures of an unlisted title must be reported: %v", rows)
	}
}

// TestAPIRedirectRefused: no request follows a redirect, a malformed
// Location (here carrying the encoded bearer token) is never parsed or
// printed, and the message says to check the server URL.
func TestAPIRedirectRefused(t *testing.T) {
	const bearer = "BearerRedirect55"
	for _, loc := range []string{"http://[::1" + base64.StdEncoding.EncodeToString([]byte(bearer)), "/elsewhere"} {
		freshProcess(t)
		home := testenv.Isolate(t)
		t.Setenv("JAMF_PRO_HOME", home)
		hits := 0
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/api/v1/oauth/token":
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprintf(w, `{"access_token":%q,"expires_in":1200}`, bearer)
			case "/elsewhere":
				hits++
			default:
				w.Header().Set("Location", loc)
				w.WriteHeader(http.StatusFound)
			}
		}))
		pointAt(t, srv, "cid", "csecret")
		out, err := runCLI(t, "computer-inventory", "list", "--json", "--data-source", "live", "--no-cache")
		srv.Close()
		if err == nil {
			t.Fatalf("%s: a redirect must fail the request", loc)
		}
		all := out + err.Error()
		if hits != 0 || strings.Contains(all, bearer) || strings.Contains(all, base64.StdEncoding.EncodeToString([]byte(bearer))) ||
			!strings.Contains(all, "redirects are refused") || !strings.Contains(all, "check the server URL") {
			t.Fatalf("%s: redirect followed (%d) or message wrong: %s", loc, hits, all)
		}
	}
}

// TestBodyReadErrorsAreFixedText: a chunked API body whose malformed trailer
// carries the bearer token (literal and encoded) fails with fixed text; the
// read error never quotes what the server sent.
func TestBodyReadErrorsAreFixedText(t *testing.T) {
	const bearer = "BearerTrailer99"
	enc := base64.StdEncoding.EncodeToString([]byte(bearer))
	api := rawJamf(t, func(path string) string {
		if path == "/api/v1/oauth/token" {
			body := `{"access_token":"` + bearer + `","expires_in":1200}`
			return fmt.Sprintf("HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: %d\r\n\r\n%s", len(body), body)
		}
		return "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nTransfer-Encoding: chunked\r\n\r\n" +
			"5\r\n{\"a\":\r\n0\r\n" + bearer + "-" + enc + "\r\n\r\n"
	})
	freshProcess(t)
	home := testenv.Isolate(t)
	t.Setenv("JAMF_PRO_HOME", home)
	t.Setenv("JAMF_PRO_BASE_URL", api)
	t.Setenv("JAMF_PRO_TOKEN_URL", api+"/api/v1/oauth/token")
	t.Setenv("JAMF_PRO_CLIENT_ID", "cid")
	t.Setenv("JAMF_PRO_CLIENT_SECRET", "s")
	out, err := runCLI(t, "computer-inventory", "list", "--json", "--data-source", "live", "--no-cache")
	if err == nil {
		t.Fatal("a malformed body must fail")
	}
	if all := out + err.Error(); strings.Contains(all, bearer) || strings.Contains(all, enc) {
		t.Fatalf("a body-read error leaked the bearer token: %s", all)
	}
}

// TestFleetExecNullMember: a null list entry is an error for that instance;
// the other instances still report.
func TestFleetExecNullMember(t *testing.T) {
	setupFleet(t, map[string]string{
		"acme/api/v1/scripts":   `{"totalCount":1,"results":[null]}`,
		"globex/api/v1/scripts": `{"totalCount":1,"results":[{"id":"1","name":"ok"}]}`,
	})
	out, err := runCLI(t, "fleet", "exec", "/api/v1/scripts", "--json", "--only", "acme,globex")
	if err != nil {
		t.Fatal(err)
	}
	rows := decodeRows(t, out)
	acme, globex := rowsFor(rows, "acme"), rowsFor(rows, "globex")
	if len(acme) != 1 || !strings.Contains(fmt.Sprint(acme[0]["error"]), "null") || len(globex) != 1 || globex[0]["name"] != "ok" {
		t.Fatalf("rows = %v", rows)
	}
}

// TestRestrictionsSurvivePinning: every restriction or opt-out setting the
// code reads (fleet.RestrictionEnv) survives MCP pinning, CLI pinning and a
// fleet child's environment, so a pinned or child process is at least as
// restricted as its parent. Read-only is checked by its effect too.
func TestRestrictionsSurvivePinning(t *testing.T) {
	for _, must := range []string{"JAMF_PRO_MCP_READ_ONLY", "JAMF_PRO_NO_LEARN", "JAMF_PRO_LEARN_NO_CAPTURE", "PRESEED_DISABLED"} {
		if !fleet.RestrictionEnv[must] {
			t.Fatalf("%s is missing from fleet.RestrictionEnv", must)
		}
	}
	for name := range fleet.RestrictionEnv {
		t.Run(name, func(t *testing.T) {
			for _, path := range []string{"mcp", "cli"} {
				setupFleet(t, nil)
				freshProcess(t)
				t.Cleanup(func() { _, _ = cliutil.SetHomeOverride("") })
				t.Setenv(fleet.EntriesEnv, "acme=s3cret")
				t.Setenv(name, "1")
				switch path {
				case "mcp":
					t.Setenv(fleet.SelectEnv, "acme")
					if err := EstablishMCPIdentity(); err != nil {
						t.Fatal(err)
					}
				case "cli":
					instanceSelection = "acme"
					if err := applyInstanceSelection(&cobra.Command{Use: "x"}, &rootFlags{}); err != nil {
						t.Fatal(err)
					}
				}
				if os.Getenv(name) != "1" {
					t.Fatalf("%s pinning dropped %s", path, name)
				}
				if name == "JAMF_PRO_MCP_READ_ONLY" && !cliutil.IsMCPReadOnlyEnv() {
					t.Fatalf("%s pinning lifted MCP read-only", path)
				}
				kept := false
				for _, kv := range fleet.ChildEnv(os.Environ()) {
					if kv == name+"=1" {
						kept = true
					}
				}
				if !kept {
					t.Fatalf("a fleet child of a %s-pinned process would lose %s", path, name)
				}
			}
		})
	}
}

// TestNoResponseValueInErrors: error messages name positions, codes and
// digests, never a value the server sent: a repeated record's id, an error
// description, a request id, a content type parameter or an unknown content
// encoding.
func TestNoResponseValueInErrors(t *testing.T) {
	err := noRepeats("/api/v1/scripts", []json.RawMessage{json.RawMessage(`{"id":"password=FleetEcho42"}`), json.RawMessage(`{"id":"password=FleetEcho42"}`)}, map[string]bool{})
	if err == nil || strings.Contains(err.Error(), "FleetEcho42") || !strings.Contains(err.Error(), "position") {
		t.Fatalf("repeat error = %v", err)
	}
	for name, handler := range map[string]func(w http.ResponseWriter){
		"error-body": func(w http.ResponseWriter) {
			w.Header().Set("Content-Type", "application/json; boundary=CTMARK")
			w.Header().Set("X-Request-ID", "REQMARK")
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"httpStatus":404,"errors":[{"code":"NOT_FOUND","field":"FIELDMARK","description":"secret DESCMARK","id":"0"}]}`))
		},
		"rate-limited": func(w http.ResponseWriter) {
			w.Header().Set("Retry-After", "0")
			w.Header().Set("X-Request-ID", "REQMARK")
			w.WriteHeader(http.StatusTooManyRequests)
		},
		"html-200": func(w http.ResponseWriter) {
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte(`<html><head><title>client_secret=HTMLMARK</title></head><body>x</body></html>`))
		},
		"cf-mitigated-403": func(w http.ResponseWriter) {
			w.Header().Set("cf-mitigated", "password=CFMARK")
			w.WriteHeader(http.StatusForbidden)
		},
		"json-overflow-200": func(w http.ResponseWriter) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"totalCount":1,"results":[{"id":1e99999,"pin":123456789123456789e9999}]}`))
		},
		"unsupported-encoding": func(w http.ResponseWriter) {
			w.Header().Set("Content-Encoding", "zstd-ENCMARK")
			_, _ = w.Write([]byte(`{}`))
		},
	} {
		t.Run(name, func(t *testing.T) {
			freshProcess(t)
			home := testenv.Isolate(t)
			t.Setenv("JAMF_PRO_HOME", home)
			if name == "rate-limited" {
				t.Setenv("PRINTING_PRESS_DOGFOOD", "1") // no retry backoff in tests
			}
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/v1/oauth/token" {
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write([]byte(`{"access_token":"tok","expires_in":1200}`))
					return
				}
				handler(w)
			}))
			defer srv.Close()
			pointAt(t, srv, "cid", "csecret")
			out, err := runCLI(t, "computer-inventory", "list", "--json", "--data-source", "live", "--no-cache")
			if err == nil {
				t.Fatal("the request must fail")
			}
			all := out + err.Error()
			for _, mark := range []string{"DESCMARK", "REQMARK", "CTMARK", "ENCMARK", "HTMLMARK", "CFMARK", "1e99999", "123456789123456789"} {
				if strings.Contains(all, mark) {
					t.Fatalf("an error quoted %s: %s", mark, all)
				}
			}
			if name == "error-body" && (strings.Contains(all, "FIELDMARK") || strings.Contains(all, "NOT_FOUND") || !strings.Contains(all, "404")) {
				t.Fatalf("an error body must be reduced to its status: %s", all)
			}
			if name == "unsupported-encoding" && !strings.Contains(all, "unsupported content encoding") {
				t.Fatalf("an unsupported encoding must be refused with fixed text: %s", all)
			}
		})
	}
}

// TestGenericWalkChecksBeforeComplete: a failing --all walk never announces
// completion first.
func TestGenericWalkChecksBeforeComplete(t *testing.T) {
	freshProcess(t)
	home := testenv.Isolate(t)
	t.Setenv("JAMF_PRO_HOME", home)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/oauth/token" {
			_, _ = w.Write([]byte(`{"access_token":"tok","expires_in":1200}`))
			return
		}
		switch r.URL.Query().Get("page") {
		case "", "0":
			_, _ = w.Write([]byte(`{"totalCount":4,"results":[{"id":"1"},{"id":"2"}]}`))
		case "1":
			_, _ = w.Write([]byte(`{"totalCount":4,"results":[{"id":"2"},{"id":"3"}]}`))
		default:
			_, _ = w.Write([]byte(`{"totalCount":4,"results":[]}`))
		}
	}))
	defer srv.Close()
	pointAt(t, srv, "cid", "csecret")
	r, w, _ := os.Pipe()
	prev := os.Stderr
	os.Stderr = w
	_, err := runCLI(t, "computer-inventory", "list", "--all", "--json", "--data-source", "live", "--no-cache")
	os.Stderr = prev
	_ = w.Close()
	stderr, _ := io.ReadAll(r)
	if err == nil {
		t.Fatal("an overlapping walk must fail")
	}
	if strings.Contains(string(stderr), `"event":"complete"`) || strings.Contains(string(stderr), "fetched ") {
		t.Fatalf("the walk announced completion before failing: %s", stderr)
	}
}

// TestSanitizeHeaders: the headers the generated client reads are reduced
// to forms that carry no server text.
func TestSanitizeHeaders(t *testing.T) {
	resp := &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("{}"))}
	resp.Header.Set("Content-Type", `application/json; boundary="CTMARK"; charset=utf-8`)
	resp.Header.Set("X-Request-ID", "REQMARK")
	resp.Header.Set("Request-ID", "REQMARK")
	resp.Header.Set("Retry-After", "soon REQMARK")
	if err := sanitizeHeaders(resp); err != nil {
		t.Fatal(err)
	}
	for _, h := range []string{"X-Request-ID", "Request-ID", "Retry-After"} {
		if resp.Header.Get(h) != "" {
			t.Errorf("%s kept: %q", h, resp.Header.Get(h))
		}
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q", ct)
	}
	for in, want := range map[string]string{
		"application/octet-stream; charset=ClientSecretEcho42": "application/octet-stream",
		"application/x-ClientSecretEcho42":                     "application/octet-stream",
		"application/problem+json; charset=UTF-8":              "application/json; charset=utf-8",
	} {
		r := &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {in}}, Body: io.NopCloser(strings.NewReader(""))}
		if _ = sanitizeHeaders(r); r.Header.Get("Content-Type") != want {
			t.Errorf("Content-Type %q became %q, want %q", in, r.Header.Get("Content-Type"), want)
		}
	}
	ok := &http.Response{StatusCode: 429, Header: http.Header{"Retry-After": {"120"}}, Body: io.NopCloser(strings.NewReader(""))}
	if _ = sanitizeHeaders(ok); ok.Header.Get("Retry-After") != "120" {
		t.Error("a numeric Retry-After must stay")
	}
}

// TestTokenResponseValidated: a 200 token response whose expires_in is a
// number too large for any parser (here the echoed numeric secret) fails
// with fixed text on both mint paths, never quoting it.
func TestTokenResponseValidated(t *testing.T) {
	const secret = "98765432109876543210987654321098765"
	for _, args := range [][]string{
		{"computer-inventory", "list", "--json", "--data-source", "live", "--no-cache"},
		{"auth", "login"},
	} {
		freshProcess(t)
		home := testenv.Isolate(t)
		t.Setenv("JAMF_PRO_HOME", home)
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if r.URL.Path == "/api/v1/oauth/token" {
				_, _ = w.Write([]byte(`{"access_token":"tok","expires_in":` + secret + `}`))
				return
			}
			_, _ = w.Write([]byte(`{"totalCount":0,"results":[]}`))
		}))
		pointAt(t, srv, "cid", secret)
		out, err := runCLI(t, args...)
		srv.Close()
		if err == nil {
			t.Fatalf("%v: an invalid token response must fail", args)
		}
		if all := out + err.Error(); strings.Contains(all, secret) || !strings.Contains(all, "token response is not valid") {
			t.Fatalf("%v: %s", args, all)
		}
	}
}

// TestCompressedResponsesChecked: the transport decodes gzip and deflate
// before checking, so compressed HTML under a JSON content type is refused
// (its title never printed), and compressed JSON still works.
func TestCompressedResponsesChecked(t *testing.T) {
	compress := func(enc string, body []byte) []byte {
		var b bytes.Buffer
		var w io.WriteCloser
		if enc == "gzip" {
			w = gzip.NewWriter(&b)
		} else {
			w, _ = flate.NewWriter(&b, flate.DefaultCompression)
		}
		_, _ = w.Write(body)
		_ = w.Close()
		return b.Bytes()
	}
	html := []byte(`<html><head><title>client_secret=ZIPMARK</title></head><body>x</body></html>`)
	for _, enc := range []string{"gzip", "deflate"} {
		for _, payload := range [][]byte{html, []byte(`{"totalCount":1,"results":[{"id":"7","general":{"name":"zipped"}}]}`)} {
			freshProcess(t)
			home := testenv.Isolate(t)
			t.Setenv("JAMF_PRO_HOME", home)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/v1/oauth/token" {
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write([]byte(`{"access_token":"tok","expires_in":1200}`))
					return
				}
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Content-Encoding", enc)
				_, _ = w.Write(compress(enc, payload))
			}))
			pointAt(t, srv, "cid", "csecret")
			out, err := runCLI(t, "computer-inventory", "list", "--json", "--data-source", "live", "--no-cache")
			srv.Close()
			isHTML := bytes.Equal(payload, html)
			all := out
			if err != nil {
				all += err.Error()
			}
			if isHTML && (err == nil || strings.Contains(all, "ZIPMARK") || !strings.Contains(all, "HTML page")) {
				t.Fatalf("%s HTML: must be refused without its title: %v %s", enc, err, all)
			}
			if !isHTML && (err != nil || !strings.Contains(out, "zipped")) {
				t.Fatalf("%s JSON: must decode and work: %v %s", enc, err, out)
			}
		}
	}
}

// TestBodyCapAndRawDeflate: every body read is capped, and a raw DEFLATE
// stream that a zlib reader accepts at first is still decoded.
func TestBodyCapAndRawDeflate(t *testing.T) {
	prev := maxBodyBytes
	maxBodyBytes = 1024
	t.Cleanup(func() { maxBodyBytes = prev })
	big := &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}},
		Body: io.NopCloser(strings.NewReader(`{"a":"` + strings.Repeat("x", 4096) + `"}`))}
	if err := sanitizeResponse(big); err == nil || !strings.Contains(err.Error(), "too large") {
		t.Fatalf("an over-cap body must be refused, got %v", err)
	}
	maxBodyBytes = prev
	raw, _ := hex.DecodeString("780100feff7b010600f9ff2278223a317d")
	r := &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}, "Content-Encoding": {"deflate"}},
		Body: io.NopCloser(bytes.NewReader(raw))}
	if err := decodeBody(r); err != nil {
		t.Fatalf("raw deflate must decode: %v", err)
	}
	if b, _ := io.ReadAll(r.Body); string(b) != `{"x":1}` {
		t.Fatalf("decoded %q", b)
	}
}

// TestFleetOutputHonesty: an unread instance's posture row has no counts, an
// empty fleet says nothing was checked, and an unreadable golden instance
// prints its error row.
func TestFleetOutputHonesty(t *testing.T) {
	setupFleet(t, map[string]string{
		"acme/api/v1/inventory-information": `{"managedComputers":0,"unmanagedComputers":0,"managedDevices":0,"unmanagedDevices":0}`,
		"acme/api/v4/computers-inventory":   `{"totalCount":0,"results":[]}`,
	})
	out, _ := runCLI(t, "fleet", "posture", "--json", "--only", "acme,down")
	down := rowsFor(decodeRows(t, out), "down")
	if len(down) != 1 || down[0]["error"] == nil {
		t.Fatalf("down row = %v", down)
	}
	for _, k := range []string{"evaluated", "compliant", "failing", "percent_compliant"} {
		if _, ok := down[0][k]; ok {
			t.Fatalf("an unread instance must carry no %s: %v", k, down[0])
		}
	}
	out, err := runCLI(t, "fleet", "baseline", "--golden", "down", "--json", "--only", "acme,down")
	if err == nil || !strings.Contains(out, `"instance": "down"`) && !strings.Contains(out, `"instance":"down"`) {
		t.Fatalf("an unreadable golden instance must print its error row and fail: %v %s", err, out)
	}
	empty := t.TempDir()
	t.Setenv(fleet.DirEnv, empty)
	out, err = runCLI(t, "fleet", "posture", "--json")
	if err != nil || !strings.Contains(out, "instances_in_scope") {
		t.Fatalf("an empty fleet must say nothing was checked: %v %s", err, out)
	}
}

// A value from an instance is data in a spreadsheet, never a formula.
func TestFleetCSVNeutralizesFormulas(t *testing.T) {
	cmd := &cobra.Command{}
	var out bytes.Buffer
	cmd.SetOut(&out)
	flags := &rootFlags{csv: true}
	rows := []map[string]any{{"instance": "acme", "name": "=1+1", "count": "-5"}}
	if err := render(cmd, flags, rows, nil, "", []string{"instance", "name", "count"}, func(r map[string]any) map[string]any { return r }); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, "'=1+1") || strings.Contains(got, ",=1+1") {
		t.Fatalf("a formula reached the CSV as a formula:\n%s", got)
	}
	if !strings.Contains(got, ",-5") {
		t.Fatalf("a plain number was changed:\n%s", got)
	}
}
