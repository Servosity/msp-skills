// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// cli-printing-press: novel-scaffold-test
// Novel command scaffold tests. Keep the wiring smoke test and add behavior cases as needed.

package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"eset-protect-pp-cli/internal/cliutil/testenv"
)

// TestNovelPatchesBacklogHelpWires smoke-tests that the patches backlog command
// resolves at runtime and renders useful --help output. Catches wiring
// regressions (missing AddCommand, panicking RunE on --help, etc.) before
// review. Keep this smoke test when adding behavior-specific cases.
func TestNovelPatchesBacklogHelpWires(t *testing.T) {
	testenv.Isolate(t)
	cmd := RootCmd()
	cmd.SetArgs([]string{"patches", "backlog", "--help"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("patches backlog --help error = %v (novel command not wired correctly?)", err)
	}
	help := out.String()
	for _, want := range []string{"Usage:", "backlog"} {
		if !strings.Contains(help, want) {
			t.Fatalf("patches backlog --help missing %q in output:\n%s", want, help)
		}
	}
}

func patchesBacklogFixtureVulns() []map[string]any {
	return []map[string]any{
		exposureVulnJSON("dev-front", "acme-hq", "Google Chrome", "CVE-2024-3400", 80, "SEVERITY_LEVEL_HIGH", "", "dp-front-chrome"),
		exposureVulnJSON("dev-recep", "acme", "Google Chrome", "CVE-2024-3401", 90, "SEVERITY_LEVEL_HIGH", "", "dp-recep-chrome"),
		exposureVulnJSON("dev-front", "acme-hq", "os", "CVE-2024-5000", 60, "SEVERITY_LEVEL_MEDIUM", "", "dp-front-os"),
		exposureVulnJSON("dev-beta", "beta", "Zoom Workplace", "CVE-2024-7000", 50, "SEVERITY_LEVEL_MEDIUM", "", "dp-missing"),
		exposureVulnJSON("dev-xray", "acme-hq", "Google Chrome", "CVE-2024-3400", 80, "SEVERITY_LEVEL_HIGH", ""),
	}
}

func patchesBacklogFixturePatches() []map[string]any {
	chrome := func(uuid, device, group, restart string) map[string]any {
		return map[string]any{"uuid": uuid, "deviceUuid": device, "deviceGroupUuid": group, "applicationPatch": map[string]any{
			"catalogId": "101", "category": patchesBacklogCategorySecurity, "developerDisplayName": "Google LLC",
			"resultVersionName": "127.0.6533.73", "severity": "SEVERITY_LEVEL_HIGH", "restartRequired": restart,
			"fixedCves": []string{"CVE-2024-3400", "cve-2024-3401", "CVE-2024-0001", "CVE-2024-0002", "CVE-2024-0003", "CVE-2024-0004", "CVE-2024-0005"},
		}}
	}
	return []map[string]any{
		chrome("dp-front-chrome", "dev-front", "acme-hq", "RESTART_REQUIREMENT_NO"),
		chrome("dp-recep-chrome", "dev-recep", "acme", "RESTART_REQUIREMENT_YES"),
		{"uuid": "dp-front-os", "deviceUuid": "dev-front", "deviceGroupUuid": "acme-hq", "osPatch": map[string]any{
			"catalogId": 202, "osFamilyId": 1, "category": patchesBacklogCategorySecurity, "resultVersionName": "10.0.22631.4317",
			"severity": "SEVERITY_LEVEL_MEDIUM", "restartRequired": "RESTART_REQUIREMENT_YES", "fixedCves": []string{"CVE-2024-5000"}}},
		{"uuid": "dp-front-reader", "deviceUuid": "dev-front", "deviceGroupUuid": "acme-hq", "applicationPatch": map[string]any{
			"catalogId": 303, "category": "PATCH_CATEGORY_REGULAR_UPDATE", "developerDisplayName": "Adobe", "resultVersionName": "24.1"}},
		{"uuid": "dp-beta-pkg", "deviceUuid": "dev-beta", "deviceGroupUuid": "beta", "packagePatch": map[string]any{
			"package": map[string]any{"displayName": "OpenSSL", "name": "openssl"}, "category": patchesBacklogCategorySecurity,
			"resultVersionName": "3.0.2-0ubuntu1.18", "fixedCves": []string{"CVE-2022-0778"}}},
	}
}

func patchesBacklogFixtureProcesses() []map[string]any {
	app := func(device, name, status string) map[string]any {
		return map[string]any{"deviceUuid": device, "displayName": name, "application": map[string]any{"displayName": name, "status": status}}
	}
	return []map[string]any{
		app("dev-front", "Google Chrome", "APPLICATION_PATCHING_STATUS_FAILED_DOWNLOAD_ERROR"),
		app("dev-recep", "Google Chrome", "APPLICATION_PATCHING_STATUS_FAILED_DOWNLOAD_ERROR"),
		app("dev-front", "google chrome", "APPLICATION_PATCHING_STATUS_FAILED_DOWNLOAD_ERROR"),
		{"deviceUuid": "dev-beta", "displayName": "OpenSSL", "package": map[string]any{"packageReference": "openssl", "status": "PATCHING_STATUS_FAILED_PREPARE_ERROR"}},
		{"deviceUuid": "dev-front", "operatingSystem": map[string]any{"status": "PATCHING_STATUS_OK_INSTALLED"}},
		app("dev-xray", "Zoom Workplace", "APPLICATION_PATCHING_STATUS_NOT_APPLICABLE"),
		{"deviceUuid": "dev-xray", "operatingSystem": map[string]any{"status": "PATCHING_STATUS_FAILED_TERMINATION_REQUIRED"}},
	}
}

func patchesBacklogFixtureInputs(t *testing.T) ([]exposureVuln, []patchesBacklogDevicePatch) {
	t.Helper()
	return exposureDecodeFixture(t, patchesBacklogFixtureVulns()...), patchesBacklogDecodePatchesFixture(t, patchesBacklogFixturePatches())
}

func patchesBacklogDecodePatchesFixture(t *testing.T, rows []map[string]any) []patchesBacklogDevicePatch {
	t.Helper()
	var rej esetRejects
	out := patchesBacklogDecodePatches(exposureItems(t, rows...), &rej)
	if rej.N != 0 {
		t.Fatalf("fixture patch rows were rejected: %+v", rej)
	}
	return out
}

func patchesBacklogDecodeProcessesFixture(t *testing.T, rows []map[string]any) []patchesBacklogProcess {
	t.Helper()
	var rej esetRejects
	out := patchesBacklogDecodeProcesses(exposureItems(t, rows...), &rej)
	if rej.N != 0 {
		t.Fatalf("fixture process rows were rejected: %+v", rej)
	}
	return out
}

func TestPatchesBacklogBuildRanksAndJoins(t *testing.T) {
	scope := exposureFixtureScope(t)
	vulns, patches := patchesBacklogFixtureInputs(t)
	got := patchesBacklogBuild(vulns, patches, scope, false, 50)
	if len(got) != 2 || got[0].Customer != "Acme Dental" || got[1].Customer != "Beta Logistics" {
		t.Fatalf("customers = %+v", got)
	}
	acme := got[0]
	if acme.TotalPatches != 3 || len(acme.Patches) != 3 {
		t.Fatalf("acme patches = %+v", acme.Patches)
	}
	chrome, osp, reader := acme.Patches[0], acme.Patches[1], acme.Patches[2]
	if chrome.Name != "Google Chrome" || chrome.Version != "127.0.6533.73" || chrome.Kind != "application" {
		t.Errorf("chrome identity (name borrowed from the vulnerability): %+v", chrome)
	}
	if chrome.AffectedDevices != 2 || chrome.MaxRiskScore != 90 || chrome.Score != 180 || chrome.CatalogID != 101 {
		t.Errorf("chrome rank: %+v", chrome)
	}
	if chrome.Category != "SECURITY_UPDATE" || chrome.Severity != "HIGH" || chrome.RestartRequired != "YES" || !chrome.Resolved {
		t.Errorf("chrome attributes: %+v", chrome)
	}
	wantCVEs := patchesBacklogCVEs{Count: 7, IDs: []string{"CVE-2024-3401", "CVE-2024-3400", "CVE-2024-0005", "CVE-2024-0004", "CVE-2024-0003"}}
	if !reflect.DeepEqual(chrome.FixedCVEs, wantCVEs) {
		t.Errorf("fixed cves = %+v, want %+v", chrome.FixedCVEs, wantCVEs)
	}
	if osp.Name != "Microsoft Windows 11 Pro update" || osp.Kind != "os" || osp.Score != 60 || osp.RestartRequired != "YES" {
		t.Errorf("os patch: %+v", osp)
	}
	if reader.Name != "Adobe update" || reader.Score != 0 || reader.Category != "REGULAR_UPDATE" || reader.FixedCVEs.IDs == nil {
		t.Errorf("unreferenced regular patch: %+v", reader)
	}
	beta := got[1]
	if len(beta.Patches) != 2 {
		t.Fatalf("beta patches = %+v", beta.Patches)
	}
	zoom, ssl := beta.Patches[0], beta.Patches[1]
	if zoom.Resolved || zoom.Name != "Zoom Workplace" || zoom.Score != 50 || zoom.RestartRequired != "" {
		t.Errorf("unresolved reference should be kept, not dropped: %+v", zoom)
	}
	if ssl.Name != "OpenSSL" || ssl.Kind != "package" || ssl.FixedCVEs.Count != 1 {
		t.Errorf("package patch: %+v", ssl)
	}
}

func TestPatchesBacklogBuildFiltersAndLimits(t *testing.T) {
	scope := exposureFixtureScope(t)
	vulns, patches := patchesBacklogFixtureInputs(t)

	sec := patchesBacklogBuild(vulns, patches, scope, true, 0)
	for _, c := range sec {
		for _, p := range c.Patches {
			if p.Resolved && p.Category != "SECURITY_UPDATE" {
				t.Errorf("--security-only kept %s (%s)", p.Name, p.Category)
			}
		}
	}
	if sec[0].TotalPatches != 2 || sec[1].TotalPatches != 2 {
		t.Errorf("security-only totals = %d, %d; want the regular Adobe patch dropped only", sec[0].TotalPatches, sec[1].TotalPatches)
	}

	lim := patchesBacklogBuild(vulns, patches, scope, false, 1)
	if lim[0].TotalPatches != 3 || len(lim[0].Patches) != 1 || lim[0].Patches[0].Name != "Google Chrome" {
		t.Errorf("limit keeps the top patch and reports the total: %+v", lim[0])
	}

	only := patchesBacklogInCustomer(patches, scope, "beta")
	if len(only) != 1 || only[0].UUID != "dp-beta-pkg" {
		t.Errorf("customer filter on local rows: %+v", only)
	}
	if got := patchesBacklogBuild(nil, only, scope, false, 0); len(got) != 1 || got[0].CustomerUUID != "beta" {
		t.Errorf("beta's patch stays beta's: %+v", got)
	}
	// A row whose group (created after the last sync) and device the mirror
	// do not know is never guessed into a customer: it is reported under the
	// UUID-less "(group not in synced tree)" customer, while a known group
	// keeps its nearest COMPANY.
	stray := patchesBacklogDecodePatchesFixture(t, []map[string]any{
		{"uuid": "dp-acme", "deviceUuid": "dev-front", "deviceGroupUuid": "acme-hq", "applicationPatch": map[string]any{"catalogId": 9, "category": patchesBacklogCategorySecurity}},
		{"uuid": "dp-new", "deviceUuid": "dev-new", "deviceGroupUuid": "group-after-sync", "applicationPatch": map[string]any{"catalogId": 9, "category": patchesBacklogCategorySecurity}},
	})
	got := map[string]string{}
	for _, c := range patchesBacklogBuild(nil, stray, scope, false, 0) {
		if c.TotalPatches != 1 || len(c.Patches) != 1 || c.Patches[0].AffectedDevices != 1 {
			t.Errorf("%s: %+v", c.Customer, c)
		}
		got[c.Customer] = c.CustomerUUID
	}
	if !reflect.DeepEqual(got, map[string]string{"Acme Dental": "acme", esetCustomerUnknownGroup: ""}) {
		t.Errorf("unknown-group attribution = %v", got)
	}
	// Local rows with an unknown group never join a --customer target.
	if kept := patchesBacklogInCustomer(stray, scope, "beta"); len(kept) != 0 {
		t.Errorf("local rows with unknown groups must not join the target: %+v", kept)
	}
	if kept := patchesBacklogInCustomer(stray, scope, "acme"); len(kept) != 1 || kept[0].UUID != "dp-acme" {
		t.Errorf("--customer acme keeps only its known-group row: %+v", kept)
	}

	empty := patchesBacklogBuild(nil, nil, exposureFixtureScope(t), false, 50)
	if empty == nil || len(empty) != 0 {
		t.Errorf("empty input should give an empty non-nil slice: %#v", empty)
	}
}

func TestPatchesBacklogFailures(t *testing.T) {
	scope := exposureFixtureScope(t)
	got := patchesBacklogFailures(patchesBacklogDecodeProcessesFixture(t, patchesBacklogFixtureProcesses()), scope)
	want := []patchesBacklogFailure{
		{Status: "FAILED_DOWNLOAD_ERROR", Application: "Google Chrome", Count: 3, Devices: 2, Customers: []string{"Acme Dental"}},
		{Status: "FAILED_PREPARE_ERROR", Application: "OpenSSL", Count: 1, Devices: 1, Customers: []string{"Beta Logistics"}},
		{Status: "FAILED_TERMINATION_REQUIRED", Application: "Operating system", Count: 1, Devices: 1, Customers: []string{"Acme Dental"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("failures =\n%+v\nwant\n%+v", got, want)
	}
	if got := patchesBacklogFailures(nil, scope); got == nil || len(got) != 0 {
		t.Errorf("empty input: %#v", got)
	}
	for status, want := range map[string]bool{
		"APPLICATION_PATCHING_STATUS_FAILED_UNINSTALL_REQUIRED": true,
		"PATCHING_STATUS_FAILED_INSTALLER_CORRUPTED":            true,
		"PATCHING_STATUS_NOT_APPLICABLE":                        false,
		"PATCHING_STATUS_SCHEDULED":                             false,
		"":                                                      false,
	} {
		if patchesBacklogIsFailure(status) != want {
			t.Errorf("isFailure(%q) != %v", status, want)
		}
	}
}

// Two distinct COMPANY groups can share a display name. Failed attempts are
// keyed by COMPANY UUID, so the same failure in both stays two customers,
// labelled apart with a UUID prefix; a unique name and a bucket stay bare.
func TestPatchesBacklogFailuresDuplicateCustomerNames(t *testing.T) {
	const acme1, acme2 = "11111111-aaaa-4aaa-8aaa-aaaaaaaaaaaa", "22222222-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	groups := []esetGroup{
		{UUID: "msp", Name: "Servosity MSP", EntityType: "DEVICE_GROUP_ENTITY_TYPE_MSP"},
		{UUID: acme1, Name: "Acme", ParentUUID: "msp", EntityType: esetCompanyEntityType},
		{UUID: acme2, Name: "ACME", ParentUUID: "msp", EntityType: esetCompanyEntityType},
		{UUID: "beta", Name: "Beta Logistics", ParentUUID: "msp", EntityType: esetCompanyEntityType},
	}
	devices := []esetDevice{
		{UUID: "dev-a1", ParentGroupUUID: acme1},
		{UUID: "dev-a1b", ParentGroupUUID: acme1},
		{UUID: "dev-a2", ParentGroupUUID: acme2},
		{UUID: "dev-b", ParentGroupUUID: "beta"},
	}
	scope := newExposureScope(groups, devices)
	fail := func(device string) map[string]any {
		return map[string]any{"deviceUuid": device, "application": map[string]any{"displayName": "Google Chrome", "status": "APPLICATION_PATCHING_STATUS_FAILED_DOWNLOAD_ERROR"}}
	}
	got := patchesBacklogFailures(patchesBacklogDecodeProcessesFixture(t, []map[string]any{fail("dev-a1"), fail("dev-a1b"), fail("dev-a2")}), scope)
	want := []patchesBacklogFailure{{Status: "FAILED_DOWNLOAD_ERROR", Application: "Google Chrome", Count: 3, Devices: 3,
		Customers: []string{"ACME (22222222)", "Acme (11111111)"}}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("two customers named Acme must stay two:\n%+v\nwant\n%+v", got, want)
	}

	got = patchesBacklogFailures(patchesBacklogDecodeProcessesFixture(t, []map[string]any{fail("dev-a1"), fail("dev-b"), fail("dev-unsynced")}), scope)
	if len(got) != 1 || !reflect.DeepEqual(got[0].Customers, []string{esetCustomerUnknownGroup, "Acme (11111111)", "Beta Logistics"}) {
		t.Fatalf("unique names and buckets keep their bare names: %+v", got)
	}
}

func TestPatchesBacklogDecodersCountRejects(t *testing.T) {
	var rej esetRejects
	patches := patchesBacklogDecodePatches(exposureItems(t,
		map[string]any{"uuid": "dp-ok", "deviceUuid": "dev-front", "applicationPatch": map[string]any{"catalogId": "101"}},
		map[string]any{"deviceUuid": "dev-front", "applicationPatch": map[string]any{"catalogId": 102}},
		map[string]any{"uuid": "dp-bad", "applicationPatch": map[string]any{"catalogId": "one-oh-three"}},
	), &rej)
	if len(patches) != 1 || rej.N != 2 || !strings.HasPrefix(rej.First, "device-patch row 1: no uuid") {
		t.Errorf("patches = %+v rejects = %+v", patches, rej)
	}
	rej = esetRejects{}
	procs := patchesBacklogDecodeProcesses(exposureItems(t,
		map[string]any{"deviceUuid": "dev-front", "application": map[string]any{"status": 7}},
		map[string]any{"deviceUuid": "dev-front", "application": map[string]any{"status": "APPLICATION_PATCHING_STATUS_FAILED_DOWNLOAD_ERROR"}},
	), &rej)
	if len(procs) != 1 || rej.N != 1 || !strings.HasPrefix(rej.First, "patching-detail row 0:") {
		t.Errorf("processes = %+v rejects = %+v", procs, rej)
	}

	// null and scalar rows decode "successfully" into a zero row; they are
	// rejected as non-objects, not as rows that merely lack a uuid.
	bad := []json.RawMessage{json.RawMessage(`null`), json.RawMessage(`7`)}
	rej = esetRejects{}
	if out := patchesBacklogDecodePatches(bad, &rej); len(out) != 0 || rej.N != 2 || rej.First != "device-patch row 0: not a JSON object" {
		t.Errorf("patches = %+v rejects = %+v", out, rej)
	}
	rej = esetRejects{}
	if out := patchesBacklogDecodeProcesses(bad, &rej); len(out) != 0 || rej.N != 2 || rej.First != "patching-detail row 0: not a JSON object" {
		t.Errorf("processes = %+v rejects = %+v", out, rej)
	}
}

func TestPatchesBacklogWindowParams(t *testing.T) {
	since := time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC)
	until := time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC)
	p := patchesBacklogWindowParams("acme", since, until)
	if p["timePeriod.startTime"] != "2026-09-14T00:00:00Z" || p["timePeriod.endTime"] != "2026-10-14T00:00:00Z" || p["deviceGroupUuid"] != "acme" {
		t.Errorf("params = %v", p)
	}
	if _, ok := patchesBacklogWindowParams("", time.Time{}, until)["timePeriod.startTime"]; ok {
		t.Error("a zero since must omit startTime")
	}
}

