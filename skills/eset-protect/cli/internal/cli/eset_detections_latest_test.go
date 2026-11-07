// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored. `detections latest` is live-only, so its parse, filter,
// sort and enrichment logic is tested with fixtures.
package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"eset-protect-pp-cli/internal/client"
	"eset-protect-pp-cli/internal/cliutil/testenv"
)

func detectionsLatestTestRaw(t *testing.T, items ...map[string]any) []json.RawMessage {
	t.Helper()
	out := make([]json.RawMessage, 0, len(items))
	for _, it := range items {
		b, err := json.Marshal(it)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, b)
	}
	return out
}

func detectionsLatestTestUUIDs(rows []detectionsLatestRow) string {
	ids := make([]string, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.UUID)
	}
	return strings.Join(ids, ",")
}

func TestDetectionsLatestParse(t *testing.T) {
	v1 := detectionsLatestTestRaw(t,
		map[string]any{"uuid": "v1-a", "displayName": "Win32/Filecoder.WannaCryptor.D", "typeName": "Trojan", "objectName": `C:\Temp\tasksche.exe`,
			"objectHashSha1": "3395856CE81F2B7382DEE72602F798B642F14140", "category": "DETECTION_CATEGORY_ANTIVIRUS", "severityLevel": "SEVERITY_LEVEL_HIGH",
			"occurTime": "2026-10-14T08:15:30.123Z", "context": map[string]any{"deviceUuid": triageTestFront}},
		map[string]any{"uuid": "v1-b", "typeName": "Potentially unwanted application", "severityLevel": "SEVERITY_LEVEL_LOW", "occurTime": "garbled"},
		map[string]any{"displayName": "no uuid is dropped"},
	)
	v1 = append(v1, json.RawMessage(`{broken`), json.RawMessage(`{"uuid":"v1-typed","severityLevel":3,"occurTime":"2026-10-14T08:00:00Z"}`))
	var rej esetRejects
	rows := detectionsLatestParse(v1, detectionsLatestSourceEndpoint, &rej)
	if len(rows) != 2 {
		t.Fatalf("want 2 parsed rows, got %d: %+v", len(rows), rows)
	}
	// no uuid, broken JSON and a numeric severityLevel are counted, not dropped.
	if rej.N != 3 || !strings.HasPrefix(rej.First, "endpoint detection row 2:") {
		t.Fatalf("rejects = %+v", rej)
	}
	a := rows[0]
	if a.OccurredAt != "2026-10-14T08:15:30Z" || a.Severity != "high" || a.Name != "Win32/Filecoder.WannaCryptor.D" || a.Object != `C:\Temp\tasksche.exe` ||
		a.SHA1 != "3395856ce81f2b7382dee72602f798b642f14140" || a.Category != "antivirus" || a.DeviceUUID != triageTestFront ||
		a.Device != triageTestFront || a.Source != "endpoint" || a.Customer != "" {
		t.Fatalf("v1 row = %+v", a)
	}
	if b := rows[1]; b.Name != "Potentially unwanted application" || b.OccurredAt != "garbled" || !b.at.IsZero() {
		t.Fatalf("typeName fallback / malformed time row = %+v", b)
	}

	v2 := detectionsLatestTestRaw(t, map[string]any{"uuid": "v2-a", "displayName": "Phishing link", "severityLevel": "SEVERITY_LEVEL_MEDIUM",
		"occurTime": "2026-10-14T09:00:00Z", "category": "DETECTION_CATEGORY_WEB_ACCESS", "resolved": false,
		"device": map[string]any{"uuid": "c0ffee00-0000-4000-8000-000000000001", "displayName": "mailbox frontdesk@acmedental.example"}})
	c := detectionsLatestParse(v2, detectionsLatestSourceCloudOffice, &rej)
	if len(c) != 1 || c[0].DeviceUUID != "c0ffee00-0000-4000-8000-000000000001" || c[0].Device != "mailbox frontdesk@acmedental.example" ||
		c[0].Source != "cloud-office" || c[0].Category != "web_access" {
		t.Fatalf("v2 row = %+v", c)
	}
	if rej.N != 3 {
		t.Fatalf("a clean v2 row must not add a reject: %+v", rej)
	}
	if empty := detectionsLatestParse(nil, detectionsLatestSourceEndpoint, nil); empty == nil || len(empty) != 0 {
		t.Fatal("empty input must give an empty, non-nil slice")
	}
}

