// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored. `incidents context`: the v1 -> v2 detection fallback,
// fan-out planning, partial-failure accounting and an end-to-end run.
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"eset-protect-pp-cli/internal/client"
	"eset-protect-pp-cli/internal/cliutil"
)

const (
	incidentsContextTestUUID    = "8d4f2a17-3c6e-4b9a-a1d2-5e7f9c0b3a64"
	incidentsContextTestDetV1   = "d7c1e0f2-1111-4a3b-9c8d-7e6f5a4b3c21"
	incidentsContextTestDetV2   = "d7c1e0f2-2222-4a3b-9c8d-7e6f5a4b3c22"
	incidentsContextTestDetGone = "d7c1e0f2-3333-4a3b-9c8d-7e6f5a4b3c23"
	incidentsContextTestDevDeny = "a1b2c3d4-0404-4e5f-8a9b-0c1d2e3f4a99"
)

var incidentsContextTestIncident = `{"incident":{"uuid":"` + incidentsContextTestUUID + `","displayName":"Ransomware behavior on FRONTDESK-01",
  "description":"Mass file renames followed by a ransom note","status":"INCIDENT_STATUS_IN_PROGRESS","severity":"INCIDENT_SEVERITY_LEVEL_HIGH",
  "assigneeUuid":"5a6b7c8d-9e0f-4a1b-8c2d-3e4f5a6b7c8d","createTime":"2026-10-13T07:55:00Z","updateTime":"2026-10-13T09:10:00Z",
  "triageDuration":"1800s","responseDuration":"5400s","tags":["ransomware","frontdesk"],
  "detectionUuids":["` + incidentsContextTestDetV1 + `","` + incidentsContextTestDetV2 + `","` + incidentsContextTestDetGone + `","` + incidentsContextTestDetV1 + `",""],
  "deviceUuids":["` + reportVulnsTestDev1 + `","` + reportVulnsTestDev3 + `","` + incidentsContextTestDevDeny + `"]}}`

var incidentsContextTestDetectionV1 = `{"detection":{"uuid":"` + incidentsContextTestDetV1 + `","displayName":"Win32/Filecoder.LockBit.A",
  "objectName":"C:\\Users\\reception\\AppData\\Local\\Temp\\lb3.exe","objectHashSha1":"3d5c8f9a0b1e2d4c6f7a8b9c0d1e2f3a4b5c6d7e",
  "occurTime":"2026-10-13T07:52:00Z","severityLevel":"SEVERITY_LEVEL_HIGH","context":{"deviceUuid":"` + reportVulnsTestDev1 + `"}}}`

var incidentsContextTestDetectionV2 = `{"detection":{"uuid":"` + incidentsContextTestDetV2 + `","displayName":"Suspicious mass rename (EDR rule)",
  "objectName":"C:\\Windows\\System32\\cmd.exe","objectHashSha1":"","occurTime":"2026-10-13T07:50:00Z",
  "severityLevel":"SEVERITY_LEVEL_MEDIUM","severityScore":"60","resolved":false,"device":{"uuid":"` + reportVulnsTestDev3 + `","displayName":"PLUMB-SRV01"}}}`

func incidentsContextTestAPIErr(status int) error {
	return &client.APIError{Method: "GET", Path: "/x", StatusCode: status}
}

// incidentsContextFakeGetter answers by URL suffix and counts calls.
type incidentsContextFakeGetter struct {
	mu        sync.Mutex
	responses map[string]string
	errs      map[string]error
	calls     []string
}

func (f *incidentsContextFakeGetter) get(_ context.Context, url string) (json.RawMessage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, url)
	for suffix, err := range f.errs {
		if strings.HasSuffix(url, suffix) {
			return nil, err
		}
	}
	for suffix, body := range f.responses {
		if strings.HasSuffix(url, suffix) {
			return json.RawMessage(body), nil
		}
	}
	return nil, incidentsContextTestAPIErr(http.StatusNotFound)
}

