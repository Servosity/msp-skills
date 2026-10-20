// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored. `triage --customer` can only keep a row whose device the
// mirror places under that customer. Incident list rows usually carry no
// deviceUuids, so a customer-scoped queue used to print "Nothing to triage"
// while an open, high-severity incident sat unattributed.
package cli

import (
	"strings"
	"testing"
	"time"

	"eset-protect-pp-cli/internal/cliutil/testenv"
)

func TestTriageCustomerCountsUnattributedIncidents(t *testing.T) {
	testenv.Isolate(t)
	resources := []string{"device-groups", "devices", "detections", "incidents"}
	// Only incidents: one with no device (the list-row shape), one naming a
	// device the mirror does not know, one on another customer's device.
	incidents := []map[string]any{
		{"uuid": "f3000001-0000-4000-8000-000000000001", "displayName": "Ransomware behavior", "severity": "INCIDENT_SEVERITY_LEVEL_HIGH",
			"status": "INCIDENT_STATUS_OPEN", "createTime": fleetStaleTestAgo(2 * time.Hour)},
		{"uuid": "f3000002-0000-4000-8000-000000000002", "displayName": "Lateral movement", "severity": "INCIDENT_SEVERITY_LEVEL_MEDIUM",
			"status": "INCIDENT_STATUS_OPEN", "createTime": fleetStaleTestAgo(3 * time.Hour), "deviceUuids": []string{"d0000099-aaaa-4bbb-8ccc-000000000099"}},
		{"uuid": "f3000003-0000-4000-8000-000000000003", "displayName": "Bright Smiles only", "severity": "INCIDENT_SEVERITY_LEVEL_HIGH",
			"status": "INCIDENT_STATUS_OPEN", "createTime": fleetStaleTestAgo(time.Hour), "deviceUuids": []string{triageTestBright}},
	}
	detections := []map[string]any{
		{"uuid": "e3000001-0000-4000-8000-000000000001", "displayName": "PUA on an unsynced device", "severityLevel": "SEVERITY_LEVEL_HIGH",
			"occurTime": fleetStaleTestAgo(time.Hour), "context": map[string]any{"deviceUuid": "d0000098-aaaa-4bbb-8ccc-000000000098"}},
		{"uuid": "e3000002-0000-4000-8000-000000000002", "displayName": "Bright Smiles detection", "severityLevel": "SEVERITY_LEVEL_HIGH",
			"occurTime": fleetStaleTestAgo(time.Hour), "context": map[string]any{"deviceUuid": triageTestBright}},
	}
	path := fleetStaleTestSeedDB(t, map[string][]map[string]any{
		"device-groups": fleetStaleTestGroups(),
		"devices":       fleetStaleTestDevices(),
		"detections":    detections,
		"incidents":     incidents,
	})
	mirrorInterrupt(t, path, resources)

	out, err := fleetStaleTestRun(t, "triage", "--customer", "Acme Dental", "--db", path, "--json")
	if err != nil {
		t.Fatalf("triage: %v\n%s", err, out)
	}
	v := triageTestDecode(t, out)
	if len(v.Items) != 0 {
		t.Fatalf("no row is Acme Dental's: %s", triageTestUUIDs(v))
	}
	if v.UnattributedIncidents != 2 || strings.Join(v.UnattributedIncidentUUIDs, ",") != "f3000001-0000-4000-8000-000000000001,f3000002-0000-4000-8000-000000000002" {
		t.Fatalf("unattributed incidents = %d %v, want the deviceless and the unknown-device incident", v.UnattributedIncidents, v.UnattributedIncidentUUIDs)
	}
	if v.UnattributedDetections != 1 || strings.Join(v.UnattributedDetectionUUIDs, ",") != "e3000001-0000-4000-8000-000000000001" {
		t.Fatalf("unattributed detections = %d %v", v.UnattributedDetections, v.UnattributedDetectionUUIDs)
	}

	human, err := fleetStaleTestRun(t, "triage", "--customer", "Acme Dental", "--db", path, "--human-friendly")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(human, "Nothing to triage") {
		t.Fatalf("an unattributed open incident must never read as a clear queue:\n%s", human)
	}
	if !strings.Contains(human, "2 open incident(s) name no device") || !strings.Contains(human, "f3000001-0000-4000-8000-000000000001") ||
		!strings.Contains(human, "1 detection(s) are on a device the mirror cannot tie to a customer") {
		t.Fatalf("human output must name the unattributed rows:\n%s", human)
	}

	// With rows for the customer, the unattributed line still prints.
	human, err = fleetStaleTestRun(t, "triage", "--customer", "Bright Smiles Ortho", "--db", path, "--human-friendly")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(human, "f3000003") || !strings.Contains(human, "2 open incident(s) name no device") {
		t.Fatalf("a non-empty queue must still name the unattributed incidents:\n%s", human)
	}

	// Without --customer nothing is filtered, so every row is listed, and
	// (round 3, adversarial P2) the owner-unknown ones are still counted,
	// with uuids, and labelled instead of carrying a blank customer.
	out, err = fleetStaleTestRun(t, "triage", "--db", path, "--json")
	if err != nil {
		t.Fatal(err)
	}
	v = triageTestDecode(t, out)
	if v.UnattributedIncidents != 2 || v.UnattributedDetections != 1 || len(v.Items) != 5 ||
		len(v.UnattributedIncidentUUIDs) != 2 || len(v.UnattributedDetectionUUIDs) != 1 {
		t.Fatalf("no --customer: unattributed %d/%d, items %s", v.UnattributedIncidents, v.UnattributedDetections, triageTestUUIDs(v))
	}
	for _, it := range v.Items {
		if it.Customer == "" {
			t.Fatalf("an owner-unknown row must be labelled, not blank: %+v", it)
		}
	}
	human, err = fleetStaleTestRun(t, "triage", "--db", path, "--human-friendly")
	if err != nil || !strings.Contains(human, "2 open incident(s) in the queue name no device") || !strings.Contains(human, "1 detection(s) in the queue are on a device") {
		t.Fatalf("fleet-wide human queue must name the owner-unknown rows: %v\n%s", err, human)
	}
}
