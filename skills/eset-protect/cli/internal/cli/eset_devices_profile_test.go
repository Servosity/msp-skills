// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored. `devices profile`: argument resolution, the per-section
// shaping, partial-failure accounting and an end-to-end run.
package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const devicesProfileTestDevice = `{"device":{"uuid":"` + reportVulnsTestDev1 + `","displayName":"FRONTDESK-01",
  "systemHostname":"frontdesk-01.acmedental.local","parentGroupUuid":"` + reportVulnsTestHQ + `",
  "functionalityStatus":"DEVICE_FUNCTIONALITY_STATUS_ATTENTION_REQUIRED","functionalityProblemCount":"2","isMuted":false,
  "lastSyncTime":"2026-10-14T08:15:00Z","primaryLocalIpAddress":"10.20.0.15",
  "operatingSystem":{"displayName":"Microsoft Windows 11 Pro","familyId":"1"},
  "deployedComponents":[
    {"name":"ESET_MANAGEMENT_AGENT","displayName":"ESET Management Agent","version":{"name":"11.1.2039.0"}},
    {"name":"ESET_ENDPOINT_SECURITY","version":{"major":"11","minor":"1","patch":"2039"}}]}}`

func devicesProfileTestDetections(n int) []json.RawMessage {
	sev := []string{"SEVERITY_LEVEL_HIGH", "SEVERITY_LEVEL_MEDIUM", "SEVERITY_LEVEL_LOW"}
	out := make([]json.RawMessage, 0, n)
	base := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	for i := 0; i < n; i++ {
		out = append(out, json.RawMessage(fmt.Sprintf(`{"uuid":"det-%02d","displayName":"Win32/Agent.%02d","typeName":"Trojan",
		  "objectName":"C:\\Users\\reception\\Downloads\\invoice%02d.exe","objectHashSha1":"%040d","occurTime":%q,
		  "severityLevel":%q,"context":{"deviceUuid":"%s"},"responses":[{"displayName":"Cleaned by deleting"}]}`,
			i, i, i, i, base.Add(time.Duration(i)*time.Hour).Format(time.RFC3339), sev[i%3], reportVulnsTestDev1)))
	}
	return out
}

var devicesProfileTestPatches = []json.RawMessage{
	json.RawMessage(`{"uuid":"patch-pkg","deviceUuid":"d","packagePatch":{"package":{"name":"openssl"},"resultVersionName":"3.0.2-0ubuntu1.15",
	  "category":"PATCH_CATEGORY_REGULAR_UPDATE","severity":"SEVERITY_LEVEL_HIGH","restartRequired":"RESTART_REQUIREMENT_NO","fixedCves":["CVE-2023-0286"]}}`),
	json.RawMessage(`{"uuid":"patch-app","deviceUuid":"d","applicationPatch":{"catalogId":"5120","developerDisplayName":"Mozilla","resultVersionName":"131.0.3",
	  "category":"PATCH_CATEGORY_SECURITY_UPDATE","severity":"SEVERITY_LEVEL_MEDIUM","restartRequired":"RESTART_REQUIREMENT_NO",
	  "fixedCves":["CVE-2024-3400","CVE-2024-9680","CVE-2024-9392"]}}`),
	json.RawMessage(`{"uuid":"patch-os","deviceUuid":"d","osPatch":{"osFamilyId":"1","developerDisplayName":"Microsoft","resultVersionName":"KB5044284",
	  "category":"PATCH_CATEGORY_SECURITY_UPDATE","severity":"SEVERITY_LEVEL_HIGH","restartRequired":"RESTART_REQUIREMENT_YES","fixedCves":["CVE-2025-21333"]}}`),
	json.RawMessage(`{"uuid":"patch-bare"}`),
}

var devicesProfileTestScans = []json.RawMessage{
	json.RawMessage(`{"deviceUuid":"d","startTime":"2026-10-10T02:00:00Z","endTime":"2026-10-10T02:05:00Z","endedSuccessfully":true,"scanTrigger":"SCAN_TRIGGER_SCHEDULER","vulnerabilityCount":"3"}`),
	json.RawMessage(`{"deviceUuid":"d","startTime":"2026-10-13T02:00:00Z","endTime":"2026-10-13T02:01:00Z","endedSuccessfully":false,"scanTrigger":"SCAN_TRIGGER_USER","vulnerabilityCount":0}`),
}

