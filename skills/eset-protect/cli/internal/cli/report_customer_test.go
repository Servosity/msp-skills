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

// TestNovelReportCustomerHelpWires smoke-tests that the report customer command
// resolves at runtime and renders useful --help output. Catches wiring
// regressions (missing AddCommand, panicking RunE on --help, etc.) before
// review. Keep this smoke test when adding behavior-specific cases.
func TestNovelReportCustomerHelpWires(t *testing.T) {
	testenv.Isolate(t)
	cmd := RootCmd()
	cmd.SetArgs([]string{"report", "customer", "--help"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("report customer --help error = %v (novel command not wired correctly?)", err)
	}
	help := out.String()
	for _, want := range []string{"Usage:", "customer"} {
		if !strings.Contains(help, want) {
			t.Fatalf("report customer --help missing %q in output:\n%s", want, help)
		}
	}
}

func reportCustomerAgo(now time.Time, d time.Duration) string {
	return now.Add(-d).UTC().Format(time.RFC3339)
}

const reportCustomerDay = 24 * time.Hour

func reportCustomerFixtureDetections(now time.Time) []map[string]any {
	det := func(uuid, device string, ago time.Duration, sev, cat, typ string, responses ...string) map[string]any {
		rs := make([]map[string]any, 0, len(responses))
		for _, r := range responses {
			rs = append(rs, map[string]any{"displayName": r, "protectionName": "Real-time file system protection"})
		}
		return map[string]any{"uuid": uuid, "displayName": "Win32/Agent.XYZ", "category": cat, "typeName": typ,
			"occurTime": reportCustomerAgo(now, ago), "severityLevel": sev, "context": map[string]any{"deviceUuid": device}, "responses": rs}
	}
	return []map[string]any{
		det("det1", "dev-front", 5*reportCustomerDay, "SEVERITY_LEVEL_HIGH", "DETECTION_CATEGORY_ANTIVIRUS", "Trojan", "Cleaned by deleting"),
		det("det2", "dev-recep", 10*reportCustomerDay, "SEVERITY_LEVEL_MEDIUM", "", "Potentially unwanted application"),
		det("det3", "dev-front", 200*reportCustomerDay, "SEVERITY_LEVEL_HIGH", "DETECTION_CATEGORY_ANTIVIRUS", "Trojan", "Cleaned by deleting"),
		det("det4", "dev-beta", 1*reportCustomerDay, "SEVERITY_LEVEL_HIGH", "DETECTION_CATEGORY_ANTIVIRUS", "Trojan", "Cleaned by deleting"),
		det("det5", "dev-xray", 2*reportCustomerDay, "SEVERITY_LEVEL_LOW", "DETECTION_CATEGORY_WEB_ACCESS", "", "Connection terminated", "Cleaned by deleting"),
	}
}

func reportCustomerFixtureIncidents(now time.Time) []map[string]any {
	inc := func(uuid string, devices []string, created, updated time.Duration, status, sev, triage, resp string) map[string]any {
		return map[string]any{"uuid": uuid, "displayName": "Suspicious PowerShell " + uuid, "deviceUuids": devices, "detectionUuids": []string{},
			"createTime": reportCustomerAgo(now, created), "updateTime": reportCustomerAgo(now, updated), "status": status,
			"severity": sev, "triageDuration": triage, "responseDuration": resp}
	}
	return []map[string]any{
		inc("inc-a", []string{"dev-front"}, 20*reportCustomerDay, 15*reportCustomerDay, "INCIDENT_STATUS_CLOSED", "INCIDENT_SEVERITY_LEVEL_HIGH", "3600s", "7200s"),
		inc("inc-b", []string{}, 3*reportCustomerDay, 3*reportCustomerDay, "INCIDENT_STATUS_OPEN", "INCIDENT_SEVERITY_LEVEL_MEDIUM", "", ""),
		inc("inc-c", []string{}, 4*reportCustomerDay, 4*reportCustomerDay, "INCIDENT_STATUS_IN_PROGRESS", "INCIDENT_SEVERITY_LEVEL_LOW", "", ""),
		inc("inc-d", []string{"dev-beta"}, 2*reportCustomerDay, 2*reportCustomerDay, "INCIDENT_STATUS_OPEN", "INCIDENT_SEVERITY_LEVEL_HIGH", "60s", "60s"),
		inc("inc-e", []string{"dev-xray"}, 300*reportCustomerDay, 280*reportCustomerDay, "INCIDENT_STATUS_CLOSED", "INCIDENT_SEVERITY_LEVEL_LOW", "10s", "10s"),
		inc("inc-f", []string{"dev-recep"}, 200*reportCustomerDay, 200*reportCustomerDay, "INCIDENT_STATUS_WAITING_FOR_INPUT", "INCIDENT_SEVERITY_LEVEL_LOW", "", ""),
	}
}

func reportCustomerHydrated(uuid string, devices []string, triage string) func() (json.RawMessage, error) {
	return func() (json.RawMessage, error) {
		return json.Marshal(map[string]any{"incident": map[string]any{"uuid": uuid, "deviceUuids": devices, "triageDuration": triage}})
	}
}

func reportCustomerFixtureLive(t *testing.T, now time.Time) *exposureFakeLive {
	app := func(device, name, status string) map[string]any {
		return map[string]any{"deviceUuid": device, "displayName": name, "application": map[string]any{"displayName": name, "status": status}}
	}
	return &exposureFakeLive{
		lists: map[string]func(map[string]string) (esetPageResult, error){
			"/v1/device-vulnerabilities": exposureScopedPage(t, patchesBacklogFixtureVulns()...),
			"/v1/patching-process-details": exposurePage(t, false,
				app("dev-front", "Google Chrome", "APPLICATION_PATCHING_STATUS_FAILED_DOWNLOAD_ERROR"),
				app("dev-recep", "Google Chrome", "APPLICATION_PATCHING_STATUS_FAILED_DOWNLOAD_ERROR"),
				app("dev-front", "Google Chrome", "APPLICATION_PATCHING_STATUS_FAILED_DOWNLOAD_ERROR"),
				app("dev-front", "Zoom Workplace", "APPLICATION_PATCHING_STATUS_OK_INSTALLED"),
				map[string]any{"deviceUuid": "dev-xray", "operatingSystem": map[string]any{"status": "PATCHING_STATUS_FAILED_TERMINATION_REQUIRED"}},
			),
			"/v1/scans/recent": exposurePage(t, false,
				map[string]any{"deviceUuid": "dev-front", "startTime": reportCustomerAgo(now, 25*time.Hour), "endTime": reportCustomerAgo(now, reportCustomerDay), "endedSuccessfully": true, "vulnerabilityCount": 3},
				map[string]any{"deviceUuid": "dev-front", "endTime": reportCustomerAgo(now, 2*reportCustomerDay), "endedSuccessfully": true},
				map[string]any{"deviceUuid": "dev-recep", "endTime": reportCustomerAgo(now, 2*reportCustomerDay), "endedSuccessfully": false},
				map[string]any{"deviceUuid": "dev-xray", "endTime": reportCustomerAgo(now, 200*reportCustomerDay), "endedSuccessfully": true},
			),
		},
		gets: map[string]func() (json.RawMessage, error){
			"/v2/incidents/inc-b": reportCustomerHydrated("inc-b", []string{"dev-recep"}, "1800s"),
			"/v2/incidents/inc-c": reportCustomerHydrated("inc-c", []string{"dev-beta"}, "5s"),
		},
	}
}

func reportCustomerSeed(t *testing.T, now time.Time) string {
	t.Helper()
	return exposureSeedDB(t, map[string][]map[string]any{
		"device-groups":  exposureFixtureGroups(),
		"devices":        exposureFixtureDevices(now),
		"detections":     reportCustomerFixtureDetections(now),
		"incidents":      reportCustomerFixtureIncidents(now),
		"device-patches": patchesBacklogFixturePatches(),
	})
}

func reportCustomerRun(t *testing.T, args ...string) (reportCustomerView, string) {
	t.Helper()
	out, errOut, err := exposureRunCLI(t, append([]string{"report", "customer"}, args...)...)
	if err != nil {
		t.Fatalf("report customer %v: %v\n%s\n%s", args, err, out, errOut)
	}
	var view reportCustomerView
	if err := json.Unmarshal([]byte(out), &view); err != nil {
		t.Fatalf("json: %v\n%s", err, out)
	}
	return view, errOut
}

// ---- pure logic ----

func TestReportCustomerParseDuration(t *testing.T) {
	cases := []struct {
		in     string
		want   float64
		wantOK bool
	}{
		{`"3600s"`, 3600, true},
		{`"1.5s"`, 1.5, true},
		{`"0s"`, 0, true},
		{`"3600.000000001s"`, 3600.000000001, true},
		{`"1h30m"`, 5400, true},
		{`{"seconds":"90","nanos":500000000}`, 90.5, true},
		{`{"seconds":120}`, 120, true},
		{`42`, 42, true},
		{`""`, 0, false},
		{`null`, 0, false},
		{``, 0, false},
		{`"soon"`, 0, false},
		{`"s"`, 0, false},
	}
	for _, c := range cases {
		got, ok := reportCustomerParseDuration(json.RawMessage(c.in))
		if ok != c.wantOK || got != c.want {
			t.Errorf("parse(%s) = %v, %v; want %v, %v", c.in, got, ok, c.want, c.wantOK)
		}
	}
}

func reportCustomerDecodeFixture[T any](t *testing.T, rows []map[string]any) []T {
	t.Helper()
	var rej esetRejects
	out := reportCustomerDecode[T](exposureItems(t, rows...), "fixture", &rej)
	if rej.N != 0 {
		t.Fatalf("fixture rows were rejected: %+v", rej)
	}
	return out
}

// reportCustomerAttribSet attributes the listed devices to the customer and
// every other device to another known customer.
func reportCustomerAttribSet(set map[string]bool) func(string) esetAttribution {
	return func(id string) esetAttribution {
		if set[id] {
			return esetAttribIn
		}
		return esetAttribOther
	}
}

func TestReportCustomerIncidentsOf(t *testing.T) {
	now := time.Now()
	incs := reportCustomerDecodeFixture[reportCustomerIncident](t, reportCustomerFixtureIncidents(now))
	incs[1].DeviceUUIDs = []string{"dev-recep"} // as if hydrated
	incs[1].TriageDuration = json.RawMessage(`"1800s"`)
	acme := map[string]bool{"dev-front": true, "dev-recep": true, "dev-xray": true}
	inScope := reportCustomerAttribSet(acme)
	got := reportCustomerIncidentsOf(incs, inScope, false, now.Add(-90*reportCustomerDay), now)
	if got.Opened != 2 || got.Closed != 1 || got.OpenNow != 2 {
		t.Errorf("opened/closed/open_now = %d/%d/%d, want 2/1/2", got.Opened, got.Closed, got.OpenNow)
	}
	if got.MeanTriageSeconds == nil || *got.MeanTriageSeconds != 2700 || got.MeanResponseSeconds == nil || *got.MeanResponseSeconds != 7200 {
		t.Errorf("means = %v / %v, want 2700 / 7200", got.MeanTriageSeconds, got.MeanResponseSeconds)
	}
	if !reflect.DeepEqual(got.BySeverity, map[string]int{"HIGH": 1, "MEDIUM": 1}) {
		t.Errorf("by severity = %v", got.BySeverity)
	}

	// Tenant mode attributes everything, even rows with no devices.
	all := reportCustomerIncidentsOf(incs, reportCustomerAttribSet(nil), true, now.Add(-90*reportCustomerDay), now)
	if all.Opened != 4 || all.OpenNow != 4 {
		t.Errorf("tenant mode = %+v", all)
	}

	empty := reportCustomerIncidentsOf(nil, inScope, false, time.Time{}, now)
	if empty.BySeverity == nil || empty.MeanTriageSeconds != nil || empty.Opened != 0 {
		t.Errorf("empty input = %+v", empty)
	}
}

func TestReportCustomerHydrationPlan(t *testing.T) {
	now := time.Now()
	incs := reportCustomerDecodeFixture[reportCustomerIncident](t, reportCustomerFixtureIncidents(now))
	// A closed incident without devices, untouched for a year, is not worth a call.
	incs = append(incs, reportCustomerIncident{UUID: "inc-old", Status: reportCustomerIncidentClosed,
		CreateTime: reportCustomerAgo(now, 400*reportCustomerDay), UpdateTime: reportCustomerAgo(now, 390*reportCustomerDay)})
	plan, skipped := reportCustomerHydrationPlan(incs, now.Add(-90*reportCustomerDay), now, 1)
	if !reflect.DeepEqual(plan, []string{"inc-b"}) || skipped != 1 {
		t.Errorf("plan = %v skipped = %d; want newest (inc-b) first, inc-c skipped", plan, skipped)
	}
	plan, skipped = reportCustomerHydrationPlan(incs, now.Add(-90*reportCustomerDay), now, 50)
	if !reflect.DeepEqual(plan, []string{"inc-b", "inc-c"}) || skipped != 0 {
		t.Errorf("plan = %v skipped = %d", plan, skipped)
	}
	if plan, skipped := reportCustomerHydrationPlan(nil, time.Time{}, now, 50); plan == nil || len(plan) != 0 || skipped != 0 {
		t.Errorf("empty input: %#v %d", plan, skipped)
	}
}

func TestReportCustomerDetectionsOf(t *testing.T) {
	now := time.Now()
	dets := reportCustomerDecodeFixture[reportCustomerDetection](t, reportCustomerFixtureDetections(now))
	acme := map[string]bool{"dev-front": true, "dev-recep": true, "dev-xray": true}
	got := reportCustomerDetectionsOf(dets, reportCustomerAttribSet(acme), now.Add(-90*reportCustomerDay), now)
	if got.Total != 3 || got.Unhandled != 1 {
		t.Errorf("total/unhandled = %d/%d, want 3/1 (det3 is outside the window, det4 is another customer)", got.Total, got.Unhandled)
	}
	if !reflect.DeepEqual(got.BySeverity, map[string]int{"HIGH": 1, "MEDIUM": 1, "LOW": 1}) {
		t.Errorf("by severity = %v", got.BySeverity)
	}
	wantCats := []reportCustomerCount{{"ANTIVIRUS", 1}, {"Potentially unwanted application", 1}, {"WEB_ACCESS", 1}}
	if !reflect.DeepEqual(got.ByCategory, wantCats) {
		t.Errorf("by category = %v", got.ByCategory)
	}
	if !reflect.DeepEqual(got.Responses, map[string]int{"Cleaned by deleting": 2, "Connection terminated": 1}) {
		t.Errorf("responses = %v", got.Responses)
	}
	empty := reportCustomerDetectionsOf(nil, func(string) esetAttribution { return esetAttribIn }, time.Time{}, now)
	if empty.ByCategory == nil || empty.BySeverity == nil || empty.Responses == nil || empty.Total != 0 {
		t.Errorf("empty input = %#v", empty)
	}
	many := map[string]int{"a": 1, "b": 5, "c": 3, "d": 3, "e": 2, "f": 9}
	if top := reportCustomerTopCounts(many, 5); len(top) != 5 || top[0].Name != "f" || top[2].Name != "c" || top[3].Name != "d" {
		t.Errorf("top counts = %v", top)
	}
}

func TestReportCustomerHealthAndCoverage(t *testing.T) {
	now := time.Now()
	scope := exposureFixtureScope(t)
	acme := reportCustomerDevicesIn(func() []esetDevice {
		out := make([]esetDevice, 0)
		for _, d := range scope.devices {
			out = append(out, d)
		}
		return out
	}(), scope.tree, "acme", false)
	if len(acme) != 3 {
		t.Fatalf("acme subtree = %d devices, want 3", len(acme))
	}
	h := reportCustomerHealthOf(acme, now, reportCustomerStaleAfter)
	want := reportCustomerHealth{Devices: 3, OK: 1, AttentionRecommended: 1, AttentionRequired: 1, Stale: 2, StaleDays: 7, Muted: 1}
	if !reflect.DeepEqual(h, want) {
		t.Errorf("health = %+v, want %+v (10-day-old and never-synced devices are stale)", h, want)
	}
	scans := []reportCustomerScan{
		{DeviceUUID: "dev-front", EndTime: reportCustomerAgo(now, reportCustomerDay), EndedSuccessfully: true},
		{DeviceUUID: "dev-xray", StartTime: reportCustomerAgo(now, 2*reportCustomerDay), EndedSuccessfully: true},
		{DeviceUUID: "dev-recep", EndTime: reportCustomerAgo(now, reportCustomerDay)},
		{DeviceUUID: "dev-beta", EndTime: reportCustomerAgo(now, reportCustomerDay), EndedSuccessfully: true},
	}
	sc := reportCustomerScanCoverageOf(scans, acme, now.Add(-30*reportCustomerDay), now)
	if sc.Devices != 3 || sc.DevicesScanned != 2 || sc.FailedScans != 1 || sc.CoveragePct != 66.7 {
		t.Errorf("coverage = %+v", sc)
	}
	if z := reportCustomerScanCoverageOf(nil, nil, time.Time{}, now); z.CoveragePct != 0 || z.Devices != 0 {
		t.Errorf("empty coverage = %+v", z)
	}
}

func TestReportCustomerVulnsAndPatchesOf(t *testing.T) {
	vulns := exposureDecodeFixture(t, patchesBacklogFixtureVulns()...)
	v := reportCustomerVulnsOf(vulns, true)
	if v.Total != 5 || v.VulnerableDevices != 4 || v.DistinctCVEs != 4 || !v.Truncated {
		t.Errorf("vulns = %+v", v)
	}
	if v.TopCVEs[0].CVE != "CVE-2024-3401" || v.TopCVEs[1].CVE != "CVE-2024-3400" || v.TopCVEs[1].Devices != 2 || v.TopCVEs[1].Software != "Google Chrome" {
		t.Errorf("top cves = %+v", v.TopCVEs)
	}
	if !reflect.DeepEqual(v.BySeverity, map[string]int{"HIGH": 3, "MEDIUM": 2}) {
		t.Errorf("by severity = %v", v.BySeverity)
	}
	if e := reportCustomerVulnsOf(nil, false); e.TopCVEs == nil || e.BySeverity == nil || e.Total != 0 {
		t.Errorf("empty vulns = %#v", e)
	}

	patches := patchesBacklogDecodePatchesFixture(t, patchesBacklogFixturePatches())
	procs := patchesBacklogDecodeProcessesFixture(t, patchesBacklogFixtureProcesses())
	p := reportCustomerPatchesOf(patches, procs)
	if p.PendingSecurityPatches != 4 || p.DevicesWithPendingSecurity != 3 || p.FailedAttempts != 5 || p.FailedDevices != 4 {
		t.Errorf("patches = %+v", p)
	}
	if !reflect.DeepEqual(p.FailedByStatus, map[string]int{"FAILED_DOWNLOAD_ERROR": 3, "FAILED_PREPARE_ERROR": 1, "FAILED_TERMINATION_REQUIRED": 1}) {
		t.Errorf("failed by status = %v", p.FailedByStatus)
	}
}

func TestReportCustomerResolve(t *testing.T) {
	scope := exposureFixtureScope(t)
	if g, all, err := reportCustomerResolve(scope.tree, "acme dental"); err != nil || all || g.UUID != "acme" {
		t.Errorf("by name: %+v %v %v", g, all, err)
	}
	if _, _, err := reportCustomerResolve(scope.tree, "tenant"); err == nil {
		t.Error("'tenant' must not resolve on an MSP tree")
	}
	single := newEsetTree([]esetGroup{{UUID: "all", Name: "All"}})
	for _, q := range []string{"tenant", "(tenant)", " Tenant "} {
		if g, all, err := reportCustomerResolve(single, q); err != nil || !all || g.Name != esetCustomerTenant {
			t.Errorf("single tenant %q: %+v %v %v", q, g, all, err)
		}
	}
	if _, _, err := reportCustomerResolve(single, "Acme Dental"); err == nil || !strings.Contains(err.Error(), "'tenant'") {
		t.Errorf("single tenant with a name should explain 'tenant': %v", err)
	}
}

// ---- command ----

func TestReportCustomerEndToEnd(t *testing.T) {
	testenv.Isolate(t)
	now := time.Now()
	db := reportCustomerSeed(t, now)
	fake := reportCustomerFixtureLive(t, now)
	exposureStub(t, &reportCustomerNewLive, fake)

	view, _ := reportCustomerRun(t, "Acme Dental", "--db", db, "--since", "90d", "--json")
	if view.Customer != (reportCustomerRef{Name: "Acme Dental", UUID: "acme", GroupPath: "Servosity MSP / Acme Dental"}) {
		t.Errorf("customer = %+v", view.Customer)
	}
	if view.Window.Since == "" || view.Window.Until == "" || view.GeneratedAt == "" {
		t.Errorf("window = %+v generated_at = %q", view.Window, view.GeneratedAt)
	}
	if len(view.FetchFailures) != 0 {
		t.Fatalf("no section should fail: %+v", view.FetchFailures)
	}
	if h := view.Health; h == nil || h.Devices != 3 || h.AttentionRequired != 1 || h.Stale != 2 || h.Muted != 1 {
		t.Errorf("health = %+v", view.Health)
	}
	if d := view.Detections; d == nil || d.Total != 3 || d.Unhandled != 1 || d.Responses["Cleaned by deleting"] != 2 {
		t.Errorf("detections = %+v", view.Detections)
	}
	in := view.Incidents
	if in == nil || in.Opened != 2 || in.Closed != 1 || in.OpenNow != 2 || in.Hydrated != 2 || in.HydrationSkipped != 0 || in.HydrationFailed != 0 {
		t.Fatalf("incidents = %+v", in)
	}
	if in.MeanTriageSeconds == nil || *in.MeanTriageSeconds != 2700 || in.MeanResponseSeconds == nil || *in.MeanResponseSeconds != 7200 {
		t.Errorf("incident means = %v / %v", in.MeanTriageSeconds, in.MeanResponseSeconds)
	}
	if v := view.Vulnerabilities; v == nil || v.VulnerableDevices != 3 || v.Total != 4 || v.TopCVEs[0].CVE != "CVE-2024-3401" {
		t.Errorf("vulnerabilities = %+v", view.Vulnerabilities)
	}
	if fake.params["/v1/device-vulnerabilities"]["deviceGroupUuid"] != "acme" || fake.params["/v1/scans/recent"]["deviceGroupUuid"] != "acme" {
		t.Errorf("live sections must be scoped to the customer group: %v", fake.params)
	}
	if p := view.Patches; p == nil || p.PendingSecurityPatches != 3 || p.DevicesWithPendingSecurity != 2 || p.FailedAttempts != 4 || p.FailedDevices != 3 || p.PatchSource != "local" {
		t.Errorf("patches = %+v", view.Patches)
	}
	pp := fake.params["/v1/patching-process-details"]
	if pp["deviceGroupUuid"] != "acme" || pp["timePeriod.startTime"] != view.Window.Since {
		t.Errorf("patching details params = %v", pp)
	}
	if sc := view.ScanCoverage; sc == nil || sc.Devices != 3 || sc.DevicesScanned != 1 || sc.FailedScans != 1 || sc.CoveragePct != 33.3 || sc.Note == "" {
		t.Errorf("scan coverage = %+v", view.ScanCoverage)
	}
	if view.RejectedRows != 0 || view.OtherCustomerRows != 0 {
		t.Errorf("clean run: rejected=%d other=%d", view.RejectedRows, view.OtherCustomerRows)
	}
	raw, _, err := exposureRunCLI(t, "report", "customer", "Acme Dental", "--db", db, "--json")
	if err != nil || !strings.Contains(raw, `"rejected_rows": 0`) || !strings.Contains(raw, `"other_customer_rows": 0`) {
		t.Errorf("rejected_rows and other_customer_rows must always be present (%v):\n%s", err, raw)
	}
}

func TestReportCustomerHumanOutput(t *testing.T) {
	testenv.Isolate(t)
	now := time.Now()
	db := reportCustomerSeed(t, now)
	fake := reportCustomerFixtureLive(t, now)
	fake.lists["/v1/scans/recent"] = func(map[string]string) (esetPageResult, error) {
		return esetPageResult{}, fmt.Errorf("HTTP 503: unavailable")
	}
	exposureStub(t, &reportCustomerNewLive, fake)
	out, _, err := exposureRunCLI(t, "report", "customer", "Acme Dental", "--db", db, "--human-friendly")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Servosity MSP / Acme Dental", "Health: 3 devices", "Incidents: 2 opened, 1 closed (updated in window), 2 open now; mean triage 45m0s, mean response 2h0m0s",
		"CVE-2024-3401 risk 90", "Patches: 3 pending security", "Scan coverage: unavailable: HTTP 503"} {
		if !strings.Contains(out, want) {
			t.Errorf("human output missing %q:\n%s", want, out)
		}
	}
}