func TestIncidentsContextFetchDetectionFallback(t *testing.T) {
	cases := []struct {
		name      string
		responses map[string]string
		errs      map[string]error
		wantVer   string
		wantErr   string
		wantCalls int
	}{
		{"v1 answers", map[string]string{"/v1/detections/x": `{"detection":{}}`}, nil, "v1", "", 1},
		{"v1 404 falls back to v2", map[string]string{"/v2/detections/x": `{"detection":{}}`}, nil, "v2", "", 2},
		{"wrapped 404 still falls back", map[string]string{"/v2/detections/x": `{}`},
			map[string]error{"/v1/detections/x": fmt.Errorf("fetch: %w", incidentsContextTestAPIErr(404))}, "v2", "", 2},
		{"v1 403 does not fall back", map[string]string{"/v2/detections/x": `{}`},
			map[string]error{"/v1/detections/x": incidentsContextTestAPIErr(403)}, "", "HTTP 403", 1},
		{"a non-API error does not fall back", nil, map[string]error{"/v1/detections/x": errors.New("dial tcp: connection refused")}, "", "connection refused", 1},
		{"both versions 404", nil, nil, "", "not found on v1 detections", 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := &incidentsContextFakeGetter{responses: tc.responses, errs: tc.errs}
			_, ver, err := incidentsContextFetchDetection(context.Background(), f.get, "x")
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("want error %q, got %v", tc.wantErr, err)
				}
			} else if err != nil || ver != tc.wantVer {
				t.Fatalf("got %q, %v; want %q", ver, err, tc.wantVer)
			}
			if len(f.calls) != tc.wantCalls {
				t.Fatalf("calls = %v", f.calls)
			}
			if !strings.HasPrefix(f.calls[0], "https://{region}.incident-management.eset.systems/v1/detections/") {
				t.Fatalf("v1 detection host/path wrong: %s", f.calls[0])
			}
		})
	}
}

func TestIncidentsContextPlan(t *testing.T) {
	inc, err := incidentsContextParseIncident(json.RawMessage(incidentsContextTestIncident))
	if err != nil {
		t.Fatal(err)
	}
	jobs, tr := incidentsContextPlan(inc, 25, 25)
	if tr.Detections || tr.Devices {
		t.Fatalf("nothing should be truncated: %+v", tr)
	}
	names := make([]string, 0, len(jobs))
	for _, j := range jobs {
		names = append(names, j.name())
	}
	want := "comments,detection:" + incidentsContextTestDetV1 + ",detection:" + incidentsContextTestDetV2 + ",detection:" + incidentsContextTestDetGone +
		",device:" + reportVulnsTestDev1 + ",device:" + reportVulnsTestDev3 + ",device:" + incidentsContextTestDevDeny
	if strings.Join(names, ",") != want {
		t.Fatalf("plan (deduped, blanks dropped) =\n%s\nwant\n%s", strings.Join(names, ","), want)
	}
	capped, tr := incidentsContextPlan(inc, 1, 0)
	if !tr.Detections || !tr.Devices || len(capped) != 2 || capped[1].ID != incidentsContextTestDetV1 {
		t.Fatalf("caps: %+v %+v", capped, tr)
	}
	if _, err := incidentsContextParseIncident(json.RawMessage(`{"incident":{}}`)); err == nil {
		t.Fatal("an incident without a uuid must be an error")
	}
}

