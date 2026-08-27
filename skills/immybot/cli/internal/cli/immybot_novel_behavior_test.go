// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Behavioural acceptance for the ImmyBot novel commands. These seed a real
// SQLite mirror and assert on output CONTENT, not just exit codes: a command
// that returns cleanly but computes the wrong answer must fail here.

package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"immybot-pp-cli/internal/store"
)

func seedStore(t *testing.T, rows map[string][]map[string]any) string {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "immy.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer db.Close()
	for resourceType, items := range rows {
		for _, item := range items {
			raw, err := json.Marshal(item)
			if err != nil {
				t.Fatalf("marshal %s: %v", resourceType, err)
			}
			id := fmt.Sprintf("%v", item["id"])
			if err := db.Upsert(resourceType, id, raw); err != nil {
				t.Fatalf("upsert %s/%s: %v", resourceType, id, err)
			}
		}
	}
	return dbPath
}

func runCLI(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := RootCmd()
	cmd.SetArgs(args)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	err := cmd.Execute()
	return out.String(), err
}

func decode[T any](t *testing.T, s string) T {
	t.Helper()
	var v T
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		t.Fatalf("output is not valid JSON (%v):\n%s", err, s)
	}
	return v
}

// --- session-triage -------------------------------------------------------

func TestSessionTriageClustersIdenticalFailures(t *testing.T) {
	now := time.Now().UTC().Format(time.RFC3339)
	db := seedStore(t, map[string][]map[string]any{
		"maintenance-actions": {
			{"id": "1", "maintenanceDisplayName": "Install Chrome", "statusName": "Failed",
				"reason":       "Exit code 1603 on host 11111111-2222-3333-4444-555555555555",
				"computerName": "WS-01", "tenantName": "Contoso", "maintenanceSessionId": "s1", "startTime": now},
			{"id": "2", "maintenanceDisplayName": "Install Chrome", "statusName": "Failed",
				"reason":       "Exit code 1604 on host 99999999-8888-7777-6666-555555555555",
				"computerName": "WS-02", "tenantName": "Fabrikam", "maintenanceSessionId": "s2", "startTime": now},
			{"id": "3", "maintenanceDisplayName": "Patch Windows", "statusName": "Failed",
				"reason": "Reboot pending", "computerName": "WS-03", "tenantName": "Contoso",
				"maintenanceSessionId": "s3", "startTime": now},
			{"id": "4", "maintenanceDisplayName": "Install Chrome", "statusName": "Success",
				"reason": "", "computerName": "WS-04", "tenantName": "Contoso",
				"maintenanceSessionId": "s4", "startTime": now},
		},
	})

	out, err := runCLI(t, "session-triage", "--since", "24h", "--json", "--db", db)
	if err != nil {
		t.Fatalf("session-triage: %v\n%s", err, out)
	}
	v := decode[triageView](t, out)

	if v.FailedActions != 3 {
		t.Fatalf("failed_actions = %d, want 3 (successes must be excluded)", v.FailedActions)
	}
	if len(v.Clusters) != 2 {
		t.Fatalf("clusters = %d, want 2; the two Chrome failures differ only by exit code and GUID and must collapse:\n%s", len(v.Clusters), out)
	}
	top := v.Clusters[0]
	if top.Action != "Install Chrome" || top.ComputerCount != 2 {
		t.Fatalf("top cluster = %+v, want Install Chrome across 2 computers", top)
	}
	if top.TenantCount != 2 {
		t.Fatalf("top cluster tenant_count = %d, want 2", top.TenantCount)
	}
}

