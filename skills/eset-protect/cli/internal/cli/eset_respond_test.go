// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored. Behavior tests for respond scan / isolate / release: request
// bodies against the ESET Connect guide examples, target rules, run
// completion and polling, create-response handling, and end-to-end preview
// and harness refusal through RootCmd with a seeded mirror.
package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"eset-protect-pp-cli/internal/client"
	"eset-protect-pp-cli/internal/cliutil"
	"eset-protect-pp-cli/internal/cliutil/testenv"
	"eset-protect-pp-cli/internal/store"
)

const (
	respondTestDevice = "11111111-2222-3333-4444-555555555555"
	respondTestGroup  = "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee"
)

var respondTestNow = time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)

func respondJSONEqual(t *testing.T, label string, got any, want string) {
	t.Helper()
	gb, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	var g, w any
	if err := json.Unmarshal(gb, &g); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(want), &w); err != nil {
		t.Fatalf("%s: bad expected JSON: %v", label, err)
	}
	if !reflect.DeepEqual(g, w) {
		t.Fatalf("%s body mismatch\n got: %s\nwant: %s", label, gb, want)
	}
}

// The expected bodies are the ESET Connect user guide "Automation" examples,
// narrowed to the one target kind respond sends, with respond's own
// displayName/description and a 24h manual trigger.
func TestRespondBuildBodyMatchesGuide(t *testing.T) {
	dev := respondNewTargets()
	dev.Devices = append(dev.Devices, respondRef{UUID: respondTestDevice, Name: "FRONTDESK-01"})
	cust := respondNewTargets()
	cust.Groups = append(cust.Groups, respondRef{UUID: respondTestGroup, Name: "Acme Dental"})

	cases := []struct {
		name    string
		action  string
		verb    string
		scan    respondScanOptions
		targets respondTargets
		want    string
	}{
		{"isolate", respondActionIsolate, "isolate", respondScanOptions{}, dev, `{"task":{
			"displayName":"eset-protect-cli: StartNetworkIsolation FRONTDESK-01",
			"description":"Created by eset-protect-cli (respond isolate): StartNetworkIsolation on FRONTDESK-01",
			"targets":{"devicesUuids":["11111111-2222-3333-4444-555555555555"]},
			"triggers":[{"manual":{"expireTime":"2026-10-15T12:00:00Z"}}],
			"action":{"name":"StartNetworkIsolation"}}}`},
		{"release", respondActionRelease, "release", respondScanOptions{}, dev, `{"task":{
			"displayName":"eset-protect-cli: EndNetworkIsolation FRONTDESK-01",
			"description":"Created by eset-protect-cli (respond release): EndNetworkIsolation on FRONTDESK-01",
			"targets":{"devicesUuids":["11111111-2222-3333-4444-555555555555"]},
			"triggers":[{"manual":{"expireTime":"2026-10-15T12:00:00Z"}}],
			"action":{"name":"EndNetworkIsolation"}}}`},
		{"scan", respondActionScan, "scan", respondScanOptions{Profile: "InDepth", Clean: true}, dev, `{"task":{
			"displayName":"eset-protect-cli: OnDemandScan FRONTDESK-01",
			"description":"Created by eset-protect-cli (respond scan): OnDemandScan on FRONTDESK-01",
			"targets":{"devicesUuids":["11111111-2222-3333-4444-555555555555"]},
			"triggers":[{"manual":{"expireTime":"2026-10-15T12:00:00Z"}}],
			"action":{"name":"OnDemandScan","params":{
				"@type":"type.googleapis.com/Era.Common.DataDefinition.Task.ESS.OnDemandScan",
				"scanProfile":"InDepth","customProfileName":"","scanTargets":["eset://AllTargets"],"cleaningEnabled":true}}}}`},
		{"scan-smart-customer", respondActionScan, "scan", respondScanOptions{Profile: "Smart"}, cust, `{"task":{
			"displayName":"eset-protect-cli: OnDemandScan Acme Dental",
			"description":"Created by eset-protect-cli (respond scan): OnDemandScan on Acme Dental",
			"targets":{"deviceGroupsUuids":["aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee"]},
			"triggers":[{"manual":{"expireTime":"2026-10-15T12:00:00Z"}}],
			"action":{"name":"OnDemandScan","params":{
				"@type":"type.googleapis.com/Era.Common.DataDefinition.Task.ESS.OnDemandScan",
				"scanProfile":"Smart","customProfileName":"","scanTargets":["eset://AllTargets"],"cleaningEnabled":false}}}}`},
		{"vuln", respondActionVulnScan, "scan", respondScanOptions{Profile: "InDepth"}, dev, `{"task":{
			"displayName":"eset-protect-cli: InitiateVulnerabilityScan FRONTDESK-01",
			"description":"Created by eset-protect-cli (respond scan): InitiateVulnerabilityScan on FRONTDESK-01",
			"targets":{"devicesUuids":["11111111-2222-3333-4444-555555555555"]},
			"triggers":[{"manual":{"expireTime":"2026-10-15T12:00:00Z"}}],
			"action":{"name":"InitiateVulnerabilityScan","params":{
				"@type":"type.googleapis.com/eset.dotnod.vulnerability_management.v1.ScanRequest"}}}}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			respondJSONEqual(t, tc.name, respondBuildBody(tc.action, tc.verb, tc.scan, tc.targets, respondTestNow), tc.want)
		})
	}

	// A non-UTC clock still produces a UTC RFC3339 expireTime 24h ahead.
	est := time.FixedZone("EST", -5*3600)
	body := respondBuildBody(respondActionIsolate, "isolate", respondScanOptions{}, dev, time.Date(2026, 10, 14, 20, 30, 0, 0, est))
	if got := body.Task.Triggers[0].Manual.ExpireTime; got != "2026-10-16T01:30:00Z" {
		t.Fatalf("expireTime = %q, want 2026-10-16T01:30:00Z", got)
	}
	// An empty profile defaults to InDepth rather than sending "".
	body = respondBuildBody(respondActionScan, "scan", respondScanOptions{}, dev, respondTestNow)
	if p := body.Task.Action.Params.(respondScanParams); p.ScanProfile != "InDepth" {
		t.Fatalf("default profile = %q", p.ScanProfile)
	}
}

func TestRespondValidateTargets(t *testing.T) {
	cases := []struct {
		verb     string
		args     []string
		customer string
		device   string
		errHas   string
	}{
		{"scan", []string{"FRONTDESK-01"}, "", "FRONTDESK-01", ""},
		{"scan", nil, "Acme Dental", "", ""},
		{"scan", []string{"FRONTDESK-01"}, "Acme Dental", "", "not both"},
		{"scan", nil, "", "", "--customer"},
		{"scan", []string{"  "}, "  ", "", "--customer"},
		{"scan", []string{"A", "B"}, "", "", "one device"},
		{"isolate", []string{"FRONTDESK-01"}, "", "FRONTDESK-01", ""},
		{"isolate", nil, "Acme Dental", "", "one device at a time"},
		{"isolate", []string{"FRONTDESK-01"}, "Acme Dental", "", "one device at a time"},
		{"isolate", nil, "", "", "needs a device"},
		{"release", []string{"FRONTDESK-01"}, "", "FRONTDESK-01", ""},
		{"release", nil, "Acme Dental", "", "one device at a time"},
		{"release", []string{"A", "B"}, "", "", "one device"},
	}
	for _, tc := range cases {
		got, err := respondValidateTargets(tc.verb, tc.args, tc.customer)
		if tc.errHas == "" {
			if err != nil || got != tc.device {
				t.Errorf("%s %v --customer %q: got (%q, %v), want %q", tc.verb, tc.args, tc.customer, got, err, tc.device)
			}
			continue
		}
		if err == nil || !strings.Contains(err.Error(), tc.errHas) {
			t.Errorf("%s %v --customer %q: err = %v, want containing %q", tc.verb, tc.args, tc.customer, err, tc.errHas)
		}
	}
}

func TestRespondNormalizeProfileAndWait(t *testing.T) {
	for in, want := range map[string]string{"InDepth": "InDepth", "indepth": "InDepth", "": "InDepth", "SMART": "Smart"} {
		if got, err := respondNormalizeProfile(in); err != nil || got != want {
			t.Errorf("profile %q = (%q, %v), want %q", in, got, err, want)
		}
	}
	if _, err := respondNormalizeProfile("Custom"); err == nil {
		t.Error("unsupported profile must be refused")
	}
	for in, want := range map[string]time.Duration{"0": 0, "": 0, "10m": 10 * time.Minute, "1h30m": 90 * time.Minute, "1d": 24 * time.Hour} {
		if got, err := respondParseWait(in); err != nil || got != want {
			t.Errorf("wait %q = (%v, %v), want %v", in, got, err, want)
		}
	}
	for _, bad := range []string{"soon", "-5m"} {
		if _, err := respondParseWait(bad); err == nil {
			t.Errorf("wait %q must be refused", bad)
		}
	}
}

func respondTestMirror() respondMirror {
	return respondMirror{
		present: true,
		path:    "/tmp/test.db",
		groups: []esetGroup{
			{UUID: "msp", Name: "Servosity MSP", EntityType: "DEVICE_GROUP_ENTITY_TYPE_MSP"},
			{UUID: respondTestGroup, Name: "Acme Dental", ParentUUID: "msp", EntityType: esetCompanyEntityType},
			{UUID: "acme-site", Name: "HQ", ParentUUID: respondTestGroup, EntityType: "DEVICE_GROUP_ENTITY_TYPE_SITE"},
			{UUID: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", Name: "Lost and found", ParentUUID: "msp"},
		},
		devices: []esetDevice{
			{UUID: respondTestDevice, DisplayName: "FRONTDESK-01", Hostname: "frontdesk-01.acme.local", ParentGroupUUID: "acme-site"},
			{UUID: "22222222-2222-4222-8222-222222222222", DisplayName: "XRAY-02", ParentGroupUUID: respondTestGroup},
			{UUID: "33333333-3333-4333-8333-333333333333", DisplayName: "SPARE", ParentGroupUUID: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"},
		},
	}
}

func TestRespondMirrorResolution(t *testing.T) {
	m := respondTestMirror()
	if ref, err := m.resolveDevice("frontdesk-01.acme.local", false); err != nil || ref.UUID != respondTestDevice || ref.Name != "FRONTDESK-01" {
		t.Fatalf("hostname resolution: %+v %v", ref, err)
	}
	if ref, err := m.resolveDevice(respondTestDevice, false); err != nil || ref.Name != "FRONTDESK-01" {
		t.Fatalf("uuid enriched with mirror name: %+v %v", ref, err)
	}
	if _, err := m.resolveDevice("NOPE-99", false); err == nil {
		t.Fatal("unknown name must be refused")
	}

	none := respondMirror{path: "/tmp/missing.db"}
	if ref, err := none.resolveDevice(respondTestDevice, false); err != nil || ref.UUID != respondTestDevice || ref.Name != "" {
		t.Fatalf("uuid must work without a mirror: %+v %v", ref, err)
	}
	_, err := none.resolveDevice("FRONTDESK-01", false)
	if err == nil || !strings.Contains(err.Error(), "UUID") || !strings.Contains(err.Error(), "sync --resources devices") {
		t.Fatalf("name without mirror must ask for the UUID or a sync, got %v", err)
	}
	// An existing but empty mirror (the learn hook creates the file) behaves the same.
	empty := respondMirror{present: true, path: "/tmp/empty.db"}
	if _, err := empty.resolveDevice("FRONTDESK-01", false); err == nil || !strings.Contains(err.Error(), "UUID") {
		t.Fatalf("empty mirror must ask for the UUID or a sync, got %v", err)
	}
	if ref, err := none.resolveDevice("example-value", true); err != nil || !ref.Unresolved || ref.UUID != "" {
		t.Fatalf("harness waiver must yield a marked placeholder: %+v %v", ref, err)
	}

	if ref, err := m.resolveCustomer("acme dental", false); err != nil || ref.UUID != respondTestGroup {
		t.Fatalf("customer by name: %+v %v", ref, err)
	}
	if ref, err := m.resolveCustomer(respondTestGroup, false); err != nil || ref.Name != "Acme Dental" {
		t.Fatalf("customer by uuid: %+v %v", ref, err)
	}
	if _, err := m.resolveCustomer("bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", false); err == nil || !strings.Contains(err.Error(), "not a customer") {
		t.Fatalf("a non-COMPANY group uuid must be refused, got %v", err)
	}
	if ref, err := none.resolveCustomer(respondTestGroup, false); err != nil || ref.UUID != respondTestGroup {
		t.Fatalf("customer uuid without a mirror is used as is: %+v %v", ref, err)
	}
	if _, err := none.resolveCustomer("Acme Dental", false); err == nil || !strings.Contains(err.Error(), "sync --resources device-groups") {
		t.Fatalf("customer name without a mirror must ask for a sync, got %v", err)
	}
	if n := len(m.customerDeviceUUIDs(respondTestGroup)); n != 2 {
		t.Fatalf("Acme Dental owns 2 mirrored devices (one in a site), got %d", n)
	}
}

func TestRespondRunsComplete(t *testing.T) {
	one := respondExpectation{DeviceUUIDs: []string{respondTestDevice}}
	run := func(dev, status string) respondRun { return respondRun{DeviceUUID: dev, Status: status} }
	cases := []struct {
		name string
		runs []respondRun
		want respondExpectation
		done bool
	}{
		{"zero runs is never complete", nil, one, false},
		{"zero runs, nothing expected", []respondRun{}, respondExpectation{}, false},
		{"queued", []respondRun{run(respondTestDevice, "QUEUED")}, one, false},
		{"running", []respondRun{run(respondTestDevice, "TASK_RUN_STATUS_RUNNING")}, one, false},
		{"finished", []respondRun{run(respondTestDevice, "FINISHED")}, one, true},
		{"failed is terminal", []respondRun{run(respondTestDevice, "TASK_RUN_STATUS_FAILED")}, one, true},
		{"suppressed is terminal", []respondRun{run(respondTestDevice, "SUPPRESSED")}, one, true},
		{"unspecified is not terminal", []respondRun{run(respondTestDevice, "UNSPECIFIED")}, one, false},
		{"unknown enum is not terminal", []respondRun{run(respondTestDevice, "TASK_RUN_STATUS_PAUSED")}, one, false},
		{"terminal run on another device", []respondRun{run("other", "FINISHED")}, one, false},
		{"uuid case-insensitive", []respondRun{run(strings.ToUpper(respondTestDevice), "FINISHED")}, one, true},
		{"customer: fewer runs than devices", []respondRun{run("a", "FINISHED"), run("b", "FINISHED")}, respondExpectation{MinRuns: 3}, false},
		{"customer: one still running", []respondRun{run("a", "FINISHED"), run("b", "RUNNING"), run("c", "FINISHED")}, respondExpectation{MinRuns: 3}, false},
		{"customer: all terminal", []respondRun{run("a", "FINISHED"), run("b", "FAILED"), run("c", "FINISHED")}, respondExpectation{MinRuns: 3}, true},
		{"customer: membership unknown is never complete", []respondRun{run("a", "FINISHED")}, respondExpectation{Unknown: true}, false},
	}
	for _, tc := range cases {
		if got := respondRunsComplete(tc.runs, tc.want); got != tc.done {
			t.Errorf("%s: complete = %v, want %v", tc.name, got, tc.done)
		}
	}
}

func TestRespondDecodeRuns(t *testing.T) {
	items := []json.RawMessage{
		json.RawMessage(`{"uuid":"r1","taskUuid":"t1","deviceUuid":"d1","status":"TASK_RUN_STATUS_FINISHED","startTime":"2026-10-14T12:01:00Z","endTime":"2026-10-14T12:09:00Z",
			"result":{"@type":"type.googleapis.com/Era.Common.DataDefinition.Task.OnDemandScanResult","scanned":5120,"infected":1,"cleaned":0,"severity":"LOG_INFORMATION"}}`),
		json.RawMessage(`{"deviceUuid":"d2","result":null}`),
		json.RawMessage(`not json`),
	}
	runs, rejected := respondDecodeRuns(items)
	if len(runs) != 2 || rejected != 1 {
		t.Fatalf("want 2 decoded runs and 1 rejected, got %d and %d", len(runs), rejected)
	}
	if runs[0].Status != "FINISHED" || runs[0].StartTime == "" || !strings.Contains(string(runs[0].Result), "OnDemandScanResult") {
		t.Fatalf("run 0: %+v", runs[0])
	}
	if runs[1].Status != "UNSPECIFIED" || runs[1].Result != nil {
		t.Fatalf("missing status/null result: %+v", runs[1])
	}
	if got, _ := respondDecodeRuns(nil); got == nil || len(got) != 0 {
		t.Fatalf("empty input must give an empty, non-nil slice: %#v", got)
	}
}

// respondSwapFetcher replaces the package-level runs fetcher for one test.
func respondSwapFetcher(t *testing.T, fn func(ctx context.Context, c *client.Client, taskUUID string) ([]respondRun, error)) {
	t.Helper()
	orig := respondFetchRuns
	respondFetchRuns = fn
	t.Cleanup(func() { respondFetchRuns = orig })
}

func TestRespondPollRuns(t *testing.T) {
	want := respondExpectation{DeviceUUIDs: []string{respondTestDevice}}

	t.Run("completes after runs appear and finish", func(t *testing.T) {
		seq := [][]respondRun{
			{},
			{{DeviceUUID: respondTestDevice, Status: "QUEUED"}},
			{{DeviceUUID: respondTestDevice, Status: "RUNNING"}},
			{{DeviceUUID: respondTestDevice, Status: "FINISHED"}},
		}
		var calls int32
		respondSwapFetcher(t, func(_ context.Context, _ *client.Client, task string) ([]respondRun, error) {
			if task != "task-1" {
				t.Errorf("fetched task %q", task)
			}
			i := atomic.AddInt32(&calls, 1) - 1
			if int(i) >= len(seq) {
				i = int32(len(seq) - 1)
			}
			return seq[i], nil
		})
		res := respondPollRuns(context.Background(), nil, "task-1", want, 5*time.Second, time.Millisecond, nil)
		if !res.Complete || len(res.Runs) != 1 || res.Runs[0].Status != "FINISHED" || res.LastError != nil {
			t.Fatalf("poll result: %+v", res)
		}
		if calls != 4 {
			t.Fatalf("want 4 fetches (stop at first complete), got %d", calls)
		}
	})

	t.Run("zero runs until the wait expires is incomplete, not an error", func(t *testing.T) {
		respondSwapFetcher(t, func(context.Context, *client.Client, string) ([]respondRun, error) { return []respondRun{}, nil })
		start := time.Now()
		res := respondPollRuns(context.Background(), nil, "task-1", want, 30*time.Millisecond, 5*time.Millisecond, nil)
		if res.Complete || res.Runs == nil || len(res.Runs) != 0 || res.LastError != nil {
			t.Fatalf("poll result: %+v", res)
		}
		if el := time.Since(start); el < 30*time.Millisecond || el > 2*time.Second {
			t.Fatalf("poll should stop at the wait deadline, took %v", el)
		}
	})

	t.Run("a transient fetch error does not end the watch", func(t *testing.T) {
		var calls int32
		respondSwapFetcher(t, func(context.Context, *client.Client, string) ([]respondRun, error) {
			if atomic.AddInt32(&calls, 1) == 1 {
				return nil, errors.New("GET runs returned HTTP 503")
			}
			return []respondRun{{DeviceUUID: respondTestDevice, Status: "FAILED"}}, nil
		})
		res := respondPollRuns(context.Background(), nil, "task-1", want, 5*time.Second, time.Millisecond, nil)
		if !res.Complete || res.LastError != nil {
			t.Fatalf("poll result: %+v", res)
		}
	})

	t.Run("a persistent fetch error is reported", func(t *testing.T) {
		respondSwapFetcher(t, func(context.Context, *client.Client, string) ([]respondRun, error) {
			return nil, errors.New("GET runs returned HTTP 503")
		})
		res := respondPollRuns(context.Background(), nil, "task-1", want, 20*time.Millisecond, 5*time.Millisecond, nil)
		if res.Complete || res.LastError == nil || res.Runs == nil {
			t.Fatalf("poll result: %+v", res)
		}
	})

	t.Run("cancellation stops the wait promptly", func(t *testing.T) {
		respondSwapFetcher(t, func(context.Context, *client.Client, string) ([]respondRun, error) {
			return []respondRun{{DeviceUUID: respondTestDevice, Status: "RUNNING"}}, nil
		})
		ctx, cancel := context.WithCancel(context.Background())
		time.AfterFunc(20*time.Millisecond, cancel)
		start := time.Now()
		res := respondPollRuns(ctx, nil, "task-1", want, time.Hour, time.Hour, nil)
		if el := time.Since(start); el > 2*time.Second {
			t.Fatalf("cancel ignored: waited %v", el)
		}
		if res.Complete || !errors.Is(res.LastError, context.Canceled) || len(res.Runs) != 1 {
			t.Fatalf("poll result: %+v", res)
		}
	})

	t.Run("each fetch gets its own request context", func(t *testing.T) {
		var bounded int32
		respondSwapFetcher(t, func(ctx context.Context, _ *client.Client, _ string) ([]respondRun, error) {
			if _, ok := ctx.Deadline(); ok {
				atomic.AddInt32(&bounded, 1)
			}
			return []respondRun{{DeviceUUID: respondTestDevice, Status: "FINISHED"}}, nil
		})
		res := respondPollRuns(context.Background(), nil, "task-1", want, time.Second, time.Millisecond,
			func(p context.Context) (context.Context, context.CancelFunc) {
				return context.WithTimeout(p, time.Minute)
			})
		if !res.Complete || bounded != 1 {
			t.Fatalf("complete=%v bounded=%d", res.Complete, bounded)
		}
	})
}

func TestRespondParseCreated(t *testing.T) {
	if id, err := respondParseCreated(json.RawMessage(`{"task":{"uuid":"9c1d2e3f-0000-4000-8000-000000000001","displayName":"x"}}`)); err != nil || id != "9c1d2e3f-0000-4000-8000-000000000001" {
		t.Fatalf("task uuid: %q %v", id, err)
	}
	if id, err := respondParseCreated(json.RawMessage(`{"uuid":"flat-1"}`)); err != nil || id != "flat-1" {
		t.Fatalf("flat uuid: %q %v", id, err)
	}
	for _, bad := range []string{``, `  `, `null`, `{}`, `{"task":{}}`, `[1,2]`} {
		if _, err := respondParseCreated(json.RawMessage(bad)); err == nil {
			t.Errorf("response %q must not be treated as a created task", bad)
		}
	}
}

func TestRespondCreateError(t *testing.T) {
	empty500 := &client.APIError{Method: "POST", Path: "https://eu.automation.eset.systems/v1/device_tasks", StatusCode: 500, Body: ""}
	err := respondCreateError(empty500)
	msg := err.Error()
	for _, want := range []string{"HTTP 500", "ESET Connect", "empty body", "device-tasks list"} {
		if !strings.Contains(msg, want) {
			t.Errorf("empty-body 500 message missing %q: %s", want, msg)
		}
	}
	var ae *client.APIError
	if !errors.As(err, &ae) {
		t.Fatal("the wrapped error must keep the APIError")
	}
	if code := ExitCode(classifyAPIErrorOnly(err)); code != 5 {
		t.Fatalf("empty-body 500 must exit as an API error (5), got %d", code)
	}
	bad := &client.APIError{Method: "POST", Path: "x", StatusCode: 400, Body: `{"message":"invalid"}`}
	if got := respondCreateError(bad); got != error(bad) {
		t.Fatalf("4xx must pass through unchanged, got %v", got)
	}
	if got := respondCreateError(errors.New("dial tcp: timeout")); got.Error() != "dial tcp: timeout" {
		t.Fatalf("non-API errors pass through, got %v", got)
	}
}

// respondSeedMirror writes a small MSP tree and three devices to a temp DB.
func respondSeedMirror(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "mirror.db")
	db, err := store.OpenWithContext(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	m := respondTestMirror()
	for _, g := range m.groups {
		raw, _ := json.Marshal(map[string]any{"uuid": g.UUID, "displayName": g.Name, "parentGroupUuid": g.ParentUUID, "linkedEntityType": g.EntityType})
		if err := db.Upsert("device-groups", g.UUID, raw); err != nil {
			t.Fatal(err)
		}
	}
	for _, d := range m.devices {
		raw, _ := json.Marshal(map[string]any{"uuid": d.UUID, "displayName": d.DisplayName, "systemHostname": d.Hostname, "parentGroupUuid": d.ParentGroupUUID})
		if err := db.Upsert("devices", d.UUID, raw); err != nil {
			t.Fatal(err)
		}
	}
	return path
}

func respondRunRoot(t *testing.T, args ...string) (map[string]any, string, error) {
	t.Helper()
	cmd := RootCmd()
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs(args)
	err := cmd.Execute()
	var parsed map[string]any
	if s := strings.TrimSpace(out.String()); strings.HasPrefix(s, "{") {
		if jerr := json.Unmarshal([]byte(s), &parsed); jerr != nil {
			t.Fatalf("output is not one JSON object: %v\n%s", jerr, s)
		}
	}
	return parsed, out.String() + errOut.String(), err
}

func respondClearHarness(t *testing.T) {
	t.Setenv(cliutil.VerifyEnvVar, "")
	t.Setenv(cliutil.DogfoodEnvVar, "")
}

func TestRespondScanPreviewEndToEnd(t *testing.T) {
	testenv.Isolate(t)
	respondClearHarness(t)
	db := respondSeedMirror(t)

	got, text, err := respondRunRoot(t, "respond", "scan", "FRONTDESK-01", "--clean", "--db", db, "--json")
	if err != nil {
		t.Fatalf("preview must exit 0: %v\n%s", err, text)
	}
	if got["preview"] != true || got["action"] != respondActionScan {
		t.Fatalf("preview envelope: %v", got)
	}
	devs := got["targets"].(map[string]any)["devices"].([]any)
	if len(devs) != 1 || devs[0].(map[string]any)["uuid"] != respondTestDevice || devs[0].(map[string]any)["name"] != "FRONTDESK-01" {
		t.Fatalf("resolved targets: %v", got["targets"])
	}
	if groups := got["targets"].(map[string]any)["groups"].([]any); len(groups) != 0 {
		t.Fatalf("groups must be an empty array: %v", groups)
	}
	req := got["request"].(map[string]any)
	// The URL --yes would send to (preview-url-is-the-sent-url): the
	// isolated test env has no region, so the eu default.
	if req["method"] != "POST" || req["url"] != "https://eu.automation.eset.systems/v1/device_tasks" {
		t.Fatalf("request: %v", req)
	}
	task := req["body"].(map[string]any)["task"].(map[string]any)
	if ids := task["targets"].(map[string]any)["devicesUuids"].([]any); len(ids) != 1 || ids[0] != respondTestDevice {
		t.Fatalf("body targets: %v", task["targets"])
	}
	params := task["action"].(map[string]any)["params"].(map[string]any)
	if params["@type"] != respondScanParamsType || params["cleaningEnabled"] != true || params["scanProfile"] != "InDepth" {
		t.Fatalf("scan params: %v", params)
	}
	exp, perr := time.Parse(time.RFC3339, task["triggers"].([]any)[0].(map[string]any)["manual"].(map[string]any)["expireTime"].(string))
	if perr != nil || exp.Sub(time.Now()) < 23*time.Hour || exp.Sub(time.Now()) > 25*time.Hour {
		t.Fatalf("expireTime should be ~now+24h: %v %v", exp, perr)
	}
	if note, _ := got["note"].(string); !strings.Contains(note, "nothing was sent") {
		t.Fatalf("note: %q", note)
	}
}

func TestRespondCustomerVulnPreviewEndToEnd(t *testing.T) {
	testenv.Isolate(t)
	respondClearHarness(t)
	db := respondSeedMirror(t)

	got, text, err := respondRunRoot(t, "respond", "scan", "--customer", "acme dental", "--vuln", "--db", db, "--json")
	if err != nil {
		t.Fatalf("customer preview must exit 0: %v\n%s", err, text)
	}
	if got["action"] != respondActionVulnScan {
		t.Fatalf("action: %v", got["action"])
	}
	groups := got["targets"].(map[string]any)["groups"].([]any)
	if len(groups) != 1 || groups[0].(map[string]any)["uuid"] != respondTestGroup {
		t.Fatalf("customer target: %v", got["targets"])
	}
	tt := got["request"].(map[string]any)["body"].(map[string]any)["task"].(map[string]any)["targets"].(map[string]any)
	if _, has := tt["devicesUuids"]; has {
		t.Fatalf("customer scan must not send devicesUuids: %v", tt)
	}
	if ids := tt["deviceGroupsUuids"].([]any); len(ids) != 1 || ids[0] != respondTestGroup {
		t.Fatalf("deviceGroupsUuids: %v", tt)
	}
}

func TestRespondIsolatePreviewWarnsAndRefusesCustomer(t *testing.T) {
	testenv.Isolate(t)
	respondClearHarness(t)
	db := respondSeedMirror(t)

	got, text, err := respondRunRoot(t, "respond", "isolate", "FRONTDESK-01", "--db", db, "--json")
	if err != nil {
		t.Fatalf("isolate preview must exit 0: %v\n%s", err, text)
	}
	if note, _ := got["note"].(string); !strings.Contains(note, "except for ESET management") || !strings.Contains(note, "released") {
		t.Fatalf("isolate preview note must warn about network loss: %q", note)
	}

	_, text, err = respondRunRoot(t, "respond", "isolate", "FRONTDESK-01", "--customer", "Acme Dental", "--db", db, "--json")
	if err == nil || ExitCode(err) != 2 || !strings.Contains(text, "one device at a time") {
		t.Fatalf("isolate --customer must be a usage error explaining why: code=%d err=%v\n%s", ExitCode(err), err, text)
	}
	_, text, err = respondRunRoot(t, "respond", "scan", "FRONTDESK-01", "--customer", "Acme Dental", "--db", db, "--json")
	if err == nil || ExitCode(err) != 2 || !strings.Contains(text, "not both") {
		t.Fatalf("scan with device and --customer must be a usage error: %v\n%s", err, text)
	}
	_, _, err = respondRunRoot(t, "respond", "scan", "FRONTDESK-01", "--vuln", "--scan-profile", "Smart", "--db", db, "--json")
	if err == nil || ExitCode(err) != 2 {
		t.Fatalf("--vuln with --scan-profile must be a usage error: %v", err)
	}
}

func TestRespondUnresolvedNameEndToEnd(t *testing.T) {
	testenv.Isolate(t)
	respondClearHarness(t)
	missing := filepath.Join(t.TempDir(), "absent.db")

	_, text, err := respondRunRoot(t, "respond", "release", "FRONTDESK-01", "--db", missing, "--json")
	if err == nil || ExitCode(err) != 2 || !strings.Contains(text, "UUID") {
		t.Fatalf("a name with no mirror must be a usage error asking for the UUID: code=%d %v\n%s", ExitCode(err), err, text)
	}

	// A UUID needs no mirror at all.
	got, text, err := respondRunRoot(t, "respond", "release", respondTestDevice, "--db", missing, "--json")
	if err != nil || got["preview"] != true {
		t.Fatalf("uuid preview without a mirror must exit 0: %v\n%s", err, text)
	}

	// Under the dogfood harness the happy-path placeholder still previews.
	t.Setenv(cliutil.DogfoodEnvVar, "1")
	got, text, err = respondRunRoot(t, "respond", "scan", "example-value", "--db", missing, "--json")
	if err != nil || got["preview"] != true {
		t.Fatalf("harness placeholder preview must exit 0: %v\n%s", err, text)
	}
	if w, _ := got["warnings"].([]any); len(w) != 1 {
		t.Fatalf("placeholder preview must carry a warning: %v", got)
	}
	ids := got["request"].(map[string]any)["body"].(map[string]any)["task"].(map[string]any)["targets"].(map[string]any)["devicesUuids"].([]any)
	if ids[0] != "<unresolved:example-value>" {
		t.Fatalf("placeholder must be visibly unresolved: %v", ids)
	}
}

func TestRespondHarnessRefusesYes(t *testing.T) {
	testenv.Isolate(t)
	respondClearHarness(t)
	db := respondSeedMirror(t)
	t.Setenv(cliutil.VerifyEnvVar, "1")

	for _, args := range [][]string{
		{"respond", "isolate", "FRONTDESK-01"},
		{"respond", "scan", "--customer", "Acme Dental"},
		{"respond", "release", respondTestDevice},
	} {
		got, text, err := respondRunRoot(t, append(args, "--db", db, "--json", "--yes", "--wait", "1m")...)
		if err != nil {
			t.Fatalf("%v: harness refusal must exit 0: %v\n%s", args, err, text)
		}
		if got["refused"] != true || got["harness"] != "verify" {
			t.Fatalf("%v: want a structured refusal, got %v", args, got)
		}
		if reason, _ := got["reason"].(string); !strings.Contains(reason, "harness") {
			t.Fatalf("%v: refusal reason: %q", args, reason)
		}
		if _, created := got["created"]; created {
			t.Fatalf("%v: nothing may be created under a harness: %v", args, got)
		}
	}
}

func TestRespondResultNote(t *testing.T) {
	r := respondResultView{Runs: make([]respondRun, 0)}
	if n := respondResultNote(r, 0, "t1"); !strings.Contains(n, "not waiting") || !strings.Contains(n, "device-tasks runs device-tasks-list-task t1") {
		t.Fatalf("no-wait note: %q", n)
	}
	if n := respondResultNote(r, time.Minute, "t1"); !strings.Contains(n, "picks the task up") {
		t.Fatalf("zero-runs note: %q", n)
	}
	r.Complete = true
	if n := respondResultNote(r, time.Minute, "t1"); n != "" {
		t.Fatalf("complete result needs no note: %q", n)
	}
}

// respondFakeESET points the client at an httptest server standing in for
// ESET Connect (ESET_PROTECT_BASE_URL rebases the absolute ESET hosts onto
// it) with a static bearer token, so the real --yes path runs offline.
func respondFakeESET(t *testing.T, h http.HandlerFunc) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	t.Setenv("ESET_PROTECT_BASE_URL", srv.URL)
	t.Setenv("ESET_PROTECT_TOKEN", "test-token")
	t.Setenv("ESET_PROTECT_REGION", "eu")
	t.Setenv("ESET_PROTECT_USERNAME", "")
	t.Setenv("ESET_PROTECT_PASSWORD", "")
	was := respondPollInterval
	respondPollInterval = 5 * time.Millisecond
	t.Cleanup(func() { respondPollInterval = was })
}

func TestRespondYesCreatesAndWaitsEndToEnd(t *testing.T) {
	testenv.Isolate(t)
	respondClearHarness(t)
	db := respondSeedMirror(t)

	var posted map[string]any
	var runCalls int32
	respondFakeESET(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/device_tasks":
			if err := json.NewDecoder(r.Body).Decode(&posted); err != nil {
				t.Errorf("decoding posted body: %v", err)
			}
			_, _ = io.WriteString(w, `{"task":{"uuid":"9c1d2e3f-0000-4000-8000-000000000001","displayName":"x"}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/device_tasks/9c1d2e3f-0000-4000-8000-000000000001/runs":
			if r.URL.Query().Get("listOnlyLastRuns") != "true" {
				t.Errorf("runs must ask for the latest run per device: %s", r.URL.RawQuery)
			}
			if atomic.AddInt32(&runCalls, 1) == 1 {
				_, _ = io.WriteString(w, `{"taskRuns":[],"nextPageToken":""}`)
				return
			}
			_, _ = io.WriteString(w, `{"taskRuns":[{"deviceUuid":"`+respondTestDevice+`","status":"TASK_RUN_STATUS_FINISHED","startTime":"2026-10-14T12:01:00Z","endTime":"2026-10-14T12:02:00Z","result":{"@type":"type.googleapis.com/Era.Common.DataDefinition.Task.OnDemandScanResult","scanned":10,"infected":0}}]}`)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusTeapot)
		}
	})

	got, text, err := respondRunRoot(t, "respond", "isolate", "FRONTDESK-01", "--yes", "--wait", "30s", "--db", db, "--json")
	if err != nil {
		t.Fatalf("create+wait must exit 0: %v\n%s", err, text)
	}
	task := posted["task"].(map[string]any)
	if task["action"].(map[string]any)["name"] != respondActionIsolate || task["targets"].(map[string]any)["devicesUuids"].([]any)[0] != respondTestDevice {
		t.Fatalf("posted body: %v", posted)
	}
	created := got["created"].(map[string]any)
	if created["task_uuid"] != "9c1d2e3f-0000-4000-8000-000000000001" || created["action"] != respondActionIsolate {
		t.Fatalf("created: %v", created)
	}
	runs := got["runs"].([]any)
	if got["complete"] != true || len(runs) != 1 {
		t.Fatalf("want one finished run and complete=true: %v", got)
	}
	run := runs[0].(map[string]any)
	if run["status"] != "FINISHED" || run["device_uuid"] != respondTestDevice || run["device_name"] != "FRONTDESK-01" || run["result"] == nil {
		t.Fatalf("run: %v", run)
	}
	if runCalls != 2 {
		t.Fatalf("want 2 runs fetches (empty, then finished), got %d", runCalls)
	}
}