func TestPatchesBacklogCommandPartialFailure(t *testing.T) {
	testenv.Isolate(t)
	db := exposureSeedDB(t, map[string][]map[string]any{
		"device-groups":  exposureFixtureGroups(),
		"devices":        exposureFixtureDevices(time.Now()),
		"device-patches": patchesBacklogFixturePatches(),
	})
	fake := &exposureFakeLive{lists: map[string]func(map[string]string) (esetPageResult, error){
		"/v1/device-vulnerabilities": exposureScopedPage(t, patchesBacklogFixtureVulns()...),
		"/v1/patching-process-details": func(map[string]string) (esetPageResult, error) {
			return esetPageResult{}, fmt.Errorf("HTTP 503: service unavailable")
		},
	}}
	exposureStub(t, &patchesBacklogNewLive, fake)

	out, errOut, err := exposureRunCLI(t, "patches", "backlog", "--db", db, "--json")
	if err != nil {
		t.Fatalf("a failed failures-fetch must not fail the command: %v\n%s", err, out)
	}
	var view patchesBacklogView
	if err := json.Unmarshal([]byte(out), &view); err != nil {
		t.Fatalf("json: %v\n%s", err, out)
	}
	if len(view.Customers) != 2 || view.Customers[0].Patches[0].Name != "Google Chrome" || view.PatchSource != "local" {
		t.Errorf("backlog should still be built from the mirror: %+v", view)
	}
	if len(view.FetchFailures) != 1 || view.FetchFailures[0].Source != "patching-process-details" || view.FailedAttempts == nil {
		t.Errorf("fetch failures = %+v, failed attempts = %#v", view.FetchFailures, view.FailedAttempts)
	}
	if !strings.Contains(errOut, "1 of 2 live fetches failed") {
		t.Errorf("stderr warning missing: %q", errOut)
	}
	if p := fake.params["/v1/patching-process-details"]; p["timePeriod.startTime"] == "" || p["timePeriod.endTime"] == "" {
		t.Errorf("--since should become a timePeriod window: %v", p)
	}

	human, _, err := exposureRunCLI(t, "patches", "backlog", "--db", db, "--human-friendly")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Acme Dental (3 patch(es))", "Google Chrome", "Zoom Workplace (unresolved)", "fetch failed: patching-process-details"} {
		if !strings.Contains(human, want) {
			t.Errorf("human output missing %q:\n%s", want, human)
		}
	}

	// --customer filters local rows to the customer.
	out, _, err = exposureRunCLI(t, "patches", "backlog", "--customer", "Beta Logistics", "--security-only", "--db", db, "--json")
	if err != nil {
		t.Fatal(err)
	}
	view = patchesBacklogView{}
	_ = json.Unmarshal([]byte(out), &view)
	if len(view.Customers) != 1 || view.Customers[0].Customer != "Beta Logistics" || view.Customers[0].TotalPatches != 2 {
		t.Errorf("customer view = %+v", view.Customers)
	}
	if fake.params["/v1/device-vulnerabilities"]["deviceGroupUuid"] != "beta" {
		t.Errorf("vulnerability scan should be scoped to the customer group: %v", fake.params)
	}
	if view.RejectedRows != 0 || view.OtherCustomerRows != 0 || view.SyncedHint != "" {
		t.Errorf("clean run: rejected=%d other=%d hint=%q", view.RejectedRows, view.OtherCustomerRows, view.SyncedHint)
	}
	if !strings.Contains(out, `"rejected_rows": 0`) || !strings.Contains(out, `"other_customer_rows": 0`) {
		t.Errorf("rejected_rows and other_customer_rows must always be present:\n%s", out)
	}
}