func TestDevicesProfileResolve(t *testing.T) {
	mirror := []esetDevice{
		{UUID: reportVulnsTestDev1, DisplayName: "FRONTDESK-01", Hostname: "frontdesk-01.acmedental.local"},
		{UUID: reportVulnsTestDev2, DisplayName: "LAB-PC"},
		{UUID: reportVulnsTestDev3, DisplayName: "lab-pc"},
	}
	unsynced := "9f8e7d6c-5b4a-4321-8fed-cba987654321"
	cases := []struct {
		name, q    string
		haveMirror bool
		want       string
		errHas     string
	}{
		{"display name via mirror", "frontdesk-01", true, reportVulnsTestDev1, ""},
		{"hostname via mirror", "FRONTDESK-01.acmedental.local", true, reportVulnsTestDev1, ""},
		{"mirrored uuid", reportVulnsTestDev2, true, reportVulnsTestDev2, ""},
		{"uuid not mirrored yet is used as-is", unsynced, true, unsynced, ""},
		{"unknown name with mirror", "RECEPTION-02", true, "", "no synced device is named"},
		{"ambiguous name is refused", "lab-pc", true, "", "pass the UUID"},
		{"uuid without mirror", unsynced, false, unsynced, ""},
		{"name without mirror", "FRONTDESK-01", false, "", "pass the device UUID"},
		{"blank", "  ", true, "", "empty"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := devicesProfileResolve(tc.q, mirror, tc.haveMirror)
			if tc.errHas != "" {
				if err == nil || !strings.Contains(err.Error(), tc.errHas) {
					t.Fatalf("want error containing %q, got %v (%q)", tc.errHas, err, got)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("got %q, %v; want %q", got, err, tc.want)
			}
		})
	}
}

func TestDevicesProfileDeviceView(t *testing.T) {
	d, err := devicesProfileParseDevice(json.RawMessage(devicesProfileTestDevice))
	if err != nil {
		t.Fatal(err)
	}
	v := devicesProfileDeviceView(d, reportVulnsTestGroups())
	if v.Name != "FRONTDESK-01" || v.FunctionalityStatus != "ATTENTION_REQUIRED" || v.ProblemCount != 2 || v.IP != "10.20.0.15" || v.OS != "Microsoft Windows 11 Pro" {
		t.Fatalf("device view = %+v", v)
	}
	if v.Customer != "Acme Dental" || v.GroupPath != "Servosity MSP / Acme Dental / HQ" {
		t.Fatalf("customer/group_path from tree = %q / %q", v.Customer, v.GroupPath)
	}
	if len(v.Components) != 2 || v.Components[0] != (devicesProfileComponent{"ESET Management Agent", "11.1.2039.0"}) || v.Components[1] != (devicesProfileComponent{"ESET_ENDPOINT_SECURITY", "11.1.2039"}) {
		t.Fatalf("components = %+v", v.Components)
	}
	bare := devicesProfileDeviceView(d, nil)
	if bare.Customer != "" || bare.GroupPath != "" {
		t.Fatalf("without a tree customer/group_path must be empty: %+v", bare)
	}
	if _, err := devicesProfileParseDevice(json.RawMessage(`{"device":{}}`)); err == nil {
		t.Fatal("a device without a uuid must be an error")
	}
}