func TestReportCustomerSectionFailureIsolated(t *testing.T) {
	testenv.Isolate(t)
	now := time.Now()
	db := reportCustomerSeed(t, now)
	fake := reportCustomerFixtureLive(t, now)
	boom := func(map[string]string) (esetPageResult, error) {
		return esetPageResult{}, fmt.Errorf("HTTP 503: unavailable")
	}
	fake.lists["/v1/device-vulnerabilities"] = boom
	fake.lists["/v1/scans/recent"] = boom
	fake.gets["/v2/incidents/inc-b"] = func() (json.RawMessage, error) { return nil, fmt.Errorf("HTTP 500: boom") }
	exposureStub(t, &reportCustomerNewLive, fake)

	view, errOut := reportCustomerRun(t, "acme", "--db", db, "--json")
	failed := map[string]bool{}
	for _, f := range view.FetchFailures {
		failed[f.Section] = true
	}
	if !failed["vulnerabilities"] || !failed["scan_coverage"] || !failed["incidents"] || len(failed) != 3 {
		t.Fatalf("fetch failures = %+v", view.FetchFailures)
	}
	if view.Vulnerabilities != nil || view.ScanCoverage != nil {
		t.Errorf("failed sections must be null, not zero: %+v %+v", view.Vulnerabilities, view.ScanCoverage)
	}
	// The other sections keep their full values.
	if view.Health == nil || view.Health.Devices != 3 || view.Detections == nil || view.Detections.Total != 3 {
		t.Errorf("local sections were zeroed: %+v %+v", view.Health, view.Detections)
	}
	if view.Patches == nil || view.Patches.PendingSecurityPatches != 3 || view.Patches.FailedAttempts != 4 {
		t.Errorf("patches section was zeroed: %+v", view.Patches)
	}
	// One hydration failed: inc-b stays unattributed, inc-a still counts.
	if in := view.Incidents; in == nil || in.Opened != 1 || in.Hydrated != 1 || in.HydrationFailed != 1 {
		t.Errorf("incidents = %+v", view.Incidents)
	}
	if !strings.Contains(errOut, "3 report section fetch(es) failed") {
		t.Errorf("stderr warning = %q", errOut)
	}

	// A broken client constructor fails every live section, never the report.
	orig := reportCustomerNewLive
	reportCustomerNewLive = func(*rootFlags) (exposureLiveAPI, error) {
		return exposureLiveAPI{}, fmt.Errorf("config: no credentials")
	}
	t.Cleanup(func() { reportCustomerNewLive = orig })
	view, _ = reportCustomerRun(t, "acme", "--db", db, "--json")
	if view.Health == nil || view.Detections == nil || view.Incidents == nil || view.Vulnerabilities != nil || view.Patches != nil || view.ScanCoverage != nil {
		t.Errorf("no live client: %+v", view)
	}
}