func TestRespondYesWithoutWaitReportsIncomplete(t *testing.T) {
	testenv.Isolate(t)
	respondClearHarness(t)
	respondFakeESET(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("--wait 0 must not poll: %s %s", r.Method, r.URL.Path)
		}
		_, _ = io.WriteString(w, `{"task":{"uuid":"task-nowait"}}`)
	})
	got, text, err := respondRunRoot(t, "respond", "scan", respondTestDevice, "--vuln", "--yes", "--json")
	if err != nil {
		t.Fatalf("exit: %v\n%s", err, text)
	}
	if got["complete"] != false || len(got["runs"].([]any)) != 0 || got["waited_seconds"] != float64(0) {
		t.Fatalf("no-wait result: %v", got)
	}
	if note, _ := got["note"].(string); !strings.Contains(note, "task-nowait") {
		t.Fatalf("note must say how to follow the task: %q", note)
	}
}

func TestRespondYesEmptyBody500IsAnESETError(t *testing.T) {
	testenv.Isolate(t)
	respondClearHarness(t)
	var posts int32
	respondFakeESET(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&posts, 1)
		w.WriteHeader(http.StatusInternalServerError)
	})
	got, text, err := respondRunRoot(t, "respond", "release", respondTestDevice, "--yes", "--wait", "1m", "--json")
	if err == nil || ExitCode(err) != 5 {
		t.Fatalf("empty-body 500 must fail as an API error (exit 5): code=%d %v\n%s", ExitCode(err), err, text)
	}
	if _, ok := got["created"]; ok {
		t.Fatalf("a 500 must never be reported as a created task: %v", got)
	}
	if !strings.Contains(err.Error(), "ESET Connect") || !strings.Contains(err.Error(), "empty body") {
		t.Fatalf("error must name ESET and the empty body: %v", err)
	}
	if posts != 1 {
		t.Fatalf("a non-idempotent create must not be retried blindly, got %d POSTs", posts)
	}
}