func TestDevicesProfileDetectionsSummary(t *testing.T) {
	since := time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC)
	s := devicesProfileDetectionsSummary(devicesProfileTestDetections(12), true, since, nil)
	if s.Count != 12 || !s.Truncated || s.Since != "2026-09-14T00:00:00Z" || s.RejectedRows != 0 {
		t.Fatalf("summary = %+v", s)
	}
	if s.BySeverity["HIGH"] != 4 || s.BySeverity["MEDIUM"] != 4 || s.BySeverity["LOW"] != 4 {
		t.Fatalf("by_severity = %v", s.BySeverity)
	}
	if len(s.Latest) != devicesProfileListCap || s.Latest[0].UUID != "det-11" || s.Latest[9].UUID != "det-02" {
		t.Fatalf("latest must be the 10 newest, newest first: first=%s last=%s n=%d", s.Latest[0].UUID, s.Latest[len(s.Latest)-1].UUID, len(s.Latest))
	}
	if s.Latest[0].Severity != "LOW" || s.Latest[1].Severity != "MEDIUM" || s.Latest[0].Type != "Trojan" || len(s.Latest[0].Responses) != 1 {
		t.Fatalf("latest[0] = %+v", s.Latest[0])
	}
	empty := devicesProfileDetectionsSummary(nil, false, time.Time{}, nil)
	b, _ := json.Marshal(empty)
	if !strings.Contains(string(b), `"count":0`) || !strings.Contains(string(b), `"latest":[]`) || !strings.Contains(string(b), `"by_severity":{}`) ||
		!strings.Contains(string(b), `"rejected_rows":0`) {
		t.Fatalf("empty detections JSON = %s", b)
	}
	// A wrongly typed row is counted, not silently dropped from the count.
	var rej esetRejects
	bad := append(devicesProfileTestDetections(2), json.RawMessage(`{"uuid":"det-x","occurTime":["2026-10-01"]}`))
	withBad := devicesProfileDetectionsSummary(bad, false, time.Time{}, &rej)
	if withBad.Count != 2 || withBad.RejectedRows != 1 || rej.N != 1 {
		t.Fatalf("rejected detection = %+v / %+v", withBad, rej)
	}
}

func TestDevicesProfileVulnsSummary(t *testing.T) {
	items := []json.RawMessage{
		json.RawMessage(reportVulnsTestRows[3]), // os, 64, patchable
		json.RawMessage(reportVulnsTestRows[0]), // app, 92, patchable
		json.RawMessage(reportVulnsTestRows[4]), // package, 81, patchable
		json.RawMessage(reportVulnsTestRows[1]), // app, 88, not patchable
	}
	s := devicesProfileVulnsSummary(items, false, "Microsoft Windows 11 Pro", nil)
	if s.Count != 4 || s.MaxRiskScore != 92 || s.Patchable != 3 || s.RejectedRows != 0 {
		t.Fatalf("summary = %+v", s)
	}
	gotOrder := []int64{}
	for _, v := range s.Top {
		gotOrder = append(gotOrder, v.RiskScore)
	}
	if fmt.Sprint(gotOrder) != "[92 88 81 64]" {
		t.Fatalf("top must be by risk score: %v", gotOrder)
	}
	if s.Top[0].Target != "Mozilla Firefox 115.0.2" || s.Top[3].Target != "Microsoft Windows 11 Pro" || s.Top[3].Scope != "os" {
		t.Fatalf("targets = %q / %q", s.Top[0].Target, s.Top[3].Target)
	}
	noOS := devicesProfileVulnsSummary([]json.RawMessage{json.RawMessage(reportVulnsTestRows[3])}, false, "", nil)
	if noOS.Top[0].Target != "OS family 1" {
		t.Fatalf("OS fallback label = %q", noOS.Top[0].Target)
	}
	many := make([]json.RawMessage, 0, 15)
	for i := 0; i < 15; i++ {
		many = append(many, json.RawMessage(fmt.Sprintf(`{"applicationVulnerability":{"cveNumber":"CVE-2024-%04d","riskScore":%d}}`, i, i)))
	}
	capped := devicesProfileVulnsSummary(many, true, "", nil)
	if len(capped.Top) != devicesProfileListCap || capped.Top[0].CVE != "CVE-2024-0014" || !capped.Truncated {
		t.Fatalf("cap/sort = %+v", capped.Top)
	}
	empty := devicesProfileVulnsSummary(nil, false, "", nil)
	if b, _ := json.Marshal(empty); !strings.Contains(string(b), `"top":[]`) || !strings.Contains(string(b), `"rejected_rows":0`) {
		t.Fatalf("empty vulns JSON = %s", b)
	}
	// riskScore "oops" used to read as 0 and stay in the count; now the row is rejected.
	var rej esetRejects
	odd := devicesProfileVulnsSummary([]json.RawMessage{json.RawMessage(reportVulnsTestRows[0]),
		json.RawMessage(`{"deviceUuid":"d","applicationVulnerability":{"cveNumber":"CVE-2024-2","riskScore":"oops"}}`)}, false, "", &rej)
	if odd.Count != 1 || odd.RejectedRows != 1 || rej.N != 1 || odd.MaxRiskScore != 92 {
		t.Fatalf("rejected vuln = %+v / %+v", odd, rej)
	}
}