func TestReportCustomerTenantAndErrors(t *testing.T) {
	testenv.Isolate(t)
	now := time.Now()
	db := exposureSeedDB(t, map[string][]map[string]any{
		"device-groups": {{"uuid": "all", "displayName": "All"}},
		"devices":       {{"uuid": "dev-1", "displayName": "FRONTDESK-01", "parentGroupUuid": "all", "functionalityStatus": "DEVICE_FUNCTIONALITY_STATUS_OK", "lastSyncTime": reportCustomerAgo(now, time.Hour)}},
		"incidents":     reportCustomerFixtureIncidents(now),
	})
	fake := reportCustomerFixtureLive(t, now)
	fake.gets = map[string]func() (json.RawMessage, error){} // tenant mode must not hydrate
	fake.lists["/v1/device-patches"] = exposurePage(t, false, patchesBacklogFixturePatches()...)
	exposureStub(t, &reportCustomerNewLive, fake)
	view, _ := reportCustomerRun(t, "tenant", "--db", db, "--json")
	if view.Customer.Name != esetCustomerTenant || len(view.FetchFailures) != 0 {
		t.Fatalf("tenant view = %+v", view)
	}
	if view.Incidents == nil || view.Incidents.Hydrated != 0 || view.Incidents.Opened != 4 || view.Health.Devices != 1 {
		t.Errorf("tenant sections = %+v %+v", view.Incidents, view.Health)
	}
	if _, scoped := fake.params["/v1/device-vulnerabilities"]["deviceGroupUuid"]; scoped {
		t.Errorf("tenant mode is account-wide: %v", fake.params)
	}

	var ce *cliError
	if _, _, err := exposureRunCLI(t, "report", "customer", "Acme Dental", "--db", db, "--json"); !errors.As(err, &ce) || ce.code != 2 {
		t.Errorf("a name on a single-tenant tree should be a usage error, got %v", err)
	}
	msp := reportCustomerSeed(t, now)
	if _, _, err := exposureRunCLI(t, "report", "customer", "Nobody Inc", "--db", msp, "--json"); !errors.As(err, &ce) || ce.code != 3 {
		t.Errorf("unknown customer should be not-found, got %v", err)
	}
	if _, _, err := exposureRunCLI(t, "report", "customer", "--json"); !errors.As(err, &ce) || ce.code != 2 {
		t.Errorf("missing positional should be a usage error, got %v", err)
	}

	// No mirror: the customer cannot be resolved, so the command must fail
	// (exit 2) instead of rendering an empty "successful" report.
	if _, _, err := exposureRunCLI(t, "report", "customer", "Acme Dental", "--db", filepath.Join(t.TempDir(), "absent.db"), "--json"); !errors.As(err, &ce) || ce.code != 2 || !strings.Contains(err.Error(), "no local mirror") {
		t.Errorf("missing mirror should be a usage error naming the sync, got %v", err)
	}
}