func TestSessionTriageEmptyIsHonest(t *testing.T) {
	db := seedStore(t, map[string][]map[string]any{
		"maintenance-actions": {
			{"id": "1", "maintenanceDisplayName": "Install Chrome", "statusName": "Success", "computerName": "WS-01"},
		},
	})
	out, err := runCLI(t, "session-triage", "--since", "24h", "--json", "--db", db)
	if err != nil {
		t.Fatalf("session-triage: %v", err)
	}
	v := decode[triageView](t, out)
	if len(v.Clusters) != 0 {
		t.Fatalf("clusters = %d, want 0 when nothing failed", len(v.Clusters))
	}
	if v.Note == "" {
		t.Fatal("expected a note explaining that actions were scanned but none failed")
	}
	// Empty lists must marshal as [], never null, or agents crash on iteration.
	if !bytes.Contains([]byte(out), []byte(`"clusters": []`)) && !bytes.Contains([]byte(out), []byte(`"clusters":[]`)) {
		t.Fatalf("clusters must serialise as [] not null:\n%s", out)
	}
}

// --- version-spread -------------------------------------------------------

func TestVersionSpreadOrdersNumericallyNotLexically(t *testing.T) {
	db := seedStore(t, map[string][]map[string]any{
		"tenants-software-from-inventory-dx": {
			{"id": "1", "softwareName": "Google Chrome", "version": "140.0.1", "tenantName": "Contoso", "computerName": "WS-01"},
			{"id": "2", "softwareName": "Google Chrome", "version": "9.0.5", "tenantName": "Fabrikam", "computerName": "WS-02"},
			{"id": "3", "softwareName": "Google Chrome", "version": "140.0.1", "tenantName": "Contoso", "computerName": "WS-03"},
			{"id": "4", "softwareName": "7-Zip", "version": "23.01", "tenantName": "Contoso", "computerName": "WS-01"},
		},
	})
	out, err := runCLI(t, "version-spread", "Google Chrome", "--min-version", "140", "--json", "--db", db)
	if err != nil {
		t.Fatalf("version-spread: %v\n%s", err, out)
	}
	v := decode[versionSpreadView](t, out)

	if v.MatchedInstalls != 3 {
		t.Fatalf("matched_installs = %d, want 3 (7-Zip must not match)", v.MatchedInstalls)
	}
	if len(v.Versions) != 2 {
		t.Fatalf("distinct versions = %d, want 2", len(v.Versions))
	}
	// The whole point: 140.0.1 must outrank 9.0.5. String sorting inverts this.
	if v.Versions[0].Version != "140.0.1" {
		t.Fatalf("highest version = %q, want 140.0.1 (numeric ordering, not lexical)", v.Versions[0].Version)
	}
	if v.Versions[0].BelowFloor {
		t.Fatal("140.0.1 must not be flagged below a 140 floor")
	}
	if !v.Versions[1].BelowFloor {
		t.Fatal("9.0.5 must be flagged below a 140 floor")
	}
	if v.BelowFloor != 1 {
		t.Fatalf("computers_below_floor = %d, want 1", v.BelowFloor)
	}
	if len(v.TenantsBehind) != 1 || v.TenantsBehind[0].Tenant != "Fabrikam" {
		t.Fatalf("tenants_behind = %+v, want only Fabrikam", v.TenantsBehind)
	}
}

func TestVersionSpreadNegativeMatch(t *testing.T) {
	db := seedStore(t, map[string][]map[string]any{
		"tenants-software-from-inventory-dx": {
			{"id": "1", "softwareName": "Google Chrome", "version": "140.0", "tenantName": "Contoso", "computerName": "WS-01"},
		},
	})
	out, err := runCLI(t, "version-spread", "Definitely Not Installed", "--json", "--db", db)
	if err != nil {
		t.Fatalf("version-spread: %v", err)
	}
	v := decode[versionSpreadView](t, out)
	if len(v.Versions) != 0 {
		t.Fatalf("versions = %+v, want none for a non-matching title", v.Versions)
	}
	if v.Note == "" {
		t.Fatal("expected a note when nothing matched")
	}
}

// --- onboarding-stalled ---------------------------------------------------