func TestDevicesProfilePatchesSummary(t *testing.T) {
	s := devicesProfilePatchesSummary(devicesProfileTestPatches, false, nil)
	if s.Count != 4 || s.SecurityCount != 2 || s.RejectedRows != 0 {
		t.Fatalf("counts = %+v", s)
	}
	order := []string{}
	for _, p := range s.Items {
		order = append(order, p.UUID)
	}
	// security first (HIGH os before MEDIUM app), then the HIGH regular package patch, then the bare row.
	if strings.Join(order, ",") != "patch-os,patch-app,patch-pkg,patch-bare" {
		t.Fatalf("order = %v", order)
	}
	osP, app, pkg, bare := s.Items[0], s.Items[1], s.Items[2], s.Items[3]
	if osP.Kind != "os" || osP.RestartRequired != "YES" || osP.Version != "KB5044284" || osP.Category != "SECURITY_UPDATE" {
		t.Fatalf("os patch = %+v", osP)
	}
	if app.Kind != "application" || app.Name != "Mozilla" || app.CatalogID != 5120 || len(app.FixedCVEs) != 3 || app.Severity != "MEDIUM" {
		t.Fatalf("app patch = %+v", app)
	}
	if pkg.Name != "openssl" || pkg.RestartRequired != "NO" {
		t.Fatalf("package patch = %+v", pkg)
	}
	// profile-restart-keeps-unknown: no restartRequired is unknown, not "no".
	if bare.RestartRequired != "UNSPECIFIED" {
		t.Fatalf("a patch with no restart requirement must read UNSPECIFIED, got %q", bare.RestartRequired)
	}
	if bare.Kind != "unknown" || bare.FixedCVEs == nil || bare.Category != "UNSPECIFIED" {
		t.Fatalf("bare patch = %+v", bare)
	}
	if b, _ := json.Marshal(devicesProfilePatchesSummary(nil, false, nil)); !strings.Contains(string(b), `"items":[]`) {
		t.Fatalf("empty patches JSON = %s", b)
	}
	// A non-integer catalogId is a rejected row, not catalog 0.
	var rej esetRejects
	bad := append(append([]json.RawMessage(nil), devicesProfileTestPatches...),
		json.RawMessage(`{"uuid":"patch-typed","applicationPatch":{"catalogId":"5120.5","category":"PATCH_CATEGORY_SECURITY_UPDATE"}}`))
	withBad := devicesProfilePatchesSummary(bad, false, &rej)
	if withBad.Count != 4 || withBad.SecurityCount != 2 || withBad.RejectedRows != 1 || rej.N != 1 {
		t.Fatalf("rejected patch = %+v / %+v", withBad, rej)
	}
}

func TestDevicesProfileScansSummary(t *testing.T) {
	s := devicesProfileScansSummary(devicesProfileTestScans, false, 1, nil)
	if s.Count != 2 || s.Latest == nil || s.LastSuccessful == nil || s.Truncated || s.Note != "" {
		t.Fatalf("scans = %+v", s)
	}
	if s.Latest.StartTime != "2026-10-13T02:00:00Z" || s.Latest.EndedSuccessfully || s.Latest.Trigger != "USER" {
		t.Fatalf("latest = %+v", s.Latest)
	}
	if s.LastSuccessful.VulnerabilityCount != 3 || s.LastSuccessful.Trigger != "SCHEDULER" {
		t.Fatalf("last successful = %+v", s.LastSuccessful)
	}
	empty := devicesProfileScansSummary(nil, false, 1, nil)
	if b, _ := json.Marshal(empty); string(b) != `{"count":0,"latest":null,"last_successful":null,"truncated":false,"rejected_rows":0}` {
		t.Fatalf("empty scans JSON = %s", b)
	}
}

