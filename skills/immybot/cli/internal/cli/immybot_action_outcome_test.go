// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Behavioural acceptance for the maintenance-action outcome, on rows shaped like
// the vendor's GetMaintenanceActionResponse (sources/swagger.json):
//   - status/statusName is MaintenanceActionStatus, the lifecycle (NotStarted ...
//     Complete, Skipped, PendingExecution). It never says Failed or Success.
//   - result/resultName is MaintenanceActionResult, the outcome: Pending=0,
//     Success=1, Failed=2, Cancelled=3, Indeterminable=4, Resolved=5, sent as the
//     string or the integer.
//   - resultReasonMessage is the cause; reason is MaintenanceActionReason (why the
//     action was planned, e.g. UpdateAvailable), not a failure cause.
// Every command that reads an action outcome must count these rows correctly.

package cli

import (
	"strings"
	"testing"
	"time"
)

// specActionRows is one computer-and-assignment fleet in the vendor's documented
// shape: two string-form failures, one integer-form failure with the readOnly
// name fields null, one cancellation, one success and one still running.
func specActionRows(now string) []map[string]any {
	return []map[string]any{
		{"id": 1, "maintenanceSessionId": 501, "computerId": 42, "computerName": "WS-42", "tenantId": 7, "tenantName": "Contoso",
			"assignmentId": 9001, "assignmentType": "Local", "maintenanceDisplayName": "Install Chrome",
			"actionType": "Install", "actionTypeName": "Install", "status": "Complete", "statusName": "Complete",
			"result": "Failed", "resultName": "Failed", "reason": "SoftwareMissing", "resultReason": "DetectionActionFailed",
			"resultReasonMessage": "Installer failed on host 11111111-2222-3333-4444-555555555555",
			"startTime":           now, "endTime": now},
		{"id": 2, "maintenanceSessionId": 502, "computerId": 43, "computerName": "WS-43", "tenantId": 8, "tenantName": "Fabrikam",
			"assignmentId": 9001, "assignmentType": "Local", "maintenanceDisplayName": "Install Chrome",
			"actionType": "Install", "actionTypeName": "Install", "status": "Complete", "statusName": "Complete",
			"result": "Failed", "resultName": "Failed", "reason": "SoftwareMissing", "resultReason": "DetectionActionFailed",
			"resultReasonMessage": "Installer failed on host 99999999-8888-7777-6666-555555555555",
			"startTime":           now, "endTime": now},
		{"id": 3, "maintenanceSessionId": 503, "computerId": 42, "computerName": "WS-42", "tenantId": 7, "tenantName": "Contoso",
			"assignmentId": 9002, "assignmentType": "Local", "maintenanceDisplayName": "Patch Windows",
			"actionType": 2, "actionTypeName": nil, "status": 4, "statusName": nil,
			"result": 2, "resultName": nil, "reason": 1, "resultReasonMessage": "Reboot pending",
			"startTime": now, "endTime": now},
		{"id": 4, "maintenanceSessionId": 504, "computerId": 44, "computerName": "WS-44", "tenantId": 7, "tenantName": "Contoso",
			"assignmentId": 9002, "assignmentType": "Local", "maintenanceDisplayName": "Patch Windows",
			"actionType": "Update", "status": "Complete", "statusName": "Complete",
			"result": 3, "resultName": "Cancelled", "reason": "UpdateAvailable", "resultReasonMessage": "Cancelled by technician",
			"startTime": now, "endTime": now},
		{"id": 5, "maintenanceSessionId": 505, "computerId": 42, "computerName": "WS-42", "tenantId": 7, "tenantName": "Contoso",
			"assignmentId": 9001, "assignmentType": "Local", "maintenanceDisplayName": "Install Chrome",
			"actionType": "Install", "status": "Complete", "statusName": "Complete",
			"result": "Success", "resultName": "Success", "reason": "SoftwareMissing",
			"startTime": now, "endTime": now},
		{"id": 6, "maintenanceSessionId": 506, "computerId": 45, "computerName": "WS-45", "tenantId": 7, "tenantName": "Contoso",
			"assignmentId": 9003, "assignmentType": "Local", "maintenanceDisplayName": "Install 7-Zip",
			"actionType": "Install", "status": "PerformingAction", "statusName": "PerformingAction",
			"result": "Pending", "resultName": "Pending", "reason": "SoftwareMissing",
			"startTime": now},
	}
}