func TestDetectionsLatestSelect(t *testing.T) {
	raw := detectionsLatestTestRaw(t,
		map[string]any{"uuid": "old-high", "severityLevel": "SEVERITY_LEVEL_HIGH", "occurTime": "2026-10-13T08:00:00Z", "context": map[string]any{"deviceUuid": triageTestFront}},
		map[string]any{"uuid": "new-low", "severityLevel": "SEVERITY_LEVEL_LOW", "occurTime": "2026-10-14T10:00:00Z", "context": map[string]any{"deviceUuid": triageTestBright}},
		map[string]any{"uuid": "mid-med", "severityLevel": "SEVERITY_LEVEL_MEDIUM", "occurTime": "2026-10-14T06:00:00Z", "context": map[string]any{"deviceUuid": triageTestFront}},
		map[string]any{"uuid": "undated-high", "severityLevel": "SEVERITY_LEVEL_HIGH", "context": map[string]any{"deviceUuid": triageTestFront}},
		map[string]any{"uuid": "new-info", "severityLevel": "SEVERITY_LEVEL_INFORMATIONAL", "occurTime": "2026-10-14T11:00:00Z", "context": map[string]any{"deviceUuid": triageTestFront}},
	)
	rows := detectionsLatestParse(raw, detectionsLatestSourceEndpoint, nil)
	cases := []struct {
		name, min, device string
		limit             int
		want              string
	}{
		{"newest first, undated last, no floor", "", "", 20, "new-info,new-low,mid-med,old-high,undated-high"},
		{"medium floor", "medium", "", 20, "mid-med,old-high,undated-high"},
		{"high floor with limit", "high", "", 1, "old-high"},
		{"device filter (case-insensitive)", "", strings.ToUpper(triageTestBright), 20, "new-low"},
		{"floor excludes everything", "high", triageTestBright, 20, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := detectionsLatestSelect(rows, tc.min, tc.device, tc.limit)
			if got == nil {
				t.Fatal("result must be non-nil")
			}
			if ids := detectionsLatestTestUUIDs(got); ids != tc.want {
				t.Fatalf("got %s, want %s", ids, tc.want)
			}
		})
	}
}