func TestPatchesBacklogCommandLiveFallback(t *testing.T) {
	testenv.Isolate(t)
	db := exposureSeedDB(t, map[string][]map[string]any{
		"device-groups": exposureFixtureGroups(),
		"devices":       exposureFixtureDevices(time.Now()),
	})
	fake := &exposureFakeLive{lists: map[string]func(map[string]string) (esetPageResult, error){
		"/v1/device-vulnerabilities":   exposureScopedPage(t, patchesBacklogFixtureVulns()...),
		"/v1/device-patches":           exposureScopedPage(t, patchesBacklogFixturePatches()[:2]...),
		"/v1/patching-process-details": exposurePage(t, false, patchesBacklogFixtureProcesses()...),
	}}
	exposureStub(t, &patchesBacklogNewLive, fake)

	out, _, err := exposureRunCLI(t, "patches", "backlog", "--customer", "acme", "--db", db, "--json")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	var view patchesBacklogView
	if err := json.Unmarshal([]byte(out), &view); err != nil {
		t.Fatalf("json: %v\n%s", err, out)
	}
	if view.PatchSource != "live" || fake.params["/v1/device-patches"]["deviceGroupUuid"] != "acme" {
		t.Errorf("empty mirror should fetch device patches live, scoped: source=%s params=%v", view.PatchSource, fake.params)
	}
	if len(view.Customers) != 1 || view.Customers[0].Patches[0].AffectedDevices != 2 {
		t.Errorf("customers = %+v", view.Customers)
	}
	if len(view.FailedAttempts) == 0 || view.FailedAttempts[0].Count != 3 || len(view.FetchFailures) != 0 {
		t.Errorf("failed attempts = %+v, fetch failures = %+v", view.FailedAttempts, view.FetchFailures)
	}
}