// TestDevicesProfileScansSummaryTruncated: ESET does not order recent scans,
// so when the page cap is hit a newer or successful scan may be unread. The
// summary must say so (truncated + note) and withhold latest/last_successful
// instead of presenting a possibly wrong scan as the latest.
func TestDevicesProfileScansSummaryTruncated(t *testing.T) {
	// Only the failed scan was on the pages read; the successful one is "on page two".
	s := devicesProfileScansSummary(devicesProfileTestScans[1:], true, 5, nil)
	if !s.Truncated || s.Latest != nil || s.LastSuccessful != nil || s.Count != 1 {
		t.Fatalf("truncated scans = %+v", s)
	}
	if !strings.Contains(s.Note, "only the first 5 page(s) of scans were read") {
		t.Fatalf("note = %q", s.Note)
	}
	b, _ := json.Marshal(s)
	if !strings.Contains(string(b), `"truncated":true`) || !strings.Contains(string(b), `"note":"only the first 5 page(s)`) {
		t.Fatalf("truncated scans JSON = %s", b)
	}
	var rej esetRejects
	bad := devicesProfileScansSummary([]json.RawMessage{devicesProfileTestScans[0], json.RawMessage(`{"vulnerabilityCount":"2.5"}`)}, false, 1, &rej)
	if bad.Count != 1 || bad.RejectedRows != 1 || rej.N != 1 || bad.LastSuccessful == nil {
		t.Fatalf("rejected scan = %+v / %+v", bad, rej)
	}
	// devicesProfileBuild carries the page walk's truncated flag through.
	v, err := devicesProfileBuild(devicesProfileParts{device: json.RawMessage(devicesProfileTestDevice),
		scans: esetPageResult{Items: devicesProfileTestScans, Pages: 5, Truncated: true}}, nil, time.Time{}, nil)
	if err != nil || v.Scans == nil || !v.Scans.Truncated || v.Scans.Latest != nil {
		t.Fatalf("build scans = %+v, %v", v.Scans, err)
	}
}

// TestDevicesProfileHumanShowsPartials: the human output must render every
// section's truncation flag and rejected rows, not only the JSON.
func TestDevicesProfileHumanShowsPartials(t *testing.T) {
	var rej esetRejects
	v, err := devicesProfileBuild(devicesProfileParts{
		device:     json.RawMessage(devicesProfileTestDevice),
		detections: esetPageResult{Items: devicesProfileTestDetections(2), Pages: 2, Truncated: true},
		vulns:      esetPageResult{Items: []json.RawMessage{json.RawMessage(reportVulnsTestRows[0]), json.RawMessage(`{"osVulnerability":{"riskScore":"high"}}`)}, Pages: 3, Truncated: true},
		patches:    esetPageResult{Items: devicesProfileTestPatches, Pages: 3, Truncated: true},
		scans:      esetPageResult{Items: devicesProfileTestScans, Pages: 5, Truncated: true},
	}, nil, time.Time{}, &rej)
	if err != nil {
		t.Fatal(err)
	}
	if v.RejectedRows != 1 || rej.N != 1 || v.Vulnerabilities.RejectedRows != 1 {
		t.Fatalf("rejected rows = %d / %+v", v.RejectedRows, rej)
	}
	var buf bytes.Buffer
	if err := devicesProfileHuman(&buf, v, "30d"); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{
		"Detections (last 30d): 2 [partial: stopped at the page cap]",
		"Vulnerabilities: 1 (max risk 92, 1 patchable) [partial: stopped at the page cap; 1 row(s) could not be decoded]",
		"Pending patches: 4 (2 security) [partial: stopped at the page cap]",
		"Vulnerability scans: 2 read [partial: stopped at the page cap]; latest and last successful unknown",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("human output missing %q:\n%s", want, out)
		}
	}
	clean, _ := devicesProfileBuild(devicesProfileParts{device: json.RawMessage(devicesProfileTestDevice),
		scans: esetPageResult{Items: devicesProfileTestScans, Pages: 1}}, nil, time.Time{}, nil)
	buf.Reset()
	_ = devicesProfileHuman(&buf, clean, "30d")
	if strings.Contains(buf.String(), "partial") || !strings.Contains(buf.String(), "Vulnerability scans: 2 recent; latest ended 2026-10-13T02:01:00Z") {
		t.Fatalf("a complete profile must carry no partial markers:\n%s", buf.String())
	}
}