func TestSessionTriageReadsSpecShapedOutcome(t *testing.T) {
	now := time.Now().UTC().Format(time.RFC3339)
	db := seedStore(t, map[string][]map[string]any{"maintenance-actions": specActionRows(now)})

	out, err := runCLI(t, "session-triage", "--since", "24h", "--json", "--db", db)
	if err != nil {
		t.Fatalf("session-triage: %v\n%s", err, out)
	}
	v := decode[triageView](t, out)
	// Failed (string), Failed (string), 2 (integer form, names null), Cancelled.
	if v.FailedActions != 4 {
		t.Fatalf("failed_actions = %d, want 4 (result/resultName carry the outcome, statusName is the lifecycle):\n%s", v.FailedActions, out)
	}
	if len(v.Clusters) != 3 {
		t.Fatalf("clusters = %d, want 3 (two Chrome failures differ only by host GUID and must collapse):\n%s", len(v.Clusters), out)
	}
	top := v.Clusters[0]
	if top.Action != "Install Chrome" || top.ComputerCount != 2 || top.TenantCount != 2 {
		t.Fatalf("top cluster = %+v, want Install Chrome across 2 computers in 2 tenants", top)
	}
	if !strings.Contains(top.Reason, "Installer failed") || strings.Contains(top.Reason, "SoftwareMissing") {
		t.Fatalf("cluster reason = %q, want the resultReasonMessage cause, not the planning reason", top.Reason)
	}
	if !strings.EqualFold(top.Status, "Failed") {
		t.Fatalf("cluster status = %q, want the outcome Failed (not the lifecycle Complete)", top.Status)
	}
	foundIntForm := false
	for _, c := range v.Clusters {
		if c.Action == "Patch Windows" && strings.Contains(c.Reason, "Reboot pending") {
			foundIntForm = true
			if !strings.EqualFold(c.Status, "Failed") {
				t.Fatalf("integer-form result 2 must read as Failed, got status %q", c.Status)
			}
		}
	}
	if !foundIntForm {
		t.Fatalf("integer-form failure (result: 2, resultName: null) was not counted:\n%s", out)
	}
}

func TestSessionTriageStatusFilterMatchesOutcomeAndLifecycle(t *testing.T) {
	now := time.Now().UTC().Format(time.RFC3339)
	db := seedStore(t, map[string][]map[string]any{"maintenance-actions": specActionRows(now)})

	out, err := runCLI(t, "session-triage", "--status", "Cancelled", "--json", "--db", db)
	if err != nil {
		t.Fatalf("session-triage --status Cancelled: %v\n%s", err, out)
	}
	// Cancelled is a failure outcome, so the one action it matches is also failed.
	if v := decode[countTriageView](t, out); v.MatchedActions != 1 || v.FailedActions != 1 {
		t.Fatalf("--status Cancelled matched %d / failed %d actions, want 1 / 1 (the resultName outcome):\n%s", v.MatchedActions, v.FailedActions, out)
	}
	out, err = runCLI(t, "session-triage", "--status", "PerformingAction", "--json", "--db", db)
	if err != nil {
		t.Fatalf("session-triage --status PerformingAction: %v\n%s", err, out)
	}
	// A still-running action matches its lifecycle, and is not a failure.
	if v := decode[countTriageView](t, out); v.MatchedActions != 1 || v.FailedActions != 0 {
		t.Fatalf("--status PerformingAction matched %d / failed %d actions, want 1 / 0 (the lifecycle statusName stays a filter signal):\n%s",
			v.MatchedActions, v.FailedActions, out)
	}
}

