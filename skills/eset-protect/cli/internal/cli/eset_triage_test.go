// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored. `triage` ranking, windowing and filters.
package cli

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"eset-protect-pp-cli/internal/cliutil/testenv"
)

const (
	triageTestFront  = "d0000001-aaaa-4bbb-8ccc-000000000001"
	triageTestBright = "d0000005-aaaa-4bbb-8ccc-000000000005"
)

func triageTestDetections() []map[string]any {
	return []map[string]any{
		{"uuid": "e1000001-0000-4000-8000-000000000001", "displayName": "Win32/Filecoder.WannaCryptor.D", "severityLevel": "SEVERITY_LEVEL_HIGH",
			"occurTime": fleetStaleTestAgo(time.Hour), "objectHashSha1": "3395856CE81F2B7382DEE72602F798B642F14140", "context": map[string]any{"deviceUuid": triageTestFront}},
		{"uuid": "e1000002-0000-4000-8000-000000000002", "typeName": "Trojan", "objectName": `C:\Users\frontdesk\Downloads\invoice.exe`, "severityLevel": "SEVERITY_LEVEL_MEDIUM",
			"occurTime": fleetStaleTestAgo(3 * time.Hour), "context": map[string]any{"deviceUuid": triageTestBright}},
		{"uuid": "e1000003-0000-4000-8000-000000000003", "displayName": "Old HIGH outside window", "severityLevel": "SEVERITY_LEVEL_HIGH",
			"occurTime": fleetStaleTestAgo(3 * fleetStaleTestDay), "context": map[string]any{"deviceUuid": triageTestFront}},
		{"uuid": "e1000004-0000-4000-8000-000000000004", "displayName": "Informational noise", "severityLevel": "SEVERITY_LEVEL_INFORMATIONAL",
			"occurTime": fleetStaleTestAgo(30 * time.Minute), "context": map[string]any{"deviceUuid": triageTestFront}},
		{"uuid": "e1000005-0000-4000-8000-000000000005", "displayName": "PUA on an unsynced device", "severityLevel": "SEVERITY_LEVEL_LOW",
			"occurTime": fleetStaleTestAgo(2 * time.Hour), "context": map[string]any{"deviceUuid": "d0000099-aaaa-4bbb-8ccc-000000000099"}},
	}
}

func triageTestIncidents() []map[string]any {
	return []map[string]any{
		{"uuid": "f2000001-0000-4000-8000-000000000001", "displayName": "Ransomware behavior on FRONTDESK-01", "severity": "INCIDENT_SEVERITY_LEVEL_HIGH",
			"status": "INCIDENT_STATUS_OPEN", "createTime": fleetStaleTestAgo(5 * time.Hour), "deviceUuids": []string{}},
		{"uuid": "f2000002-0000-4000-8000-000000000002", "displayName": "Closed already", "severity": "INCIDENT_SEVERITY_LEVEL_HIGH",
			"status": "INCIDENT_STATUS_CLOSED", "createTime": fleetStaleTestAgo(2 * time.Hour), "updateTime": fleetStaleTestAgo(time.Hour)},
		{"uuid": "f2000003-0000-4000-8000-000000000003", "displayName": "Lingering investigation", "severity": "INCIDENT_SEVERITY_LEVEL_MEDIUM",
			"status": "INCIDENT_STATUS_IN_PROGRESS", "createTime": fleetStaleTestAgo(10 * fleetStaleTestDay), "updateTime": fleetStaleTestAgo(9 * fleetStaleTestDay)},
		{"uuid": "f2000004-0000-4000-8000-000000000004", "displayName": "Waiting on the customer", "severity": "INCIDENT_SEVERITY_LEVEL_LOW",
			"status": "INCIDENT_STATUS_WAITING_FOR_INPUT", "createTime": fleetStaleTestAgo(10 * fleetStaleTestDay), "updateTime": fleetStaleTestAgo(90 * time.Minute),
			"deviceUuids": []string{triageTestFront, triageTestBright}},
		{"uuid": "f2000005-0000-4000-8000-000000000005", "displayName": "Unlabelled incident", "status": "INCIDENT_STATUS_OPEN", "createTime": fleetStaleTestAgo(time.Hour)},
	}
}

func triageTestDecode(t *testing.T, out string) triageView {
	t.Helper()
	var v triageView
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatalf("output is not triage JSON: %v\n%s", err, out)
	}
	return v
}

func triageTestUUIDs(v triageView) string {
	ids := make([]string, 0, len(v.Items))
	for _, it := range v.Items {
		ids = append(ids, it.UUID[:8])
	}
	return strings.Join(ids, ",")
}