func TestReportCustomerDataSource(t *testing.T) {
	testenv.Isolate(t)
	fake := reportCustomerFixtureLive(t, time.Now())
	exposureStub(t, &reportCustomerNewLive, fake)
	missing := filepath.Join(t.TempDir(), "absent.db")
	for _, ds := range []string{"live", "local"} {
		_, errOut, err := exposureRunCLI(t, "report", "customer", "Acme Dental", "--data-source", ds, "--db", missing, "--json")
		var ce *cliError
		if !errors.As(err, &ce) || ce.code != 2 || !strings.Contains(err.Error(), "only --data-source auto") {
			t.Errorf("--data-source %s: want a usage error, got %v", ds, err)
		}
		if strings.Contains(errOut, "no local mirror") || len(fake.params) != 0 {
			t.Errorf("--data-source %s must fail before any IO: stderr=%q params=%v", ds, errOut, fake.params)
		}
	}
	if !strings.Contains(fmt.Sprint(reportCustomerDataSourceErr("live")), "synced mirror") ||
		!strings.Contains(fmt.Sprint(reportCustomerDataSourceErr("local")), "live-only") {
		t.Errorf("each refusal must say why: %v / %v", reportCustomerDataSourceErr("live"), reportCustomerDataSourceErr("local"))
	}
	if reportCustomerDataSourceErr("auto") != nil {
		t.Error("auto must be accepted")
	}
}