func TestSessionTriageKeepsLegacyStatusNameSignal(t *testing.T) {
	now := time.Now().UTC().Format(time.RFC3339)
	db := seedStore(t, map[string][]map[string]any{
		"maintenance-actions": {
			// A row whose only failure signal is statusName must still count: a widened
			// detector keeps every signal it already had.
			{"id": 1, "maintenanceDisplayName": "Install Chrome", "statusName": "Failed", "computerName": "WS-01",
				"tenantName": "Contoso", "maintenanceSessionId": 1, "startTime": now},
		},
	})
	out, err := runCLI(t, "session-triage", "--since", "24h", "--json", "--db", db)
	if err != nil {
		t.Fatalf("session-triage: %v\n%s", err, out)
	}
	if v := decode[triageView](t, out); v.FailedActions != 1 {
		t.Fatalf("failed_actions = %d, want 1 for a statusName-only failure:\n%s", v.FailedActions, out)
	}
}

func TestDeploymentHealthReadsSpecShapedOutcome(t *testing.T) {
	now := time.Now().UTC().Format(time.RFC3339)
	db := seedStore(t, map[string][]map[string]any{
		"target-assignments": {
			{"id": 9001, "targetName": "Chrome everywhere", "maintenanceIdentifier": "chrome", "tenantId": 7, "databaseType": "Local"},
			{"id": 9002, "targetName": "Windows patches", "maintenanceIdentifier": "wu", "tenantId": 7, "databaseType": "Local"},
			{"id": 9003, "targetName": "7-Zip", "maintenanceIdentifier": "7zip", "tenantId": 7, "databaseType": "Local"},
		},
		"maintenance-actions": specActionRows(now),
	})
	out, err := runCLI(t, "deployment-health", "--json", "--db", db)
	if err != nil {
		t.Fatalf("deployment-health: %v\n%s", err, out)
	}
	v := decode[deploymentHealthView](t, out)
	got := map[string]deploymentHealth{}
	for _, d := range v.Deployments {
		got[d.AssignmentID] = d
	}
	if d := got["9001"]; d.Succeeded != 1 || d.Failed != 2 || d.NeverSucceeded {
		t.Fatalf("assignment 9001 = %+v, want 1 succeeded / 2 failed, not never-succeeded:\n%s", d, out)
	}
	if d := got["9002"]; d.Succeeded != 0 || d.Failed != 2 || !d.NeverSucceeded {
		t.Fatalf("assignment 9002 = %+v, want 0 succeeded / 2 failed (integer form + Cancelled), never-succeeded:\n%s", d, out)
	}
	if d := got["9003"]; d.Failed != 0 {
		t.Fatalf("assignment 9003 = %+v, want 0 failed (a Pending action has not failed):\n%s", d, out)
	}
	out, err = runCLI(t, "deployment-health", "--only-failing", "--json", "--db", db)
	if err != nil {
		t.Fatalf("deployment-health --only-failing: %v\n%s", err, out)
	}
	failing := map[string]bool{}
	for _, d := range decode[deploymentHealthView](t, out).Deployments {
		failing[d.AssignmentID] = true
	}
	if !failing["9001"] || !failing["9002"] {
		t.Fatalf("--only-failing must list 9001 and 9002, got %v:\n%s", failing, out)
	}
}

func TestComputerDossierReadsSpecShapedOutcome(t *testing.T) {
	now := time.Now().UTC().Format(time.RFC3339)
	db := seedStore(t, map[string][]map[string]any{
		"computers": {
			{"id": 42, "name": "WS-42", "tenant": "Contoso", "tenantId": 7, "online": true, "excludeFromMaintenance": false},
		},
		"maintenance-actions": specActionRows(now),
	})
	out, err := runCLI(t, "computer-dossier", "42", "--json", "--db", db)
	if err != nil {
		t.Fatalf("computer-dossier: %v\n%s", err, out)
	}
	v := decode[computerDossierView](t, out)
	if len(v.RecentActions) != 3 {
		t.Fatalf("recent_actions = %d, want 3 for WS-42:\n%s", len(v.RecentActions), out)
	}
	// WS-42: Install Chrome Failed (string), Patch Windows 2 (integer form), Install Chrome Success.
	if v.FailedActions != 2 {
		t.Fatalf("failed_actions = %d, want 2 (result/resultName, string and integer form):\n%s", v.FailedActions, out)
	}
	failedSeen := 0
	for _, a := range v.RecentActions {
		if strings.EqualFold(a.Status, "Complete") {
			t.Fatalf("action status %q is the lifecycle; the dossier must show the outcome:\n%s", a.Status, out)
		}
		if !strings.EqualFold(a.Status, "Failed") {
			continue
		}
		failedSeen++
		if strings.Contains(a.Reason, "SoftwareMissing") || a.Reason == "1" || a.Reason == "" {
			t.Fatalf("failed action reason %q is not the cause; want resultReasonMessage:\n%s", a.Reason, out)
		}
	}
	if failedSeen != 2 {
		t.Fatalf("recent_actions show %d Failed outcomes, want 2:\n%s", failedSeen, out)
	}
}