func TestIncidentsContextBuild(t *testing.T) {
	inc, _ := incidentsContextParseIncident(json.RawMessage(incidentsContextTestIncident))
	results := []cliutil.FanoutResult[incidentsContextFetched]{
		{Source: "detection:" + incidentsContextTestDetV1, Value: incidentsContextFetched{Job: incidentsContextJob{"detection", incidentsContextTestDetV1}, Raw: json.RawMessage(incidentsContextTestDetectionV1), APIVersion: "v1"}},
		{Source: "detection:" + incidentsContextTestDetV2, Value: incidentsContextFetched{Job: incidentsContextJob{"detection", incidentsContextTestDetV2}, Raw: json.RawMessage(incidentsContextTestDetectionV2), APIVersion: "v2"}},
		{Source: "device:" + reportVulnsTestDev1, Value: incidentsContextFetched{Job: incidentsContextJob{"device", reportVulnsTestDev1}, Raw: json.RawMessage(devicesProfileTestDevice)}},
		{Source: "device:bad", Value: incidentsContextFetched{Job: incidentsContextJob{"device", "bad"}, Raw: json.RawMessage(`{"device":{}}`)}},
	}
	errs := []cliutil.FanoutError{
		{Source: "comments", Err: incidentsContextTestAPIErr(403)},
		{Source: "detection:" + incidentsContextTestDetGone, Err: errors.New("not found on v1 detections; v2 lookup failed: HTTP 404")},
	}
	var rej esetRejects
	v := incidentsContextBuild(inc, incidentsContextTruncated{Devices: true}, results, errs, reportVulnsTestGroups(), &rej)
	if v.Incident.Status != "IN_PROGRESS" || v.Incident.Severity != "HIGH" || v.Incident.TriageDuration != "1800s" ||
		v.Incident.DetectionCount != 3 || v.Incident.DeviceCount != 3 || len(v.Incident.Tags) != 2 {
		t.Fatalf("incident = %+v", v.Incident)
	}
	if v.Comments != nil {
		t.Fatalf("a failed comments fetch must be null, got %+v", v.Comments)
	}
	if len(v.Detections) != 2 || v.Detections[0].UUID != incidentsContextTestDetV2 || v.Detections[0].APIVersion != "v2" ||
		v.Detections[0].DeviceUUID != reportVulnsTestDev3 || v.Detections[1].DeviceUUID != reportVulnsTestDev1 || v.Detections[1].Severity != "HIGH" {
		t.Fatalf("detections (oldest first, v1 context / v2 device uuid) = %+v", v.Detections)
	}
	if len(v.Devices) != 1 || v.Devices[0].Customer != "Acme Dental" || v.Devices[0].FunctionalityStatus != "ATTENTION_REQUIRED" {
		t.Fatalf("devices = %+v", v.Devices)
	}
	// Request failures are fetch_failures; the device that came back without a
	// uuid was fetched fine but could not be decoded, so it is a rejected row.
	if len(v.FetchFailures) != 2 || v.FetchFailures[0].Part != "comments" || v.FetchFailures[1].Part != "detection:"+incidentsContextTestDetGone {
		t.Fatalf("fetch_failures = %+v", v.FetchFailures)
	}
	if v.RejectedRows != 1 || rej.N != 1 || !strings.HasPrefix(rej.First, "device row bad:") {
		t.Fatalf("rejected = %d / %+v", v.RejectedRows, rej)
	}
	if !v.Truncated.Devices || v.Truncated.Detections {
		t.Fatalf("truncated = %+v", v.Truncated)
	}
	if got := incidentsContextFailureSummary(v.FetchFailures); got != "comments, 1 detection(s)" {
		t.Fatalf("summary = %q", got)
	}

	empty := incidentsContextBuild(incidentsContextWire{UUID: "i"}, incidentsContextTruncated{},
		[]cliutil.FanoutResult[incidentsContextFetched]{{Source: "comments", Value: incidentsContextFetched{Job: incidentsContextJob{Kind: "comments"}, Raw: json.RawMessage(`{"comments":[]}`)}}}, nil, nil, nil)
	b, _ := json.Marshal(empty)
	for _, want := range []string{`"comments":[]`, `"detections":[]`, `"devices":[]`, `"fetch_failures":[]`, `"tags":[]`, `"rejected_rows":0`} {
		if !strings.Contains(string(b), want) {
			t.Fatalf("empty context JSON lacks %s: %s", want, b)
		}
	}
}