func TestPatchesBacklogCommandVulnFetchFails(t *testing.T) {
	testenv.Isolate(t)
	db := exposureSeedDB(t, map[string][]map[string]any{"device-groups": exposureFixtureGroups()})
	fake := &exposureFakeLive{lists: map[string]func(map[string]string) (esetPageResult, error){
		"/v1/device-vulnerabilities": func(map[string]string) (esetPageResult, error) {
			return esetPageResult{}, fmt.Errorf("HTTP 401: unauthorized")
		},
	}}
	exposureStub(t, &patchesBacklogNewLive, fake)
	if _, _, err := exposureRunCLI(t, "patches", "backlog", "--db", db, "--json"); err == nil {
		t.Fatal("the primary vulnerability fetch failing must fail the command")
	}
}

func TestPatchesBacklogDataSource(t *testing.T) {
	testenv.Isolate(t)
	db := exposureSeedDB(t, map[string][]map[string]any{
		"device-groups":  exposureFixtureGroups(),
		"devices":        exposureFixtureDevices(time.Now()),
		"device-patches": patchesBacklogFixturePatches(),
	})
	fake := &exposureFakeLive{lists: map[string]func(map[string]string) (esetPageResult, error){
		"/v1/device-vulnerabilities":   exposureScopedPage(t, patchesBacklogFixtureVulns()...),
		"/v1/device-patches":           exposureScopedPage(t, patchesBacklogFixturePatches()[:2]...),
		"/v1/patching-process-details": exposurePage(t, false),
	}}
	exposureStub(t, &patchesBacklogNewLive, fake)

	// local: refused before any IO (no mirror read, no ESET call).
	missing := filepath.Join(t.TempDir(), "absent.db")
	_, errOut, err := exposureRunCLI(t, "patches", "backlog", "--data-source", "local", "--db", missing, "--json")
	var ce *cliError
	if !errors.As(err, &ce) || ce.code != 2 || !strings.Contains(err.Error(), "patches backlog needs live vulnerability data; use auto or live") {
		t.Fatalf("--data-source local: want usage error, got %v", err)
	}
	if len(fake.params) != 0 || strings.Contains(errOut, "no local mirror") {
		t.Errorf("--data-source local must fail before any IO: params=%v stderr=%q", fake.params, errOut)
	}

	// auto: the mirror has device patches, so they are read locally.
	out, _, err := exposureRunCLI(t, "patches", "backlog", "--db", db, "--json")
	if err != nil {
		t.Fatal(err)
	}
	var view patchesBacklogView
	_ = json.Unmarshal([]byte(out), &view)
	if view.PatchSource != "local" || fake.params["/v1/device-patches"] != nil {
		t.Errorf("auto with synced patches: source=%s params=%v", view.PatchSource, fake.params)
	}

	// live: device patches are fetched live even though the mirror has them.
	out, _, err = exposureRunCLI(t, "patches", "backlog", "--data-source", "live", "--db", db, "--json")
	if err != nil {
		t.Fatal(err)
	}
	view = patchesBacklogView{}
	_ = json.Unmarshal([]byte(out), &view)
	if view.PatchSource != "live" || fake.params["/v1/device-patches"] == nil {
		t.Errorf("live: source=%s params=%v", view.PatchSource, fake.params)
	}
	// Only the two live chrome rows are in hand, so the OS patch reference
	// is unresolved and the mirror-only Adobe patch is absent.
	for _, p := range view.Customers[0].Patches {
		if p.Name == "Adobe update" {
			t.Errorf("--data-source live must not read synced device patches: %+v", view.Customers[0].Patches)
		}
	}
}