func TestImmyParseActionOutcome(t *testing.T) {
	tests := []struct {
		name                                   string
		resultName, result, statusName, status string
		failed, succeeded                      bool
		label                                  string
	}{
		{"string result Failed", "Failed", "Failed", "Complete", "Complete", true, false, "Failed"},
		{"integer result 2, names null", "", "2", "", "4", true, false, "Failed"},
		{"integer result as float", "", "2.0", "", "4", true, false, "Failed"},
		{"Cancelled reads as a failure", "Cancelled", "3", "Complete", "Complete", true, false, "Cancelled"},
		{"string result Success", "Success", "Success", "Complete", "Complete", false, true, "Success"},
		{"integer result 1", "", "1", "", "4", false, true, "Success"},
		{"Pending is neither", "Pending", "0", "PerformingAction", "2", false, false, "Pending"},
		{"Indeterminable is neither", "", "4", "", "4", false, false, "Indeterminable"},
		{"lifecycle only, no result", "", "", "Skipped", "7", false, false, "Skipped"},
		{"legacy statusName Failed still counts", "", "", "Failed", "", true, false, "Failed"},
		{"any failure signal wins", "Success", "1", "Failed", "", true, false, "Failed"},
		{"resultName and result disagree: failure wins", "Success", "2", "Complete", "4", true, false, "Failed"},
		{"unknown integer is not a failure", "", "99", "", "", false, false, "99"},
		{"lowercase result name", "failed", "", "complete", "", true, false, "Failed"},
	}
	for _, tc := range tests {
		o := immyParseActionOutcome(tc.resultName, tc.result, tc.statusName, tc.status, "", "", "")
		if o.Failed != tc.failed || o.Succeeded != tc.succeeded || o.Label != tc.label {
			t.Errorf("%s: got failed=%t succeeded=%t label=%q, want failed=%t succeeded=%t label=%q",
				tc.name, o.Failed, o.Succeeded, o.Label, tc.failed, tc.succeeded, tc.label)
		}
	}

	if o := immyParseActionOutcome("Failed", "2", "Complete", "4", "Exit code 1603", "2", "0"); o.Cause != "Exit code 1603" {
		t.Errorf("cause = %q, want resultReasonMessage first", o.Cause)
	}
	if o := immyParseActionOutcome("Failed", "2", "Complete", "4", "", "2", "0"); o.Cause != "DetectionActionFailed" {
		t.Errorf("cause = %q, want the resultReason name when there is no message", o.Cause)
	}
	if o := immyParseActionOutcome("", "2", "", "4", "", "", "1"); o.Cause != "UpdateAvailable" {
		t.Errorf("cause = %q, want the planning reason name as the last fallback", o.Cause)
	}

	o := immyParseActionOutcome("", "3", "", "7", "", "", "")
	for _, want := range []string{"Cancelled", "cancelled", "3", "Skipped", "7"} {
		if !o.Matches(want) {
			t.Errorf("Matches(%q) = false for result 3 / status 7", want)
		}
	}
	if o.Matches("Failed") || o.Matches("") {
		t.Error("Matches must not match an unrelated or empty name")
	}
}