func TestDevicesProfileBuildPartialFailure(t *testing.T) {
	parts := devicesProfileParts{
		device:     json.RawMessage(devicesProfileTestDevice),
		detections: esetPageResult{Items: devicesProfileTestDetections(2), Pages: 1},
		vulnErr:    errors.New("GET /v1/device-vulnerabilities returned HTTP 403"),
		patches:    esetPageResult{Items: devicesProfileTestPatches, Pages: 1},
		scanErr:    errors.New("context deadline exceeded"),
	}
	v, err := devicesProfileBuild(parts, nil, time.Time{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if v.Vulnerabilities != nil || v.Scans != nil || v.Detections == nil || v.Patches == nil {
		t.Fatalf("failed parts must be nil and the rest present: %+v", v)
	}
	if len(v.FetchFailures) != 2 || v.FetchFailures[0].Part != "vulnerabilities" || v.FetchFailures[1].Part != "scans" {
		t.Fatalf("fetch_failures = %+v", v.FetchFailures)
	}
	b, _ := json.Marshal(v)
	if !strings.Contains(string(b), `"vulnerabilities":null`) || strings.Contains(string(b), `"vulnerabilities":{"count":0`) {
		t.Fatalf("a failed part must be null, never zero-filled: %s", b)
	}

	ok, _ := devicesProfileBuild(devicesProfileParts{device: json.RawMessage(devicesProfileTestDevice)}, nil, time.Time{}, nil)
	if ok.FetchFailures == nil || len(ok.FetchFailures) != 0 {
		t.Fatalf("no failures must render as []: %#v", ok.FetchFailures)
	}
	if _, err := devicesProfileBuild(devicesProfileParts{deviceErr: errors.New("HTTP 404")}, nil, time.Time{}, nil); err == nil {
		t.Fatal("the device record is required")
	}
}

func devicesProfileTestHandler(t *testing.T) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		switch r.URL.Path {
		case "/v1/devices/" + reportVulnsTestDev1:
			_, _ = w.Write([]byte(devicesProfileTestDevice))
		case "/v1/detections":
			if q.Get("deviceUuid") == "" || q.Get("startTime") == "" {
				t.Errorf("detections query = %v", q)
			}
			b, _ := json.Marshal(map[string]any{"detections": devicesProfileTestDetections(3), "nextPageToken": ""})
			_, _ = w.Write(b)
		case "/v1/device-vulnerabilities":
			if q.Get("deviceUuid") == "" {
				t.Errorf("vulnerabilities query = %v", q)
			}
			_, _ = w.Write([]byte(`{"vulnerabilities":[` + reportVulnsTestRows[0] + `,` + reportVulnsTestRows[3] +
				`,{"deviceUuid":"d","packageVulnerability":{"cveNumber":"CVE-2024-2","riskScore":"oops"}}]}`))
		case "/v1/device-patches":
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"message":"patch management is not licensed"}`))
		case "/v1/scans/recent":
			b, _ := json.Marshal(map[string]any{"scanDetails": devicesProfileTestScans})
			_, _ = w.Write(b)
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"message":"not found"}`))
		}
	}
}