func TestOnboardingStalledBucketsAndOutcomes(t *testing.T) {
	old := time.Now().UTC().Add(-10 * 24 * time.Hour).Format(time.RFC3339)
	recent := time.Now().UTC().Add(-2 * time.Hour).Format(time.RFC3339)
	db := seedStore(t, map[string][]map[string]any{
		"computers-onboarding": {
			{"id": "1", "computerName": "WS-OLD", "tenantName": "Contoso", "updatedDate": old,
				"onboardingSessionId": "", "onboardingFailed": false, "onboardingStatus": "Queued"},
			{"id": "2", "computerName": "WS-FAILED", "tenantName": "Contoso", "updatedDate": old,
				"onboardingSessionId": "sess-9", "onboardingFailed": true, "onboardingStatus": "Failed"},
			{"id": "3", "computerName": "WS-NEW", "tenantName": "Fabrikam", "updatedDate": recent,
				"onboardingSessionId": "", "onboardingFailed": false, "onboardingStatus": "Queued"},
		},
	})
	out, err := runCLI(t, "onboarding-stalled", "--older-than", "3d", "--json", "--db", db)
	if err != nil {
		t.Fatalf("onboarding-stalled: %v\n%s", err, out)
	}
	v := decode[onboardingStalledView](t, out)

	if v.StalledCount != 2 {
		t.Fatalf("stalled_count = %d, want 2 (the 2-hour-old machine is not stalled)", v.StalledCount)
	}
	if v.NeverAttempted != 1 {
		t.Fatalf("never_attempted = %d, want 1", v.NeverAttempted)
	}
	if v.FailedAttempts != 1 {
		t.Fatalf("failed_attempts = %d, want 1", v.FailedAttempts)
	}
	if v.BucketCounts["7-30d"] != 2 {
		t.Fatalf("bucket 7-30d = %d, want 2; buckets=%+v", v.BucketCounts["7-30d"], v.BucketCounts)
	}
}

// --- assignment-explain ---------------------------------------------------

func TestAssignmentExplainShadowsBroaderScope(t *testing.T) {
	db := seedStore(t, map[string][]map[string]any{
		"computers": {
			{"id": "42", "name": "WS-42", "tenant": "Contoso", "tenantId": "t1"},
			{"id": "43", "name": "WS-43", "tenant": "Fabrikam", "tenantId": "t2"},
		},
		"target-assignments": {
			{"id": "a-tenant", "maintenanceIdentifier": "chrome", "maintenanceType": "software",
				"targetName": "Chrome (tenant-wide)", "targetTypeName": "Tenant", "tenantId": "t1"},
			{"id": "a-computer", "maintenanceIdentifier": "chrome", "maintenanceType": "software",
				"targetName": "Chrome (this machine)", "targetTypeName": "Computer", "tenantId": "t1", "target": "42"},
			{"id": "a-other", "maintenanceIdentifier": "sevenzip", "maintenanceType": "software",
				"targetName": "7-Zip (other tenant)", "targetTypeName": "Tenant", "tenantId": "t2"},
		},
	})
	out, err := runCLI(t, "assignment-explain", "42", "--json", "--data-source", "local", "--db", db)
	if err != nil {
		t.Fatalf("assignment-explain: %v\n%s", err, out)
	}
	v := decode[assignmentExplainView](t, out)

	if v.ComputerName != "WS-42" {
		t.Fatalf("computer_name = %q, want WS-42", v.ComputerName)
	}
	if len(v.Effective) != 1 || v.Effective[0].ID != "a-computer" {
		t.Fatalf("effective = %+v, want only the computer-scoped rule to win", v.Effective)
	}
	if v.Effective[0].ScopeMatched != "computer" {
		t.Fatalf("scope_matched = %q, want computer", v.Effective[0].ScopeMatched)
	}
	if len(v.Shadowed) != 1 || v.Shadowed[0].ID != "a-tenant" {
		t.Fatalf("shadowed = %+v, want the tenant-wide rule shadowed", v.Shadowed)
	}
	if v.Shadowed[0].ShadowedBy != "a-computer" {
		t.Fatalf("shadowed_by = %q, want a-computer", v.Shadowed[0].ShadowedBy)
	}
	// Negative: another tenant's assignment must never leak in.
	for _, a := range append(v.Effective, v.Shadowed...) {
		if a.ID == "a-other" {
			t.Fatal("an assignment scoped to a different tenant leaked into the result")
		}
	}
}