func TestComputerDossierJoinsActionsOnComputerID(t *testing.T) {
	now := time.Now().UTC().Format(time.RFC3339)
	db := seedStore(t, map[string][]map[string]any{
		// Two tenants each own a machine named RECEPTION: names collide across
		// tenants, ids (int32 in the vendor shape) do not.
		"computers": {
			{"id": 42, "name": "RECEPTION", "tenant": "Contoso", "tenantId": 7, "online": true},
			{"id": 77, "name": "RECEPTION", "tenant": "Fabrikam", "tenantId": 8, "online": true},
		},
		"tenants-software-from-inventory-dx": {
			{"id": 1, "computerId": 42, "computerName": "RECEPTION", "softwareName": "Google Chrome", "version": "141.0", "tenantId": 7},
			{"id": 2, "computerId": 77, "computerName": "RECEPTION", "softwareName": "Firefox", "version": "1.0", "tenantId": 8},
		},
		"maintenance-actions": {
			{"id": 1, "computerId": 42, "computerName": "RECEPTION", "tenantId": 7, "maintenanceDisplayName": "Install Chrome",
				"status": "Complete", "statusName": "Complete", "result": "Success", "resultName": "Success", "startTime": now},
			{"id": 2, "computerId": 77, "computerName": "RECEPTION", "tenantId": 8, "maintenanceDisplayName": "Install Firefox",
				"status": "Complete", "statusName": "Complete", "result": "Failed", "resultName": "Failed",
				"resultReasonMessage": "Installer failed", "startTime": now},
			// Renamed since: the action row still carries the old name, the id still joins.
			{"id": 3, "computerId": 42, "computerName": "OLD-NAME", "tenantId": 7, "maintenanceDisplayName": "Patch Windows",
				"status": "Complete", "statusName": "Complete", "result": "Failed", "resultName": "Failed",
				"resultReasonMessage": "Reboot pending", "startTime": now},
		},
	})
	out, err := runCLI(t, "computer-dossier", "42", "--json", "--db", db)
	if err != nil {
		t.Fatalf("computer-dossier: %v\n%s", err, out)
	}
	v := decode[computerDossierView](t, out)
	names := map[string]bool{}
	for _, a := range v.RecentActions {
		names[a.Action] = true
	}
	if len(v.RecentActions) != 2 || !names["Install Chrome"] || !names["Patch Windows"] || v.FailedActions != 1 {
		t.Fatalf("recent_actions = %+v, failed_actions = %d; want Install Chrome and Patch Windows with 1 failure (join on computerId, never on a name another tenant shares):\n%s",
			v.RecentActions, v.FailedActions, out)
	}
	if v.SoftwareCount != 1 || v.Software[0].Name != "Google Chrome" {
		t.Fatalf("software = %+v, want only computer 42's Google Chrome:\n%s", v.Software, out)
	}
}

// TestImmyNormalizeReasonKeepsDistinctCodes: resultReasonMessage carries the
// installer's own exit code or HRESULT, and different codes are different root
// causes (1603 fatal, 1618 another install running, 3010 reboot required).
// Clustering may mask what varies per machine, never the code itself.
func TestImmyNormalizeReasonKeepsDistinctCodes(t *testing.T) {
	distinct := []string{
		"Installer exited with code 1603",
		"Installer exited with code 1618",
		"Installer exited with code 3010",
		"Installer exited with code 0x80070005",
		"Installer exited with code 0x80070002",
		"Installer exited with code -2147024891",
	}
	seen := map[string]string{}
	for _, msg := range distinct {
		key := immyNormalizeReasonFor(msg, "")
		if prev, ok := seen[key]; ok {
			t.Errorf("%q and %q collapse to one cluster key %q; distinct codes are distinct causes", prev, msg, key)
		}
		seen[key] = msg
	}

	same := [][2]string{
		{"Installer failed on host 11111111-2222-3333-4444-555555555555", "Installer failed on host 99999999-8888-7777-6666-555555555555"},
		{`Log at C:\Windows\Temp\immy\run-1.log`, `Log at C:\Windows\Temp\immy\run-2.log`},
		{"Timed out at 2026-10-26T01:02:03Z", "Timed out at 2026-10-25T23:59:58.1234567+00:00"},
		{"Session 4823311 failed", "Session 4823390 failed"},
	}
	for _, p := range same {
		if a, b := immyNormalizeReasonFor(p[0], ""), immyNormalizeReasonFor(p[1], ""); a != b {
			t.Errorf("per-machine detail not masked:\n %q -> %q\n %q -> %q", p[0], a, p[1], b)
		}
	}
	// A short computer name never rewrites part of another word.
	if got := immyNormalizeReasonFor("ADD failed on AD", "AD"); got != "ADD failed on <computer>" {
		t.Errorf("short name masking = %q, want %q", got, "ADD failed on <computer>")
	}
	// The machine's own name varies per row; it is masked from its own message only.
	if a, b := immyNormalizeReasonFor("WS-01 could not reach the agent", "WS-01"),
		immyNormalizeReasonFor("WS-02 could not reach the agent", "WS-02"); a != b {
		t.Errorf("own computer name not masked: %q vs %q", a, b)
	}
}

