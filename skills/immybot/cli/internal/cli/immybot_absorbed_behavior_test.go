// Copyright 2026 Abhi Saini and contributors. Licensed under Apache-2.0. See LICENSE.
//
// Behavioural acceptance for the hand-built absorbed commands.

package cli

import "testing"

func TestDriftRanksTenantsByTitlesBehind(t *testing.T) {
	db := seedStore(t, map[string][]map[string]any{
		"tenants-software-from-inventory-dx": {
			// Fleet latest: Chrome 140.0, 7-Zip 23.01
			{"id": "1", "softwareName": "Google Chrome", "version": "140.0", "tenantName": "Contoso", "computerName": "WS-01"},
			{"id": "2", "softwareName": "7-Zip", "version": "23.01", "tenantName": "Contoso", "computerName": "WS-01"},
			// Fabrikam behind on both
			{"id": "3", "softwareName": "Google Chrome", "version": "9.0", "tenantName": "Fabrikam", "computerName": "WS-02"},
			{"id": "4", "softwareName": "7-Zip", "version": "19.00", "tenantName": "Fabrikam", "computerName": "WS-02"},
			// Initech behind on one
			{"id": "5", "softwareName": "Google Chrome", "version": "139.0", "tenantName": "Initech", "computerName": "WS-03"},
			{"id": "6", "softwareName": "7-Zip", "version": "23.01", "tenantName": "Initech", "computerName": "WS-03"},
		},
	})
	out, err := runCLI(t, "drift", "--json", "--db", db)
	if err != nil {
		t.Fatalf("drift: %v\n%s", err, out)
	}
	v := decode[driftView](t, out)

	if v.TitlesTracked != 2 {
		t.Fatalf("titles_tracked = %d, want 2", v.TitlesTracked)
	}
	if len(v.Tenants) != 3 {
		t.Fatalf("tenants = %d, want 3", len(v.Tenants))
	}
	if v.Tenants[0].Tenant != "Fabrikam" || v.Tenants[0].TitlesBehind != 2 {
		t.Fatalf("worst tenant = %+v, want Fabrikam behind on 2", v.Tenants[0])
	}
	// Contoso defines the fleet latest for both titles, so it must be behind on zero.
	for _, tn := range v.Tenants {
		if tn.Tenant == "Contoso" && tn.TitlesBehind != 0 {
			t.Fatalf("Contoso sets the fleet latest and must be behind on 0, got %d", tn.TitlesBehind)
		}
		if tn.Tenant == "Initech" && tn.TitlesBehind != 1 {
			t.Fatalf("Initech should be behind on exactly 1 title, got %d", tn.TitlesBehind)
		}
	}
}

func TestDeploymentHealthFlagsNeverSucceeded(t *testing.T) {
	db := seedStore(t, map[string][]map[string]any{
		"target-assignments": {
			{"id": "asg-good", "targetName": "Chrome everywhere", "maintenanceIdentifier": "chrome", "tenantId": "t1"},
			{"id": "asg-bad", "targetName": "Broken package", "maintenanceIdentifier": "broken", "tenantId": "t1"},
		},
		"maintenance-actions": {
			{"id": "1", "assignmentId": "asg-good", "statusName": "Success"},
			{"id": "2", "assignmentId": "asg-good", "statusName": "Success"},
			{"id": "3", "assignmentId": "asg-good", "statusName": "Failed"},
			{"id": "4", "assignmentId": "asg-bad", "statusName": "Failed"},
			{"id": "5", "assignmentId": "asg-bad", "statusName": "Failed"},
		},
	})
	out, err := runCLI(t, "deployment-health", "--json", "--db", db)
	if err != nil {
		t.Fatalf("deployment-health: %v\n%s", err, out)
	}
	v := decode[deploymentHealthView](t, out)

	if v.NeverSucceeded != 1 {
		t.Fatalf("never_succeeded_count = %d, want 1", v.NeverSucceeded)
	}
	if len(v.Deployments) != 2 {
		t.Fatalf("deployments = %d, want 2", len(v.Deployments))
	}
	// Worst first.
	worst := v.Deployments[0]
	if worst.AssignmentID != "asg-bad" || !worst.NeverSucceeded || worst.Failed != 2 {
		t.Fatalf("worst deployment = %+v, want asg-bad never-succeeded with 2 failures", worst)
	}
	best := v.Deployments[1]
	if best.Succeeded != 2 || best.Failed != 1 {
		t.Fatalf("asg-good tally = %+v, want 2 ok / 1 fail", best)
	}
	if best.TargetName != "Chrome everywhere" {
		t.Fatalf("deployment name not resolved: %+v", best)
	}
}