// TestIncidentsContextRejectsMalformedRows: one malformed comment used to
// null the whole comments list (or, row by row, vanish); now the good
// comments stay and the bad one is counted. A detection payload with a wrongly
// typed field and an undecodable comments payload are counted the same way.
func TestIncidentsContextRejectsMalformedRows(t *testing.T) {
	var rej esetRejects
	comments, err := incidentsContextParseComments(json.RawMessage(`{"comments":[
	  {"uuid":"c1","text":"Triage started","createTime":"2026-10-13T08:00:00Z"},
	  {"uuid":"c2","text":"bad flag","isFinalComment":"yes"},
	  {"uuid":"c3","text":"Isolated FRONTDESK-01","createTime":"2026-10-13T08:20:00Z"}]}`), &rej)
	if err != nil || len(comments) != 2 || comments[1].UUID != "c3" || rej.N != 1 || !strings.HasPrefix(rej.First, "comment row 1:") {
		t.Fatalf("comments = %+v, %v, rejects %+v", comments, err, rej)
	}
	if _, err := incidentsContextParseComments(json.RawMessage(`{"comments":"none"}`), nil); err == nil {
		t.Fatal("a comments payload that is not a list must fail")
	}

	inc, _ := incidentsContextParseIncident(json.RawMessage(incidentsContextTestIncident))
	results := []cliutil.FanoutResult[incidentsContextFetched]{
		{Source: "comments", Value: incidentsContextFetched{Job: incidentsContextJob{Kind: "comments"}, Raw: json.RawMessage(`{"comments":{"uuid":"c1"}}`)}},
		{Source: "detection:" + incidentsContextTestDetV1, Value: incidentsContextFetched{Job: incidentsContextJob{"detection", incidentsContextTestDetV1},
			Raw: json.RawMessage(`{"detection":{"uuid":"` + incidentsContextTestDetV1 + `","severityLevel":5}}`), APIVersion: "v1"}},
		{Source: "detection:" + incidentsContextTestDetV2, Value: incidentsContextFetched{Job: incidentsContextJob{"detection", incidentsContextTestDetV2}, Raw: json.RawMessage(incidentsContextTestDetectionV2), APIVersion: "v2"}},
	}
	rej = esetRejects{}
	v := incidentsContextBuild(inc, incidentsContextTruncated{}, results, nil, nil, &rej)
	if v.RejectedRows != 2 || rej.N != 2 || v.Comments != nil || len(v.Detections) != 1 || len(v.FetchFailures) != 0 {
		t.Fatalf("view = %+v / rejects %+v", v, rej)
	}
}

func incidentsContextTestHandler(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/v2/incidents/" + incidentsContextTestUUID:
		_, _ = w.Write([]byte(incidentsContextTestIncident))
	case "/v2/incidents/" + incidentsContextTestUUID + "/comments":
		_, _ = w.Write([]byte(`{"comments":[
		  {"uuid":"c2","authorUuid":"u1","text":"Isolated FRONTDESK-01 from the network","createTime":"2026-10-13T08:20:00Z"},
		  {"uuid":"c1","authorUuid":"u1","text":"Triage started","createTime":"2026-10-13T08:00:00Z"}]}`))
	case "/v1/detections/" + incidentsContextTestDetV1:
		_, _ = w.Write([]byte(incidentsContextTestDetectionV1))
	case "/v2/detections/" + incidentsContextTestDetV2:
		_, _ = w.Write([]byte(incidentsContextTestDetectionV2))
	case "/v1/devices/" + reportVulnsTestDev1:
		_, _ = w.Write([]byte(devicesProfileTestDevice))
	case "/v1/devices/" + reportVulnsTestDev3:
		_, _ = w.Write([]byte(`{"device":{"uuid":"` + reportVulnsTestDev3 + `","displayName":"PLUMB-SRV01","parentGroupUuid":"` + reportVulnsTestPlumb + `",
		  "functionalityStatus":"DEVICE_FUNCTIONALITY_STATUS_OK","lastSyncTime":"2026-10-14T06:00:00Z"}}`))
	case "/v1/devices/" + incidentsContextTestDevDeny:
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"access denied"}`))
	default:
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"not found"}`))
	}
}