func TestSessionTriageSplitsDistinctExitCodes(t *testing.T) {
	now := time.Now().UTC().Format(time.RFC3339)
	row := func(id int, computer, code string) map[string]any {
		return map[string]any{"id": id, "maintenanceSessionId": 900 + id, "computerId": id, "computerName": computer,
			"tenantId": 7, "tenantName": "Contoso", "maintenanceDisplayName": "Install Office",
			"status": "Complete", "statusName": "Complete", "result": "Failed", "resultName": "Failed",
			"resultReasonMessage": computer + ": installer exited with code " + code, "startTime": now}
	}
	db := seedStore(t, map[string][]map[string]any{"maintenance-actions": {
		row(1, "WS-01", "1603"), row(2, "WS-02", "1603"), row(3, "WS-03", "1618"), row(4, "WS-04", "3010"),
	}})
	out, err := runCLI(t, "session-triage", "--since", "24h", "--json", "--db", db)
	if err != nil {
		t.Fatalf("session-triage: %v\n%s", err, out)
	}
	v := decode[triageView](t, out)
	if len(v.Clusters) != 3 {
		t.Fatalf("clusters = %d, want 3 (1603 on two machines, 1618, 3010):\n%s", len(v.Clusters), out)
	}
	if v.Clusters[0].ComputerCount != 2 || !strings.Contains(v.Clusters[0].Reason, "1603") {
		t.Fatalf("top cluster = %+v, want the 1603 failure on 2 computers", v.Clusters[0])
	}
}

func TestActionReadersCountAnActionOnce(t *testing.T) {
	now := time.Now().UTC().Format(time.RFC3339)
	action := map[string]any{"id": 1, "assignmentId": 9001, "assignmentType": "Local", "computerId": 42, "computerName": "WS-42",
		"tenantName": "Contoso", "maintenanceDisplayName": "Install Chrome", "status": "Complete", "statusName": "Complete",
		"result": "Failed", "resultName": "Failed", "resultReasonMessage": "Installer failed", "startTime": now}
	// The same action mirrored by two endpoints (dx and version) is one action.
	db := seedStore(t, map[string][]map[string]any{
		"maintenance-actions":         {action},
		"maintenance-actions-version": {action},
		"computers":                   {{"id": 42, "name": "WS-42", "tenant": "Contoso", "tenantId": 7}},
	})
	out, err := runCLI(t, "session-triage", "--since", "24h", "--json", "--db", db)
	if err != nil {
		t.Fatalf("session-triage: %v\n%s", err, out)
	}
	if v := decode[triageView](t, out); v.ScannedActions != 1 || v.FailedActions != 1 {
		t.Fatalf("session-triage scanned %d / failed %d, want 1 / 1:\n%s", v.ScannedActions, v.FailedActions, out)
	}
	out, err = runCLI(t, "deployment-health", "--json", "--db", db)
	if err != nil {
		t.Fatalf("deployment-health: %v\n%s", err, out)
	}
	if v := decode[deploymentHealthView](t, out); len(v.Deployments) != 1 || v.Deployments[0].Actions != 1 {
		t.Fatalf("deployment-health = %+v, want one deployment with 1 action:\n%s", v.Deployments, out)
	}
	out, err = runCLI(t, "computer-dossier", "42", "--json", "--db", db)
	if err != nil {
		t.Fatalf("computer-dossier: %v\n%s", err, out)
	}
	if v := decode[computerDossierView](t, out); len(v.RecentActions) != 1 || v.FailedActions != 1 {
		t.Fatalf("computer-dossier actions %d / failed %d, want 1 / 1:\n%s", len(v.RecentActions), v.FailedActions, out)
	}
}