// A missing mirror must still scan ESET live and report what it finds.
func TestPatchesBacklogCommandNoMirror(t *testing.T) {
	testenv.Isolate(t)
	fake := &exposureFakeLive{lists: map[string]func(map[string]string) (esetPageResult, error){
		"/v1/device-vulnerabilities":   exposureScopedPage(t, patchesBacklogFixtureVulns()...),
		"/v1/device-patches":           exposureScopedPage(t, patchesBacklogFixturePatches()...),
		"/v1/patching-process-details": exposurePage(t, false, patchesBacklogFixtureProcesses()...),
	}}
	exposureStub(t, &patchesBacklogNewLive, fake)
	missing := filepath.Join(t.TempDir(), "absent.db")

	out, errOut, err := exposureRunCLI(t, "patches", "backlog", "--db", missing, "--json")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	var view patchesBacklogView
	if err := json.Unmarshal([]byte(out), &view); err != nil {
		t.Fatalf("json: %v\n%s", err, out)
	}
	if len(view.Customers) != 1 || view.Customers[0].Customer != exposureUnsyncedBucket || view.Customers[0].CustomerUUID != "" {
		t.Fatalf("every patch should land in the unsynced bucket: %+v", view.Customers)
	}
	if view.PatchSource != "live" || view.ScannedVulnerabilities != 5 || view.Customers[0].TotalPatches == 0 {
		t.Errorf("view = %+v", view)
	}
	if len(view.FailedAttempts) == 0 || !reflect.DeepEqual(view.FailedAttempts[0].Customers, []string{exposureUnsyncedBucket}) {
		t.Errorf("failed attempts = %+v", view.FailedAttempts)
	}
	if !strings.Contains(view.Note, "sync --resources devices,device-groups") || !strings.Contains(view.SyncedHint, "no local mirror") {
		t.Errorf("note = %q synced_hint = %q", view.Note, view.SyncedHint)
	}
	if !strings.Contains(errOut, "no local mirror") {
		t.Errorf("stderr = %q", errOut)
	}

	const groupUUID = "6f1c2d3e-4a5b-4c6d-8e7f-9a0b1c2d3e4f"
	if _, _, err := exposureRunCLI(t, "patches", "backlog", "--customer", groupUUID, "--db", missing, "--json"); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/v1/device-vulnerabilities", "/v1/device-patches", "/v1/patching-process-details"} {
		if fake.params[path]["deviceGroupUuid"] != groupUUID {
			t.Errorf("%s should be scoped to %s: %v", path, groupUUID, fake.params[path])
		}
	}
	fake.params = nil
	_, _, err = exposureRunCLI(t, "patches", "backlog", "--customer", "Acme Dental", "--db", missing, "--json")
	var ce *cliError
	if !errors.As(err, &ce) || ce.code != 2 || len(fake.params) != 0 {
		t.Errorf("a customer name without a mirror: want usage error and no ESET call, got %v %v", err, fake.params)
	}
}