func TestTriageEndToEnd(t *testing.T) {
	testenv.Isolate(t)
	path := fleetStaleTestSeedDB(t, map[string][]map[string]any{
		"device-groups": fleetStaleTestGroups(),
		"devices":       fleetStaleTestDevices(),
		"detections":    triageTestDetections(),
		"incidents":     triageTestIncidents(),
	})
	out, err := fleetStaleTestRun(t, "triage", "--db", path, "--json")
	if err != nil {
		t.Fatalf("triage: %v\n%s", err, out)
	}
	v := triageTestDecode(t, out)
	// high (newest first), medium, low (newest first), then unlabelled.
	if got, want := triageTestUUIDs(v), "e1000001,f2000001,e1000002,f2000004,e1000005,f2000005"; got != want {
		t.Fatalf("order = %s, want %s", got, want)
	}
	if v.Total != 6 || v.Counts.Detections != 3 || v.Counts.Incidents != 3 || v.Since == "" || v.RejectedRows != 0 {
		t.Fatalf("totals = %+v", v)
	}
	if !strings.Contains(out, `"rejected_rows": 0`) {
		t.Fatalf("rejected_rows must always be present:\n%s", out)
	}
	byID := map[string]triageItem{}
	for _, it := range v.Items {
		byID[it.UUID[:8]] = it
	}
	if it := byID["e1000001"]; it.Kind != "detection" || it.Severity != "high" || it.Device != "FRONTDESK-01" || it.Customer != "Acme Dental" ||
		it.Next != "eset-protect-cli detections spread --sha1 3395856ce81f2b7382dee72602f798b642f14140" {
		t.Fatalf("hashed detection row = %+v", it)
	}
	if it := byID["e1000002"]; it.Title != `Trojan: C:\Users\frontdesk\Downloads\invoice.exe` || it.Customer != "Bright Smiles Ortho" ||
		it.Next != "eset-protect-cli detections get e1000002-0000-4000-8000-000000000002" {
		t.Fatalf("unhashed detection row = %+v", it)
	}
	if it := byID["e1000005"]; it.Device != "d0000099-aaaa-4bbb-8ccc-000000000099" || it.Customer != detectionsSpreadUnknownDevice {
		t.Fatalf("unsynced device should show its UUID and the owner-unknown label: %+v", it)
	}
	if it := byID["f2000001"]; it.Kind != "incident" || it.Device != "" || it.Customer != triageOwnerUnknown || it.Status != "open" ||
		it.Next != "eset-protect-cli incidents context f2000001-0000-4000-8000-000000000001" {
		t.Fatalf("deviceless incident row = %+v", it)
	}
	if it := byID["f2000004"]; it.Device != "FRONTDESK-01 (+1 more)" || it.Customer != "Acme Dental, Bright Smiles Ortho" || it.Status != "waiting_for_input" || it.Severity != "low" {
		t.Fatalf("multi-device incident row = %+v", it)
	}
	if it := byID["f2000005"]; it.Severity != "unknown" {
		t.Fatalf("unlabelled incident severity = %q", it.Severity)
	}

	cases := []struct {
		name  string
		args  []string
		want  string
		total int
	}{
		{"medium floor drops low and unlabelled", []string{"--min-severity", "medium"}, "e1000001,f2000001,e1000002", 3},
		{"stale incidents kept on request", []string{"--include-stale-incidents"}, "e1000001,f2000001,e1000002,f2000003,f2000004,e1000005,f2000005", 7},
		{"customer filter needs an attributed device", []string{"--customer", "Acme Dental"}, "e1000001,f2000004", 2},
		{"limit keeps total", []string{"--limit", "2"}, "e1000001,f2000001", 6},
		{"wider window adds the old detection", []string{"--since", "4d"}, "e1000001,f2000001,e1000003,e1000002,f2000004,e1000005,f2000005", 7},
		{"info floor adds informational", []string{"--min-severity", "info"}, "e1000001,f2000001,e1000002,f2000004,e1000005,f2000005,e1000004", 7},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := fleetStaleTestRun(t, append([]string{"triage", "--db", path, "--json"}, tc.args...)...)
			if err != nil {
				t.Fatalf("%v\n%s", err, out)
			}
			v := triageTestDecode(t, out)
			if got := triageTestUUIDs(v); got != tc.want {
				t.Fatalf("items = %s, want %s", got, tc.want)
			}
			if v.Total != tc.total {
				t.Fatalf("total = %d, want %d", v.Total, tc.total)
			}
		})
	}

	for _, bad := range [][]string{{"--min-severity", "critical"}, {"--since", "soon"}, {"--limit", "0"}} {
		if _, err := fleetStaleTestRun(t, append([]string{"triage", "--db", path, "--json"}, bad...)...); err == nil {
			t.Fatalf("%v must be a usage error", bad)
		}
	}
}