func TestRespondPollRunsHonorsWaitDeadline(t *testing.T) {
	respondSwapFetcher(t, func(ctx context.Context, _ *client.Client, _ string) ([]respondRun, error) {
		<-ctx.Done() // a fetch that would hang past --wait
		return nil, ctx.Err()
	})
	start := time.Now()
	res := respondPollRuns(context.Background(), nil, "task-1", respondExpectation{DeviceUUIDs: []string{respondTestDevice}}, 150*time.Millisecond, 10*time.Millisecond, nil)
	if took := time.Since(start); took > 2*time.Second {
		t.Fatalf("--wait 150ms took %s; fetches must be bound by the wait deadline", took)
	}
	if res.Complete {
		t.Fatal("a hung fetch must not be reported complete")
	}
}

func TestRespondPollRunsIncompleteListingNeverCompletes(t *testing.T) {
	respondSwapFetcher(t, func(context.Context, *client.Client, string) ([]respondRun, error) {
		return []respondRun{{DeviceUUID: respondTestDevice, Status: "FINISHED"}}, errors.New("the task has more runs than 5 pages")
	})
	res := respondPollRuns(context.Background(), nil, "task-1", respondExpectation{DeviceUUIDs: []string{respondTestDevice}}, 50*time.Millisecond, 10*time.Millisecond, nil)
	if res.Complete || res.LastError == nil || len(res.Runs) != 1 {
		t.Fatalf("truncated listing: complete=%v err=%v runs=%d", res.Complete, res.LastError, len(res.Runs))
	}
}