func TestPatchesBacklogCommandNestedCustomerAndRejects(t *testing.T) {
	testenv.Isolate(t)
	labVuln := exposureVulnJSON("dev-lab", "acme-lab", "Google Chrome", "CVE-2024-3400", 99, "SEVERITY_LEVEL_HIGH", "", "dp-lab-chrome")
	badVuln := exposureVulnJSON("dev-front", "acme-hq", "Google Chrome", "CVE-2024-3400", 80, "SEVERITY_LEVEL_HIGH", "")
	badVuln["applicationVulnerability"].(map[string]any)["riskScore"] = "very high"
	labPatch := map[string]any{"uuid": "dp-lab-chrome", "deviceUuid": "dev-lab", "deviceGroupUuid": "acme-lab", "applicationPatch": map[string]any{
		"catalogId": 101, "category": patchesBacklogCategorySecurity, "resultVersionName": "127.0.6533.73"}}
	noUUIDPatch := map[string]any{"deviceUuid": "dev-front", "deviceGroupUuid": "acme-hq", "applicationPatch": map[string]any{"catalogId": 5}}
	labProcess := map[string]any{"deviceUuid": "dev-lab", "application": map[string]any{"displayName": "Google Chrome", "status": "APPLICATION_PATCHING_STATUS_FAILED_DOWNLOAD_ERROR"}}
	badProcess := map[string]any{"deviceUuid": "dev-front", "application": []int{1}}
	devices := exposureFixtureDevices(time.Now())
	devices = append(devices, map[string]any{"uuid": "dev-lab", "displayName": "LAB-01", "parentGroupUuid": "acme-lab"})
	db := exposureSeedDB(t, map[string][]map[string]any{"device-groups": exposureFixtureGroups(), "devices": devices})
	// Patching details carry no group, so the fake cannot scope them; drop
	// Beta's row as ESET's group scope would and keep Acme Labs' nested one.
	procs := make([]map[string]any, 0)
	for _, p := range patchesBacklogFixtureProcesses() {
		if p["deviceUuid"] != "dev-beta" {
			procs = append(procs, p)
		}
	}
	fake := &exposureFakeLive{lists: map[string]func(map[string]string) (esetPageResult, error){
		"/v1/device-vulnerabilities":   exposureScopedPage(t, append(patchesBacklogFixtureVulns(), labVuln, badVuln)...),
		"/v1/device-patches":           exposureScopedPage(t, append(patchesBacklogFixturePatches(), labPatch, noUUIDPatch)...),
		"/v1/patching-process-details": exposurePage(t, false, append(procs, labProcess, badProcess)...),
	}}
	exposureStub(t, &patchesBacklogNewLive, fake)

	out, errOut, err := exposureRunCLI(t, "patches", "backlog", "--customer", "acme", "--db", db, "--json")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	var view patchesBacklogView
	if err := json.Unmarshal([]byte(out), &view); err != nil {
		t.Fatalf("json: %v\n%s", err, out)
	}
	if len(view.Customers) != 1 || view.Customers[0].CustomerUUID != "acme" {
		t.Fatalf("customers = %+v", view.Customers)
	}
	chrome := view.Customers[0].Patches[0]
	if chrome.Name != "Google Chrome" || chrome.AffectedDevices != 2 || chrome.MaxRiskScore != 90 {
		t.Errorf("Acme Labs' device and risk must not join Acme's chrome row: %+v", chrome)
	}
	// One nested row from each live list: vulnerability, device patch, process.
	if view.OtherCustomerRows != 3 {
		t.Errorf("other_customer_rows = %d, want 3", view.OtherCustomerRows)
	}
	for _, f := range view.FailedAttempts {
		if f.Application == "Google Chrome" && f.Devices != 2 {
			t.Errorf("Acme Labs' failed attempt must be left out: %+v", f)
		}
	}
	if view.RejectedRows != 3 || !strings.Contains(view.Note, "3 row(s) could not be decoded") {
		t.Errorf("rejected_rows = %d note = %q", view.RejectedRows, view.Note)
	}
	if strings.Count(errOut, "could not be decoded") != 1 || !strings.Contains(errOut, "warning: 3 row(s)") {
		t.Errorf("one stderr warning expected: %q", errOut)
	}
	human, _, err := exposureRunCLI(t, "patches", "backlog", "--customer", "acme", "--db", db, "--human-friendly")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(human, "nested under Acme Dental") || !strings.Contains(human, "could not be decoded") {
		t.Errorf("human output must name the left-out rows:\n%s", human)
	}
}

// TestPatchesBacklogRestartAggregation (handfixes.json:
// profile-restart-keeps-unknown): aggregating a patch across devices turned an
// unknown restart requirement into NO when another device said NO.
func TestPatchesBacklogRestartAggregation(t *testing.T) {
	for _, tc := range []struct {
		seen map[string]bool
		want string
	}{
		{map[string]bool{"YES": true, "NO": true}, "YES"},
		{map[string]bool{"NO": true, "UNSPECIFIED": true}, "UNSPECIFIED"},
		{map[string]bool{"NO": true, "": true}, "UNSPECIFIED"},
		{map[string]bool{"NO": true}, "NO"},
		{map[string]bool{}, ""},
	} {
		if got := patchesBacklogRestart(tc.seen); got != tc.want {
			t.Errorf("patchesBacklogRestart(%v) = %q, want %q", tc.seen, got, tc.want)
		}
	}
}