// --- script-blast-radius --------------------------------------------------

func TestScriptBlastRadiusWalksAllHops(t *testing.T) {
	db := seedStore(t, map[string][]map[string]any{
		"scripts": {
			{"id": "312", "name": "Detect Chrome", "databaseType": "Global"},
			{"id": "999", "name": "Unrelated", "databaseType": "Local"},
		},
		"maintenance-tasks": {
			{"id": "task-1", "name": "Chrome Task", "getScriptId": "312", "setScriptId": "312", "testScriptId": nil},
			{"id": "task-2", "name": "Other Task", "getScriptId": "999"},
		},
		"target-assignments": {
			{"id": "asg-1", "maintenanceIdentifier": "task-1", "targetName": "All Contoso", "tenantId": "t1"},
			{"id": "asg-2", "maintenanceIdentifier": "task-2", "targetName": "Unrelated", "tenantId": "t2"},
		},
		"tenants": {
			{"id": "t1", "name": "Contoso"},
			{"id": "t2", "name": "Fabrikam"},
		},
		"computers": {
			{"id": "1", "name": "WS-01", "tenantId": "t1"},
			{"id": "2", "name": "WS-02", "tenantId": "t1"},
			{"id": "3", "name": "WS-03", "tenantId": "t2"},
		},
	})
	out, err := runCLI(t, "script-blast-radius", "312", "--json", "--db", db)
	if err != nil {
		t.Fatalf("script-blast-radius: %v\n%s", err, out)
	}
	v := decode[blastRadiusView](t, out)

	if v.ScriptName != "Detect Chrome" {
		t.Fatalf("script_name = %q, want Detect Chrome", v.ScriptName)
	}
	if len(v.Tasks) != 1 || v.Tasks[0].TaskID != "task-1" {
		t.Fatalf("consuming_tasks = %+v, want only task-1", v.Tasks)
	}
	if len(v.Tasks[0].Roles) != 2 {
		t.Fatalf("roles = %v, want get and set", v.Tasks[0].Roles)
	}
	if len(v.Assignments) != 1 || v.Assignments[0].ID != "asg-1" {
		t.Fatalf("deployments = %+v, want only asg-1", v.Assignments)
	}
	if v.Assignments[0].TenantName != "Contoso" {
		t.Fatalf("tenant name not resolved: %+v", v.Assignments[0])
	}
	// Only Contoso machines are reachable; WS-03 belongs to the unrelated tenant.
	if v.ComputerCount != 2 {
		t.Fatalf("computers_in_affected_tenants = %d, want 2", v.ComputerCount)
	}
}

// --- psa-reconcile --------------------------------------------------------

func TestPsaReconcileFindsBothSides(t *testing.T) {
	db := seedStore(t, map[string][]map[string]any{
		"provider-links": {
			{"id": "7", "name": "ConnectWise", "computers": []map[string]any{
				{"id": "1", "computerName": "WS-01", "tenantId": "t1", "tenantName": "Contoso"},
				{"id": "404", "computerName": "GHOST", "tenantId": "t1", "tenantName": "Contoso"},
			}, "providerClients": []map[string]any{
				{"externalClientId": "cw-1", "externalClientName": "Contoso", "linkedToTenantId": "t1"},
				{"externalClientId": "cw-2", "externalClientName": "Orphan Co", "linkedToTenantId": ""},
			}},
		},
		"computers": {
			{"id": "1", "name": "WS-01", "tenantId": "t1", "tenant": "Contoso"},
			{"id": "2", "name": "WS-02", "tenantId": "t1", "tenant": "Contoso"},
		},
		"tenants": {
			{"id": "t1", "name": "Contoso"},
			{"id": "t2", "name": "Fabrikam"},
		},
	})
	out, err := runCLI(t, "psa-reconcile", "--provider", "7", "--json", "--data-source", "local", "--db", db)
	if err != nil {
		t.Fatalf("psa-reconcile: %v\n%s", err, out)
	}
	v := decode[psaReconcileView](t, out)

	if len(v.UnlinkedComputers) != 1 || v.UnlinkedComputers[0].ComputerID != "2" {
		t.Fatalf("unlinked_computers = %+v, want only WS-02", v.UnlinkedComputers)
	}
	if len(v.OrphanedAssets) != 1 || v.OrphanedAssets[0].ComputerID != "404" {
		t.Fatalf("orphaned_provider_assets = %+v, want only the ghost asset", v.OrphanedAssets)
	}
	if len(v.UnmappedClients) != 1 || v.UnmappedClients[0].ExternalClient != "cw-2" {
		t.Fatalf("unmapped_provider_clients = %+v, want only cw-2", v.UnmappedClients)
	}
	if len(v.TenantsWithNoLink) != 1 || v.TenantsWithNoLink[0] != "Fabrikam" {
		t.Fatalf("tenants_with_no_provider_client = %v, want [Fabrikam]", v.TenantsWithNoLink)
	}
}