func TestDetectionsLatestEnrich(t *testing.T) {
	tree := newEsetTree([]esetGroup{
		{UUID: "acme", Name: "Acme Dental", EntityType: esetCompanyEntityType},
		{UUID: "acme-hq", Name: "HQ", ParentUUID: "acme"},
	})
	devices := []esetDevice{{UUID: triageTestFront, DisplayName: "FRONTDESK-01", ParentGroupUUID: "acme-hq"}}
	rows := []detectionsLatestRow{
		{UUID: "known", DeviceUUID: triageTestFront, Device: triageTestFront},
		{UUID: "unknown", DeviceUUID: "d0000099-aaaa-4bbb-8ccc-000000000099", Device: "d0000099-aaaa-4bbb-8ccc-000000000099"},
	}
	un := detectionsLatestEnrich(rows, devices, tree)
	if rows[0].Device != "FRONTDESK-01" || rows[0].Customer != "Acme Dental" {
		t.Fatalf("known device not joined: %+v", rows[0])
	}
	// An unknown device keeps its UUID and is labelled owner-unknown, not
	// left blank as if there were no mirror (customer-views-count-unattributed-rows).
	if rows[1].Device != "d0000099-aaaa-4bbb-8ccc-000000000099" || rows[1].Customer != detectionsSpreadUnknownDevice {
		t.Fatalf("unknown device must keep its UUID and read %q: %+v", detectionsSpreadUnknownDevice, rows[1])
	}
	if un.N != 1 || !reflect.DeepEqual(un.UUIDs(), []string{"unknown"}) {
		t.Fatalf("unattributed = %d %v, want the unknown-device row", un.N, un.UUIDs())
	}
	// No mirror: enrichment is a no-op, never a panic.
	detectionsLatestEnrich(rows, nil, nil)
	// A mirror holding neither devices nor groups has nothing to join.
	bare := []detectionsLatestRow{{UUID: "x", DeviceUUID: triageTestFront}}
	if un := detectionsLatestEnrich(bare, nil, newEsetTree(nil)); un.N != 0 || bare[0].Customer != "" {
		t.Fatalf("an empty mirror labelled a row: %+v (%d)", bare[0], un.N)
	}

	// Codex round 4: an EMPTY device mirror beside a synced COMPANY tree
	// (every endpoint deleted, devices resynced) still files every retained
	// detection as an unknown device and counts it, instead of returning
	// early with blank customers and nothing unattributed.
	empty := []detectionsLatestRow{
		{UUID: "kept-1", DeviceUUID: triageTestFront, Device: triageTestFront},
		{UUID: "kept-2", DeviceUUID: "", Device: ""},
	}
	un = detectionsLatestEnrich(empty, nil, tree)
	if un.N != 2 || !reflect.DeepEqual(un.UUIDs(), []string{"kept-1", "kept-2"}) {
		t.Fatalf("empty device mirror: unattributed = %d %v, want both detections", un.N, un.UUIDs())
	}
	for _, r := range empty {
		if r.Customer != detectionsSpreadUnknownDevice {
			t.Fatalf("empty device mirror: %+v, want customer %q", r, detectionsSpreadUnknownDevice)
		}
	}
	// A single-tenant tree (no COMPANY) with no devices: every row is the
	// tenant's, as in every other view.
	single := []detectionsLatestRow{{UUID: "s", DeviceUUID: triageTestFront}}
	if un := detectionsLatestEnrich(single, nil, newEsetTree([]esetGroup{{UUID: "all", Name: "All"}})); un.N != 0 || single[0].Customer != esetCustomerTenant {
		t.Fatalf("single tenant, no devices: %+v (%d)", single[0], un.N)
	}

	// Two customers sharing a name are told apart by a UUID prefix.
	dup := newEsetTree([]esetGroup{
		{UUID: "aaaaaaaa-1111-4000-8000-000000000001", Name: "Acme", EntityType: esetCompanyEntityType},
		{UUID: "bbbbbbbb-2222-4000-8000-000000000002", Name: "Acme", EntityType: esetCompanyEntityType},
	})
	two := []detectionsLatestRow{{UUID: "a", DeviceUUID: "d1"}, {UUID: "b", DeviceUUID: "d2"}}
	detectionsLatestEnrich(two, []esetDevice{
		{UUID: "d1", ParentGroupUUID: "aaaaaaaa-1111-4000-8000-000000000001"},
		{UUID: "d2", ParentGroupUUID: "bbbbbbbb-2222-4000-8000-000000000002"},
	}, dup)
	if two[0].Customer != "Acme (aaaaaaaa)" || two[1].Customer != "Acme (bbbbbbbb)" {
		t.Fatalf("duplicate-name customers = %q / %q", two[0].Customer, two[1].Customer)
	}
}

func TestDetectionsLatestCloudWarning(t *testing.T) {
	forbidden := &client.APIError{Method: "GET", Path: "/v2/detections", StatusCode: 403}
	if msg := detectionsLatestCloudWarning(forbidden); !strings.Contains(msg, "HTTP 403") || !strings.Contains(msg, "Cloud Office Security") {
		t.Fatalf("4xx warning = %q", msg)
	}
	wrapped := detectionsLatestCloudWarning(errors.Join(errors.New("ctx"), forbidden))
	if !strings.Contains(wrapped, "HTTP 403") {
		t.Fatalf("wrapped 4xx must still classify: %q", wrapped)
	}
	if msg := detectionsLatestCloudWarning(errors.New("dial tcp: i/o timeout")); !strings.Contains(msg, "could not be fetched") {
		t.Fatalf("network warning = %q", msg)
	}
}