func TestComputerDossierJoinsEverything(t *testing.T) {
	db := seedStore(t, map[string][]map[string]any{
		"computers": {
			{"id": "42", "name": "WS-42", "tenant": "Contoso", "tenantId": "t1", "online": true, "excludeFromMaintenance": false},
			{"id": "43", "name": "WS-43", "tenant": "Contoso", "tenantId": "t1", "online": false},
		},
		"tenants-software-from-inventory-dx": {
			{"id": "1", "computerId": "42", "computerName": "WS-42", "softwareName": "Google Chrome", "version": "140.0"},
			{"id": "2", "computerId": "42", "computerName": "WS-42", "softwareName": "7-Zip", "version": "23.01"},
			{"id": "3", "computerId": "43", "computerName": "WS-43", "softwareName": "Firefox", "version": "1.0"},
		},
		"maintenance-actions": {
			{"id": "1", "computerId": "42", "computerName": "WS-42", "maintenanceDisplayName": "Install Chrome", "statusName": "Failed", "startTime": "2026-08-20T01:00:00Z"},
			{"id": "2", "computerId": "42", "computerName": "WS-42", "maintenanceDisplayName": "Patch", "statusName": "Success", "startTime": "2026-08-20T02:00:00Z"},
			{"id": "3", "computerId": "43", "computerName": "WS-43", "maintenanceDisplayName": "Other", "statusName": "Failed", "startTime": "2026-08-20T03:00:00Z"},
		},
	})
	out, err := runCLI(t, "computer-dossier", "42", "--json", "--db", db)
	if err != nil {
		t.Fatalf("computer-dossier: %v\n%s", err, out)
	}
	v := decode[computerDossierView](t, out)

	if v.ComputerName != "WS-42" || v.Tenant != "Contoso" || !v.Online {
		t.Fatalf("identity wrong: %+v", v)
	}
	if v.SoftwareCount != 2 {
		t.Fatalf("software_count = %d, want 2 (WS-43's Firefox must not leak in)", v.SoftwareCount)
	}
	if len(v.RecentActions) != 2 {
		t.Fatalf("recent_actions = %d, want 2", len(v.RecentActions))
	}
	if v.FailedActions != 1 {
		t.Fatalf("failed_actions = %d, want 1", v.FailedActions)
	}
	// Newest action first.
	if v.RecentActions[0].Action != "Patch" {
		t.Fatalf("actions not newest-first: %+v", v.RecentActions)
	}
}

func TestComputerDossierUnknownComputerIsHonest(t *testing.T) {
	db := seedStore(t, map[string][]map[string]any{
		"computers": {{"id": "42", "name": "WS-42", "tenantId": "t1"}},
	})
	out, err := runCLI(t, "computer-dossier", "does-not-exist", "--json", "--db", db)
	if err != nil {
		t.Fatalf("computer-dossier: %v", err)
	}
	v := decode[computerDossierView](t, out)
	if v.Note == "" {
		t.Fatal("expected an explicit note for an unknown computer rather than an empty dossier")
	}
	if v.SoftwareCount != 0 || len(v.RecentActions) != 0 {
		t.Fatal("must not attribute any data to an unknown computer")
	}
}