func TestTriageMissingMirrorIsEmptyJSON(t *testing.T) {
	testenv.Isolate(t)
	out, err := fleetStaleTestRun(t, "triage", "--db", filepath.Join(t.TempDir(), "absent.db"), "--json")
	if err != nil {
		t.Fatalf("missing mirror must exit 0: %v", err)
	}
	if !strings.Contains(out, `"items": []`) || !strings.Contains(out, `"rejected_rows": 0`) {
		t.Fatalf("want an empty items array and rejected_rows 0, got:\n%s", out)
	}
}

func TestTriageBuildEmptyAndMalformed(t *testing.T) {
	var rej esetRejects
	v := triageBuild(nil, nil, nil, newEsetTree(nil), triageOptions{Start: time.Now().Add(-24 * time.Hour), MinSeverity: "low"}, &rej)
	if v.Items == nil || len(v.Items) != 0 || v.Total != 0 || v.RejectedRows != 0 || rej.N != 0 {
		t.Fatalf("empty input must give an empty, non-nil slice and no rejects: %+v", v)
	}
	junk := []json.RawMessage{json.RawMessage(`not json`), json.RawMessage(`{"displayName":"no uuid"}`),
		json.RawMessage(`{"uuid":"x","severityLevel":"SEVERITY_LEVEL_HIGH","occurTime":"yesterday-ish"}`)}
	v = triageBuild(junk, junk[:2], nil, newEsetTree(nil), triageOptions{Start: time.Now().Add(-24 * time.Hour), MinSeverity: "low"}, &rej)
	if len(v.Items) != 0 {
		t.Fatalf("malformed or undated rows must not be listed: %+v", v.Items)
	}
	// 3 detections (bad JSON, no uuid, bad occurTime) + 2 incidents (bad JSON, no uuid).
	if v.RejectedRows != 5 || rej.N != 5 || !strings.HasPrefix(rej.First, "detection row 0:") {
		t.Fatalf("malformed rows must be counted, not dropped: view %d, rejects %+v", v.RejectedRows, rej)
	}
}

// TestTriageRejectsWronglyTypedRows: a stored row whose field has the wrong
// type used to vanish from the queue with no signal. It must be counted in
// rejected_rows and warned about once on stderr, while good rows still list.
func TestTriageRejectsWronglyTypedRows(t *testing.T) {
	testenv.Isolate(t)
	dets := triageTestDetections()
	dets = append(dets,
		map[string]any{"uuid": "e1000009-0000-4000-8000-000000000009", "displayName": "numeric time", "severityLevel": "SEVERITY_LEVEL_HIGH",
			"occurTime": 1760430000, "context": map[string]any{"deviceUuid": triageTestFront}},
		map[string]any{"uuid": "e100000a-0000-4000-8000-00000000000a", "displayName": "numeric severity", "severityLevel": 5,
			"occurTime": fleetStaleTestAgo(time.Hour), "context": map[string]any{"deviceUuid": triageTestFront}},
	)
	incs := triageTestIncidents()
	incs = append(incs, map[string]any{"uuid": "f2000009-0000-4000-8000-000000000009", "displayName": "device list is a string",
		"severity": "INCIDENT_SEVERITY_LEVEL_HIGH", "status": "INCIDENT_STATUS_OPEN", "createTime": fleetStaleTestAgo(time.Hour), "deviceUuids": "FRONTDESK-01"})
	path := fleetStaleTestSeedDB(t, map[string][]map[string]any{
		"device-groups": fleetStaleTestGroups(),
		"devices":       fleetStaleTestDevices(),
		"detections":    dets,
		"incidents":     incs,
	})
	cmd := RootCmd()
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{"triage", "--db", path, "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("triage: %v\n%s", err, errOut.String())
	}
	v := triageTestDecode(t, out.String())
	if v.RejectedRows != 3 {
		t.Fatalf("rejected_rows = %d, want 3\n%s", v.RejectedRows, out.String())
	}
	if got, want := triageTestUUIDs(v), "e1000001,f2000001,e1000002,f2000004,e1000005,f2000005"; got != want {
		t.Fatalf("good rows must still list: %s, want %s", got, want)
	}
	if n := strings.Count(errOut.String(), "could not be decoded"); n != 1 || !strings.Contains(errOut.String(), "warning: 3 row(s)") {
		t.Fatalf("want exactly one rejects warning on stderr, got:\n%s", errOut.String())
	}
}