// When synced devices cannot be read, the subtree is unknown: every section
// that depends on it is null with the root cause, never a zero.
func TestReportCustomerDevicesUnreadable(t *testing.T) {
	testenv.Isolate(t)
	now := time.Now()
	devices := append(exposureFixtureDevices(now), map[string]any{"uuid": "dev-bad", "displayName": "BROKEN-01", "functionalityProblemCount": "lots"})
	db := exposureSeedDB(t, map[string][]map[string]any{
		"device-groups":  exposureFixtureGroups(),
		"devices":        devices,
		"detections":     reportCustomerFixtureDetections(now),
		"incidents":      reportCustomerFixtureIncidents(now),
		"device-patches": patchesBacklogFixturePatches(),
	})
	fake := reportCustomerFixtureLive(t, now)
	exposureStub(t, &reportCustomerNewLive, fake)

	view, errOut := reportCustomerRun(t, "Acme Dental", "--db", db, "--json")
	if view.Customer.UUID != "acme" {
		t.Errorf("customer still resolves from the groups: %+v", view.Customer)
	}
	if view.Health != nil || view.Detections != nil || view.Incidents != nil || view.Vulnerabilities != nil || view.Patches != nil || view.ScanCoverage != nil {
		t.Fatalf("every subtree-dependent section must be null: %+v", view)
	}
	failed := map[string]string{}
	for _, f := range view.FetchFailures {
		failed[f.Section] = f.Error
	}
	for _, sec := range reportCustomerSections {
		if !strings.Contains(failed[sec], "reading synced devices") {
			t.Errorf("%s: fetch_failures must name the root cause, got %q", sec, failed[sec])
		}
	}
	if len(fake.params) != 0 {
		t.Errorf("no live call should be made for sections that cannot be attributed: %v", fake.params)
	}
	if !strings.Contains(errOut, "6 report section fetch(es) failed") {
		t.Errorf("stderr = %q", errOut)
	}

	// Tenant mode needs no subtree for detections, incidents, vulnerabilities
	// and patches; only the device-counting sections are null.
	tenantDB := exposureSeedDB(t, map[string][]map[string]any{
		"device-groups": {{"uuid": "all", "displayName": "All"}},
		"devices":       {{"uuid": "dev-bad", "functionalityProblemCount": "lots"}},
		"incidents":     reportCustomerFixtureIncidents(now),
	})
	fake.lists["/v1/device-patches"] = exposurePage(t, false, patchesBacklogFixturePatches()...)
	view, _ = reportCustomerRun(t, "tenant", "--db", tenantDB, "--json")
	if view.Health != nil || view.ScanCoverage != nil {
		t.Errorf("tenant: health and scan_coverage need devices: %+v %+v", view.Health, view.ScanCoverage)
	}
	if view.Detections == nil || view.Incidents == nil || view.Vulnerabilities == nil || view.Patches == nil || len(view.FetchFailures) != 2 {
		t.Errorf("tenant: membership-free sections must stand: %+v", view)
	}
}