func TestDevicesProfileCommandEndToEnd(t *testing.T) {
	srv := reportVulnsNewServer(t, devicesProfileTestHandler(t))
	mirrored := `{"uuid":"` + reportVulnsTestDev1 + `","displayName":"FRONTDESK-01","systemHostname":"frontdesk-01.acmedental.local","parentGroupUuid":"` + reportVulnsTestHQ + `"}`
	dbPath := reportVulnsSeedDB(t, reportVulnsTestGroups(), []string{mirrored})
	out, stderr, err := reportVulnsRunCLI(t, srv, "devices", "profile", "FRONTDESK-01", "--since", "7d", "--db", dbPath, "--json")
	if err != nil {
		t.Fatalf("run: %v\nstderr: %s", err, stderr)
	}
	var v devicesProfileView
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatalf("bad JSON: %v\n%s", err, out)
	}
	if v.Device.UUID != reportVulnsTestDev1 || v.Device.Customer != "Acme Dental" || v.Device.GroupPath != "Servosity MSP / Acme Dental / HQ" {
		t.Fatalf("device = %+v", v.Device)
	}
	if v.Detections == nil || v.Detections.Count != 3 || v.Vulnerabilities == nil || v.Vulnerabilities.MaxRiskScore != 92 ||
		v.Vulnerabilities.Top[1].Target != "Microsoft Windows 11 Pro" || v.Scans == nil || v.Scans.Count != 2 {
		t.Fatalf("sections = %s", out)
	}
	if v.Patches != nil || len(v.FetchFailures) != 1 || v.FetchFailures[0].Part != "patches" || !strings.Contains(v.FetchFailures[0].Error, "403") {
		t.Fatalf("the 403 on patches must be a fetch failure with a null section: %s", out)
	}
	if !strings.Contains(stderr, "warning: 1 of 4 profile parts failed (patches)") {
		t.Fatalf("stderr warning missing: %q", stderr)
	}
	// The riskScore "oops" row is rejected: counted, warned once, kept out of the totals.
	if v.RejectedRows != 1 || v.Vulnerabilities.RejectedRows != 1 || v.Vulnerabilities.Count != 2 ||
		strings.Count(stderr, "could not be decoded") != 1 || !strings.Contains(stderr, "vulnerability row 2") {
		t.Fatalf("rejected rows = %d (vulns %d), stderr = %q", v.RejectedRows, v.Vulnerabilities.RejectedRows, stderr)
	}
	if v.Scans.Truncated || v.Scans.Latest == nil {
		t.Fatalf("one complete page of scans is not truncated: %+v", v.Scans)
	}
	// The detection window is ~7 days back.
	for _, u := range srv.seen() {
		if u.Path == "/v1/detections" {
			st, err := time.Parse(time.RFC3339, u.Query().Get("startTime"))
			if err != nil || time.Since(st) < 6*24*time.Hour || time.Since(st) > 8*24*time.Hour {
				t.Fatalf("startTime = %q", u.Query().Get("startTime"))
			}
		}
	}

	// The --select example from the help resolves.
	sel, _, err := reportVulnsRunCLI(t, srv, "devices", "profile", reportVulnsTestDev1, "--db", dbPath, "--select", "device,vulnerabilities.top")
	if err != nil || !strings.Contains(sel, "FRONTDESK-01") || !strings.Contains(sel, "CVE-2024-3400") || strings.Contains(sel, "scanDetails") {
		t.Fatalf("--select output: %v\n%s", err, sel)
	}
}