// TestTriageCustomersSharingANameStayDistinct: an incident spanning two
// different customers that are both called "Acme" lists both, told apart by
// a UUID prefix, instead of collapsing to one "Acme".
func TestTriageCustomersSharingANameStayDistinct(t *testing.T) {
	const acme1, acme2 = "aaaaaaaa-1111-4000-8000-000000000001", "bbbbbbbb-2222-4000-8000-000000000002"
	tree := newEsetTree([]esetGroup{
		{UUID: "msp", Name: "Servosity MSP", EntityType: "DEVICE_GROUP_ENTITY_TYPE_MSP"},
		{UUID: acme1, Name: "Acme", ParentUUID: "msp", EntityType: esetCompanyEntityType},
		{UUID: acme2, Name: "Acme", ParentUUID: "msp", EntityType: esetCompanyEntityType},
	})
	devices := []esetDevice{{UUID: "d1", DisplayName: "ACME1-PC", ParentGroupUUID: acme1}, {UUID: "d2", DisplayName: "ACME2-PC", ParentGroupUUID: acme2}}
	const incID, detID = "f2000010-0000-4000-8000-000000000010", "e1000010-0000-4000-8000-000000000010"
	inc, _ := json.Marshal(map[string]any{"uuid": incID, "displayName": "Spread", "severity": "INCIDENT_SEVERITY_LEVEL_HIGH",
		"status": "INCIDENT_STATUS_OPEN", "createTime": fleetStaleTestAgo(time.Hour), "deviceUuids": []string{"d1", "d2"}})
	det, _ := json.Marshal(map[string]any{"uuid": detID, "displayName": "Trojan", "severityLevel": "SEVERITY_LEVEL_HIGH",
		"occurTime": fleetStaleTestAgo(time.Hour), "context": map[string]any{"deviceUuid": "d2"}})
	v := triageBuild([]json.RawMessage{det}, []json.RawMessage{inc}, devices, tree, triageOptions{Start: time.Now().Add(-24 * time.Hour), MinSeverity: "low"}, nil)
	byID := map[string]triageItem{}
	for _, it := range v.Items {
		byID[it.UUID] = it
	}
	if got := byID[incID].Customer; got != "Acme (aaaaaaaa), Acme (bbbbbbbb)" {
		t.Fatalf("incident customers = %q", got)
	}
	if got := byID[detID].Customer; got != "Acme (bbbbbbbb)" {
		t.Fatalf("detection customer = %q", got)
	}
	g := esetGroup{UUID: acme1, Name: "Acme", EntityType: esetCompanyEntityType}
	only := triageBuild([]json.RawMessage{det}, []json.RawMessage{inc}, devices, tree, triageOptions{Start: time.Now().Add(-24 * time.Hour), MinSeverity: "low", Customer: &g}, nil)
	if triageTestUUIDs(only) != "f2000010" {
		t.Fatalf("--customer by UUID must keep only rows touching that customer: %s", triageTestUUIDs(only))
	}
}

// TestTriageHelpHasNoResolvedClaim: v1 detections have no resolved state, so
// the help must not promise "unresolved" detections.
func TestTriageHelpHasNoResolvedClaim(t *testing.T) {
	testenv.Isolate(t)
	out, err := fleetStaleTestRun(t, "triage", "--help")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.ToLower(out), "unresolved") || !strings.Contains(out, "v1 has no") {
		t.Fatalf("triage help must not claim unresolved detections:\n%s", out)
	}
}

func TestTriageSeverityScale(t *testing.T) {
	cases := map[string]string{
		"SEVERITY_LEVEL_HIGH":              "high",
		"INCIDENT_SEVERITY_LEVEL_HIGH":     "high",
		"INCIDENT_SEVERITY_LEVEL_MEDIUM":   "medium",
		"SEVERITY_LEVEL_LOW":               "low",
		"SEVERITY_LEVEL_INFORMATIONAL":     "info",
		"SEVERITY_LEVEL_DIAGNOSTIC":        "diagnostic",
		"SEVERITY_LEVEL_UNSPECIFIED":       "unknown",
		"INCIDENT_SEVERITY_LEVEL_CRITICAL": "unknown",
		"":                                 "unknown",
	}
	for in, want := range cases {
		if got := triageSeverityShort(in); got != want {
			t.Errorf("triageSeverityShort(%q) = %q, want %q", in, got, want)
		}
	}
	if !triageSeverityPasses("unknown", "low") || triageSeverityPasses("unknown", "medium") {
		t.Error("unknown severity must pass a low floor and fail a medium one")
	}
	if triageSeverityPasses("info", "low") || !triageSeverityPasses("high", "medium") || !triageSeverityPasses("diagnostic", "") {
		t.Error("severity floor ordering is wrong")
	}
	if got, err := triageParseMinSeverity("Informational"); err != nil || got != "info" {
		t.Errorf("informational alias = %q, %v", got, err)
	}
	if _, err := triageParseMinSeverity("critical"); err == nil {
		t.Error("unknown floor must be rejected")
	}
}