func TestReportCustomerGroupsUnreadable(t *testing.T) {
	testenv.Isolate(t)
	now := time.Now()
	groups := append(exposureFixtureGroups(), map[string]any{"displayName": "Group with no uuid"})
	db := exposureSeedDB(t, map[string][]map[string]any{
		"device-groups": groups,
		"devices":       exposureFixtureDevices(now),
		"detections":    reportCustomerFixtureDetections(now),
	})
	fake := reportCustomerFixtureLive(t, now)
	exposureStub(t, &reportCustomerNewLive, fake)
	view, _ := reportCustomerRun(t, "Acme Dental", "--db", db, "--json")
	if view.Health != nil || view.Detections != nil || view.Incidents != nil || view.Vulnerabilities != nil || view.Patches != nil || view.ScanCoverage != nil {
		t.Fatalf("no section can be attributed without the group tree: %+v", view)
	}
	if len(view.FetchFailures) != len(reportCustomerSections) || !strings.Contains(view.FetchFailures[0].Error, "reading synced device groups") {
		t.Errorf("fetch failures = %+v", view.FetchFailures)
	}
	if len(fake.params) != 0 {
		t.Errorf("no live call expected: %v", fake.params)
	}
}

// Live rows of a customer nested under the target are left out exactly as
// the synced sections leave out its devices, and malformed rows are counted.
func TestReportCustomerNestedCustomerAndRejects(t *testing.T) {
	testenv.Isolate(t)
	now := time.Now()
	devices := append(exposureFixtureDevices(now), map[string]any{"uuid": "dev-lab", "displayName": "LAB-01", "parentGroupUuid": "acme-lab",
		"functionalityStatus": "DEVICE_FUNCTIONALITY_STATUS_OK", "lastSyncTime": reportCustomerAgo(now, time.Hour)})
	dets := reportCustomerFixtureDetections(now)
	badDet := map[string]any{"uuid": "det-bad", "occurTime": 1700000000, "context": map[string]any{"deviceUuid": "dev-front"}}
	labDet := map[string]any{"uuid": "det-lab", "occurTime": reportCustomerAgo(now, reportCustomerDay), "severityLevel": "SEVERITY_LEVEL_HIGH", "context": map[string]any{"deviceUuid": "dev-lab"}}
	db := exposureSeedDB(t, map[string][]map[string]any{
		"device-groups":  exposureFixtureGroups(),
		"devices":        devices,
		"detections":     append(dets, badDet, labDet),
		"incidents":      reportCustomerFixtureIncidents(now),
		"device-patches": patchesBacklogFixturePatches(),
	})
	fake := reportCustomerFixtureLive(t, now)
	badVuln := exposureVulnJSON("dev-front", "acme-hq", "Google Chrome", "CVE-2024-3400", 80, "SEVERITY_LEVEL_HIGH", "")
	badVuln["applicationVulnerability"].(map[string]any)["cveNumber"] = 3400
	fake.lists["/v1/device-vulnerabilities"] = exposureScopedPage(t, append(patchesBacklogFixtureVulns(),
		exposureVulnJSON("dev-lab", "acme-lab", "Google Chrome", "CVE-2024-9999", 100, "SEVERITY_LEVEL_HIGH", ""), badVuln)...)
	baseScans, _ := fake.lists["/v1/scans/recent"](nil)
	labScan, _ := json.Marshal(map[string]any{"deviceUuid": "dev-lab", "endTime": reportCustomerAgo(now, reportCustomerDay), "endedSuccessfully": false})
	fake.lists["/v1/scans/recent"] = func(map[string]string) (esetPageResult, error) {
		return esetPageResult{Items: append(append([]json.RawMessage{}, baseScans.Items...), labScan), Pages: 1}, nil
	}
	baseProcs, _ := fake.lists["/v1/patching-process-details"](nil)
	labProc, _ := json.Marshal(map[string]any{"deviceUuid": "dev-lab", "application": map[string]any{"displayName": "Google Chrome", "status": "APPLICATION_PATCHING_STATUS_FAILED_DOWNLOAD_ERROR"}})
	fake.lists["/v1/patching-process-details"] = func(map[string]string) (esetPageResult, error) {
		return esetPageResult{Items: append(append([]json.RawMessage{}, baseProcs.Items...), labProc), Pages: 1}, nil
	}
	exposureStub(t, &reportCustomerNewLive, fake)

	view, errOut := reportCustomerRun(t, "Acme Dental", "--db", db, "--since", "90d", "--json")
	if len(view.FetchFailures) != 0 {
		t.Fatalf("no section should fail: %+v", view.FetchFailures)
	}
	if v := view.Vulnerabilities; v == nil || v.VulnerableDevices != 3 || v.Total != 4 || v.TopCVEs[0].CVE != "CVE-2024-3401" {
		t.Errorf("Acme Labs' CVE must not appear in Acme's report: %+v", view.Vulnerabilities)
	}
	if p := view.Patches; p == nil || p.FailedAttempts != 4 || p.FailedDevices != 3 {
		t.Errorf("Acme Labs' failed attempt must be left out: %+v", view.Patches)
	}
	if sc := view.ScanCoverage; sc == nil || sc.FailedScans != 1 || sc.Devices != 3 {
		t.Errorf("Acme Labs' failed scan must be left out: %+v", view.ScanCoverage)
	}
	if d := view.Detections; d == nil || d.Total != 3 {
		t.Errorf("detections = %+v", view.Detections)
	}
	if view.OtherCustomerRows != 3 {
		t.Errorf("other_customer_rows = %d, want 3 (one vulnerability, one patching detail, one scan)", view.OtherCustomerRows)
	}
	if view.RejectedRows != 2 || !strings.Contains(errOut, "warning: 2 row(s) could not be decoded") {
		t.Errorf("rejected_rows = %d stderr = %q", view.RejectedRows, errOut)
	}

	// The nested customer's own report picks those rows up.
	lab, _ := reportCustomerRun(t, "Acme Labs", "--db", db, "--since", "90d", "--json")
	if lab.Vulnerabilities == nil || lab.Vulnerabilities.Total != 1 || lab.ScanCoverage == nil || lab.ScanCoverage.FailedScans != 1 || lab.Detections == nil || lab.Detections.Total != 1 {
		t.Errorf("Acme Labs report = %+v %+v %+v", lab.Vulnerabilities, lab.ScanCoverage, lab.Detections)
	}

	// A source whose every row is malformed failed entirely: null, not zero.
	fake.lists["/v1/scans/recent"] = exposurePage(t, false,
		map[string]any{"deviceUuid": "dev-front", "endedSuccessfully": "yes"},
		map[string]any{"deviceUuid": 7},
	)
	view, _ = reportCustomerRun(t, "Acme Dental", "--db", db, "--json")
	if view.ScanCoverage != nil || len(view.FetchFailures) != 1 || view.FetchFailures[0].Section != "scan_coverage" ||
		!strings.Contains(view.FetchFailures[0].Error, "all 2 scan row(s) could not be decoded") {
		t.Errorf("all-rejected scans: coverage=%+v failures=%+v", view.ScanCoverage, view.FetchFailures)
	}
}