// TestDevicesProfileRejectsNullScalarAndVariantlessRows: a null row decodes
// "successfully" into an empty struct, so "vulnerabilities":[null] used to
// show a fake risk-0 vulnerability with rejected_rows 0. Every section must
// count null and scalar rows (and variant-less vulnerability rows) as rejected.
func TestDevicesProfileRejectsNullScalarAndVariantlessRows(t *testing.T) {
	variantless := `{"deviceUuid":"` + reportVulnsTestDev1 + `","deviceGroupUuid":"` + reportVulnsTestHQ + `","availablePatchUuids":["p-1"]}`
	inner := devicesProfileTestHandler(t)
	srv := reportVulnsNewServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/device-vulnerabilities" {
			_, _ = w.Write([]byte(`{"vulnerabilities":[` + reportVulnsTestRows[0] + `,null,7,` + variantless + `]}`))
			return
		}
		inner(w, r)
	})
	mirrored := `{"uuid":"` + reportVulnsTestDev1 + `","displayName":"FRONTDESK-01","parentGroupUuid":"` + reportVulnsTestHQ + `"}`
	dbPath := reportVulnsSeedDB(t, reportVulnsTestGroups(), []string{mirrored})
	out, stderr, err := reportVulnsRunCLI(t, srv, "devices", "profile", "FRONTDESK-01", "--db", dbPath, "--json")
	if err != nil {
		t.Fatalf("run: %v\nstderr: %s", err, stderr)
	}
	var v devicesProfileView
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatalf("bad JSON: %v\n%s", err, out)
	}
	if v.Vulnerabilities == nil || v.Vulnerabilities.Count != 1 || v.Vulnerabilities.RejectedRows != 3 || v.RejectedRows != 3 ||
		len(v.Vulnerabilities.Top) != 1 || v.Vulnerabilities.Top[0].CVE != "CVE-2024-3400" || v.Vulnerabilities.Patchable != 1 {
		t.Fatalf("vulnerabilities = %+v (rejected %d)", v.Vulnerabilities, v.RejectedRows)
	}
	if !strings.Contains(stderr, "warning: 3 row(s) could not be decoded") || !strings.Contains(stderr, "vulnerability row 1: not a JSON object") {
		t.Fatalf("stderr = %q", stderr)
	}

	// The other sections reject null and scalar rows the same way.
	var rej esetRejects
	bad := []json.RawMessage{json.RawMessage(`null`), json.RawMessage(`"x"`)}
	pv, err := devicesProfileBuild(devicesProfileParts{
		device:     json.RawMessage(devicesProfileTestDevice),
		detections: esetPageResult{Items: append(devicesProfileTestDetections(1), bad...), Pages: 1},
		vulns:      esetPageResult{Items: []json.RawMessage{json.RawMessage(`null`)}, Pages: 1},
		patches:    esetPageResult{Items: append([]json.RawMessage{devicesProfileTestPatches[0]}, bad...), Pages: 1},
		scans:      esetPageResult{Items: append([]json.RawMessage{devicesProfileTestScans[0]}, bad...), Pages: 1},
	}, nil, time.Time{}, &rej)
	if err != nil {
		t.Fatal(err)
	}
	if pv.Detections.Count != 1 || pv.Detections.RejectedRows != 2 || pv.Vulnerabilities.Count != 0 || pv.Vulnerabilities.RejectedRows != 1 ||
		pv.Patches.Count != 1 || pv.Patches.RejectedRows != 2 || pv.Scans.Count != 1 || pv.Scans.RejectedRows != 2 || pv.RejectedRows != 7 || rej.N != 7 {
		t.Fatalf("per-section rejects: det %+v vulns %+v patches %+v scans %+v total %d", pv.Detections, pv.Vulnerabilities, pv.Patches, pv.Scans, pv.RejectedRows)
	}
	// A device response whose device is null is not a device.
	for _, raw := range []string{`null`, `{"device":null}`, `7`} {
		if _, err := devicesProfileParseDevice(json.RawMessage(raw)); err == nil {
			t.Errorf("device %s must be rejected", raw)
		}
	}
}

func TestDevicesProfileCommandRequiredInputs(t *testing.T) {
	srv := reportVulnsNewServer(t, devicesProfileTestHandler(t))
	missing := filepath.Join(t.TempDir(), "absent.db")

	// A name needs the mirror: usage error before any request.
	_, stderr, err := reportVulnsRunCLI(t, srv, "devices", "profile", "FRONTDESK-01", "--db", missing, "--json")
	if err == nil || ExitCode(err) != 2 || len(srv.seen()) != 0 {
		t.Fatalf("want usage error and no requests, got %v (%d requests)\n%s", err, len(srv.seen()), stderr)
	}

	// An unknown UUID: the required device fetch 404s, so the command fails (not-found exit).
	other := "9f8e7d6c-5b4a-4321-8fed-cba987654321"
	_, _, err = reportVulnsRunCLI(t, srv, "devices", "profile", other, "--db", missing, "--json")
	if err == nil || ExitCode(err) != 3 {
		t.Fatalf("missing device should exit 3, got %v (exit %d)", err, ExitCode(err))
	}

	// A UUID works without any mirror; customer/group_path stay empty.
	out, _, err := reportVulnsRunCLI(t, srv, "devices", "profile", reportVulnsTestDev1, "--db", missing, "--json")
	if err != nil {
		t.Fatal(err)
	}
	var v devicesProfileView
	if err := json.Unmarshal([]byte(out), &v); err != nil || v.Device.Customer != "" || v.Device.GroupPath != "" || v.Device.Name != "FRONTDESK-01" {
		t.Fatalf("uuid without mirror: %v\n%s", err, out)
	}

	dry, _, err := reportVulnsRunCLI(t, nil, "devices", "profile", "FRONTDESK-01", "--dry-run", "--json")
	if err != nil || !strings.Contains(dry, `"dry_run":true`) {
		t.Fatalf("dry-run: %v %s", err, dry)
	}
	help, _, err := reportVulnsRunCLI(t, nil, "devices", "profile")
	if err != nil || !strings.Contains(help, "Do NOT use it for a whole customer; use 'report customer' instead") {
		t.Fatalf("bare invocation should print help: %v\n%s", err, help)
	}
}