// --- fleet-diff -----------------------------------------------------------

func TestFleetDiffDetectsAddRemoveChange(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "immy.db")
	write := func(items []map[string]any) {
		db, err := store.Open(dbPath)
		if err != nil {
			t.Fatalf("open store: %v", err)
		}
		defer db.Close()
		if _, err := db.DB().Exec(`DELETE FROM resources WHERE resource_type = 'computers'`); err != nil {
			t.Fatalf("clear: %v", err)
		}
		for _, item := range items {
			raw, _ := json.Marshal(item)
			if err := db.Upsert("computers", fmt.Sprintf("%v", item["id"]), raw); err != nil {
				t.Fatalf("upsert: %v", err)
			}
		}
	}

	write([]map[string]any{
		{"id": "1", "name": "WS-01", "tenantId": "t1", "online": true},
		{"id": "2", "name": "WS-02", "tenantId": "t1", "online": true},
	})
	if out, err := runCLI(t, "fleet-diff", "--snapshot", "--json", "--db", dbPath); err != nil {
		t.Fatalf("snapshot: %v\n%s", err, out)
	}

	// WS-02 goes offline, WS-03 joins, WS-01 is unchanged... and we drop none.
	write([]map[string]any{
		{"id": "1", "name": "WS-01", "tenantId": "t1", "online": true},
		{"id": "2", "name": "WS-02", "tenantId": "t1", "online": false},
		{"id": "3", "name": "WS-03", "tenantId": "t1", "online": true},
	})
	out, err := runCLI(t, "fleet-diff", "--json", "--db", dbPath)
	if err != nil {
		t.Fatalf("fleet-diff: %v\n%s", err, out)
	}
	v := decode[fleetDiffView](t, out)

	if len(v.Added) != 1 || v.Added[0].ID != "3" {
		t.Fatalf("added = %+v, want WS-03", v.Added)
	}
	if len(v.Changed) != 1 || v.Changed[0].ID != "2" {
		t.Fatalf("changed = %+v, want WS-02 (went offline)", v.Changed)
	}
	if len(v.Removed) != 0 {
		t.Fatalf("removed = %+v, want none; fabricating removals is the failure mode here", v.Removed)
	}
}

func TestFleetDiffWithoutBaselineIsHonest(t *testing.T) {
	db := seedStore(t, map[string][]map[string]any{
		"computers": {{"id": "1", "name": "WS-01", "tenantId": "t1", "online": true}},
	})
	out, err := runCLI(t, "fleet-diff", "--since", "24h", "--json", "--db", db)
	if err != nil {
		t.Fatalf("fleet-diff: %v", err)
	}
	v := decode[fleetDiffView](t, out)
	if len(v.Added)+len(v.Removed)+len(v.Changed) != 0 {
		t.Fatal("with no baseline snapshot the command must report nothing rather than invent changes")
	}
	if v.Note == "" {
		t.Fatal("expected a note telling the user to record a baseline snapshot")
	}
}