// The human report must say, next to each section, when it is capped or
// partial; otherwise a truncated QBR reads as complete.
func TestReportCustomerHumanLimitations(t *testing.T) {
	testenv.Isolate(t)
	now := time.Now()
	incs := reportCustomerFixtureIncidents(now)
	for i := 0; i < reportCustomerHydrationCap; i++ {
		incs = append(incs, map[string]any{"uuid": fmt.Sprintf("inc-bulk-%02d", i), "status": "INCIDENT_STATUS_OPEN",
			"createTime": reportCustomerAgo(now, time.Duration(i+1)*time.Hour), "updateTime": reportCustomerAgo(now, time.Hour)})
	}
	db := exposureSeedDB(t, map[string][]map[string]any{
		"device-groups":  exposureFixtureGroups(),
		"devices":        exposureFixtureDevices(now),
		"incidents":      incs,
		"device-patches": patchesBacklogFixturePatches(),
	})
	fake := reportCustomerFixtureLive(t, now)
	fake.lists["/v1/device-vulnerabilities"] = exposurePage(t, true, patchesBacklogFixtureVulns()[:2]...)
	fake.lists["/v1/patching-process-details"] = func(map[string]string) (esetPageResult, error) {
		return esetPageResult{}, fmt.Errorf("HTTP 503: unavailable")
	}
	scans := fake.lists["/v1/scans/recent"]
	fake.lists["/v1/scans/recent"] = func(p map[string]string) (esetPageResult, error) {
		r, err := scans(p)
		r.Truncated = true
		return r, err
	}
	exposureStub(t, &reportCustomerNewLive, fake)

	out, _, err := exposureRunCLI(t, "report", "customer", "Acme Dental", "--db", db, "--since", "90d", "--human-friendly")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"not fetched (cap 50), so they are not attributed",
		"note: partial: ",
		"note: the scan stopped at --max-scan-pages; totals may be low",
		"Patches: unavailable: HTTP 503",
		"note: the scan list stopped at --max-scan-pages; coverage may be low",
		"note: ESET keeps only the last few days of vulnerability scans",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("human output missing %q:\n%s", want, out)
		}
	}
	// Each limitation sits under its own section, not in a footer.
	order := []string{"Incidents:", "not fetched (cap 50)", "Vulnerabilities:", "the scan stopped at --max-scan-pages", "Patches: unavailable", "Scan coverage:", "ESET keeps only"}
	last := -1
	for _, w := range order {
		i := strings.Index(out, w)
		if i <= last {
			t.Fatalf("%q is out of place in:\n%s", w, out)
		}
		last = i
	}

	limits := reportCustomerLimits(reportCustomerView{Incidents: &reportCustomerIncidents{HydrationSkipped: 3}})
	if len(limits["incidents"]) != 1 || len(limits["vulnerabilities"]) != 0 {
		t.Errorf("limits = %v", limits)
	}
}

func TestReportCustomerZeroDurationsCountInMeans(t *testing.T) {
	now := time.Now().UTC()
	ts := now.Add(-time.Hour).Format(time.RFC3339)
	incs := []reportCustomerIncident{
		{UUID: "i1", Status: "INCIDENT_STATUS_OPEN", CreateTime: ts, TriageDuration: json.RawMessage(`"0s"`), ResponseDuration: json.RawMessage(`"0s"`)},
		{UUID: "i2", Status: "INCIDENT_STATUS_OPEN", CreateTime: ts, TriageDuration: json.RawMessage(`"60s"`), ResponseDuration: json.RawMessage(`"0s"`)},
	}
	got := reportCustomerIncidentsOf(incs, func(string) esetAttribution { return esetAttribIn }, true, now.Add(-24*time.Hour), now)
	if got.MeanTriageSeconds == nil || *got.MeanTriageSeconds != 30 {
		t.Fatalf("mean triage = %v, want 30 (a 0s triage is a real sample)", got.MeanTriageSeconds)
	}
	if got.MeanResponseSeconds == nil || *got.MeanResponseSeconds != 0 {
		t.Fatalf("all-zero responses must average to 0, not null: %v", got.MeanResponseSeconds)
	}
}