func TestIncidentsContextCommandEndToEnd(t *testing.T) {
	srv := reportVulnsNewServer(t, incidentsContextTestHandler)
	dbPath := reportVulnsSeedDB(t, reportVulnsTestGroups(), nil)
	out, stderr, err := reportVulnsRunCLI(t, srv, "incidents", "context", incidentsContextTestUUID, "--db", dbPath, "--json")
	if err != nil {
		t.Fatalf("run: %v\nstderr: %s", err, stderr)
	}
	var v incidentsContextView
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatalf("bad JSON: %v\n%s", err, out)
	}
	if v.Incident.Name != "Ransomware behavior on FRONTDESK-01" || v.Incident.Status != "IN_PROGRESS" {
		t.Fatalf("incident = %+v", v.Incident)
	}
	if len(v.Comments) != 2 || v.Comments[0].Text != "Triage started" {
		t.Fatalf("comments (oldest first) = %+v", v.Comments)
	}
	if len(v.Detections) != 2 || v.Detections[0].APIVersion != "v2" || v.Detections[1].APIVersion != "v1" {
		t.Fatalf("detections = %+v", v.Detections)
	}
	if len(v.Devices) != 2 || v.Devices[0].Name != "FRONTDESK-01" || v.Devices[0].Customer != "Acme Dental" ||
		v.Devices[1].Name != "PLUMB-SRV01" || v.Devices[1].Customer != "Acme Plumbing" {
		t.Fatalf("devices = %+v", v.Devices)
	}
	parts := []string{}
	for _, f := range v.FetchFailures {
		parts = append(parts, f.Part)
	}
	if strings.Join(parts, ",") != "detection:"+incidentsContextTestDetGone+",device:"+incidentsContextTestDevDeny {
		t.Fatalf("fetch_failures = %+v", v.FetchFailures)
	}
	if !strings.Contains(stderr, "warning: 2 of 7 linked lookups failed (1 detection(s), 1 device(s))") {
		t.Fatalf("stderr warning = %q", stderr)
	}
	// The v2-only detection was tried on v1 first, exactly once each.
	v1, v2 := 0, 0
	for _, u := range srv.seen() {
		switch u.Path {
		case "/v1/detections/" + incidentsContextTestDetV2:
			v1++
		case "/v2/detections/" + incidentsContextTestDetV2:
			v2++
		case "/v2/detections/" + incidentsContextTestDetV1:
			t.Fatal("a detection v1 answered must not be fetched from v2")
		}
	}
	if v1 != 1 || v2 != 1 {
		t.Fatalf("v1/v2 lookups for the v2-only detection = %d/%d", v1, v2)
	}
}

func TestIncidentsContextCommandCapsAndErrors(t *testing.T) {
	srv := reportVulnsNewServer(t, incidentsContextTestHandler)
	missing := filepath.Join(t.TempDir(), "absent.db")
	out, _, err := reportVulnsRunCLI(t, srv, "incidents", "context", incidentsContextTestUUID, "--max-detections", "1", "--max-devices", "1", "--db", missing, "--json")
	if err != nil {
		t.Fatal(err)
	}
	var v incidentsContextView
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatal(err)
	}
	if !v.Truncated.Detections || !v.Truncated.Devices || len(v.Detections) != 1 || len(v.Devices) != 1 || len(v.FetchFailures) != 0 {
		t.Fatalf("caps = %s", out)
	}
	if v.Devices[0].Customer != "" {
		t.Fatalf("no mirror means no customer name: %+v", v.Devices[0])
	}

	_, _, err = reportVulnsRunCLI(t, srv, "incidents", "context", "00000000-0000-4000-8000-000000000000", "--db", missing, "--json")
	if err == nil || ExitCode(err) != 3 {
		t.Fatalf("unknown incident should exit 3, got %v", err)
	}
	if _, _, err := reportVulnsRunCLI(t, nil, "incidents", "context", incidentsContextTestUUID, "--max-devices", "-1", "--json"); err == nil || ExitCode(err) != 2 {
		t.Fatalf("negative cap should be a usage error, got %v", err)
	}
	dry, _, err := reportVulnsRunCLI(t, nil, "incidents", "context", incidentsContextTestUUID, "--dry-run", "--json")
	if err != nil || !strings.Contains(dry, `"dry_run":true`) {
		t.Fatalf("dry-run: %v %s", err, dry)
	}
	help, _, err := reportVulnsRunCLI(t, nil, "incidents", "context")
	if err != nil || !strings.Contains(help, "Do NOT use it for the queue of open incidents; use 'triage' instead") {
		t.Fatalf("bare invocation should print help: %v\n%s", err, help)
	}
}