func TestDetectionsLatestDryRunAndBadInput(t *testing.T) {
	testenv.Isolate(t)
	out, err := fleetStaleTestRun(t, "detections", "latest", "--dry-run", "--json")
	if err != nil || !strings.Contains(out, `"action":"detections latest"`) {
		t.Fatalf("dry-run = %q, %v", out, err)
	}
	for _, bad := range [][]string{{"--min-severity", "critical"}, {"--since", "-3h"}, {"--limit", "0"}, {"--max-pages", "0"}} {
		var stderr bytes.Buffer
		cmd := RootCmd()
		cmd.SetOut(&stderr)
		cmd.SetErr(&stderr)
		cmd.SetArgs(append([]string{"detections", "latest", "--json"}, bad...))
		if err := cmd.Execute(); err == nil {
			t.Fatalf("%v must be a usage error before any network call", bad)
		}
	}
	// A device name with no mirror cannot be resolved; a usage error, not a live call.
	if _, err := fleetStaleTestRun(t, "detections", "latest", "--device", "FRONTDESK-01", "--json"); err == nil || !strings.Contains(err.Error(), "not a UUID") {
		t.Fatalf("unresolvable --device name = %v", err)
	}
}

// TestDetectionsLatestCommandCountsRejectedRows runs the real command against
// a stand-in ESET host: a returned detection with a wrongly typed field is
// counted in rejected_rows and warned about once, while the good rows list.
func TestDetectionsLatestCommandCountsRejectedRows(t *testing.T) {
	srv := reportVulnsNewServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/detections" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(`{"detections":[
		  {"uuid":"e1000001-0000-4000-8000-000000000001","displayName":"Win32/Filecoder.WannaCryptor.D","severityLevel":"SEVERITY_LEVEL_HIGH",
		   "occurTime":"2026-10-14T08:15:30Z","context":{"deviceUuid":"` + triageTestFront + `"}},
		  {"uuid":"e1000002-0000-4000-8000-000000000002","displayName":"Typed wrong","severityLevel":["HIGH"],"occurTime":"2026-10-14T08:00:00Z"},
		  {"displayName":"no uuid","occurTime":"2026-10-14T07:00:00Z"}],"nextPageToken":""}`))
	})
	out, stderr, err := reportVulnsRunCLI(t, srv, "detections", "latest", "--since", "7d", "--db", filepath.Join(t.TempDir(), "absent.db"), "--json")
	if err != nil {
		t.Fatalf("run: %v\n%s", err, stderr)
	}
	var v detectionsLatestView
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatalf("bad JSON: %v\n%s", err, out)
	}
	if v.TotalFetched != 3 || v.RejectedRows != 2 || len(v.Items) != 1 || v.Items[0].UUID != "e1000001-0000-4000-8000-000000000001" {
		t.Fatalf("view = %+v", v)
	}
	if strings.Count(stderr, "could not be decoded") != 1 || !strings.Contains(stderr, "warning: 2 row(s)") {
		t.Fatalf("want one rejects warning, stderr = %q", stderr)
	}
}

// TestDetectionsLatestEmptyDeviceMirrorEndToEnd runs the Codex round-4
// scenario through the command: a synced COMPANY tree, a completed devices
// sync that holds no device, and a live detection on a deleted endpoint.
func TestDetectionsLatestEmptyDeviceMirrorEndToEnd(t *testing.T) {
	srv := reportVulnsNewServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/detections" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(`{"detections":[{"uuid":"e1000001-0000-4000-8000-000000000001","displayName":"Win32/Agent","severityLevel":"SEVERITY_LEVEL_HIGH",
		   "occurTime":"2026-10-14T08:15:30Z","context":{"deviceUuid":"` + triageTestFront + `"}}],"nextPageToken":""}`))
	})
	db := exposureSeedDB(t, map[string][]map[string]any{"device-groups": exposureFixtureGroups(), "devices": {}})
	out, stderr, err := reportVulnsRunCLI(t, srv, "detections", "latest", "--since", "3650d", "--db", db, "--json")
	if err != nil {
		t.Fatalf("run: %v\n%s", err, stderr)
	}
	var v detectionsLatestView
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatalf("bad JSON: %v\n%s", err, out)
	}
	if len(v.Items) != 1 || v.Items[0].Customer != detectionsSpreadUnknownDevice || v.UnattributedDetections != 1 {
		t.Fatalf("view = %+v; want the detection on the deleted endpoint labelled %q and counted", v, detectionsSpreadUnknownDevice)
	}
	if !strings.Contains(v.SyncedHint, "no devices in the local mirror") {
		t.Fatalf("synced_hint = %q; want it to say the device mirror is empty", v.SyncedHint)
	}
}
