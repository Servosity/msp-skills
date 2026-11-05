// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import "testing"

// Rows join a computer by computer id only. A row with no computer id
// (spec.json GetMaintenanceActionResponse.computerId is nullable) joins no
// computer and is counted as unidentified; a name never stands for a computer,
// however many mirrored resources carry it.
func TestComputerDossierJoinsRowsByComputerIDOnly(t *testing.T) {
	type dossier struct {
		ComputerID           string `json:"computer_id"`
		SoftwareCount        int    `json:"software_count"`
		ActionCount          int    `json:"action_count"`
		FailedActions        int    `json:"failed_actions"`
		UnidentifiedActions  int    `json:"unidentified_actions"`
		UnidentifiedSoftware int    `json:"unidentified_software"`
	}
	contoso := map[string]any{"id": 42, "name": "RECEPTION", "tenant": "Contoso", "tenantId": 7, "online": true}
	fabrikam := map[string]any{"id": 77, "name": "RECEPTION", "tenant": "Fabrikam", "tenantId": 8, "online": true}
	action := func(computerID, tenant any) map[string]any {
		return map[string]any{"id": 9, "maintenanceIdentifier": "chrome", "computerId": computerID, "computerName": "RECEPTION",
			"tenantId": tenant, "maintenanceDisplayName": "Install Chrome", "result": "Failed", "status": "Complete"}
	}
	software := func(computerID any) map[string]any {
		row := map[string]any{"id": 5, "softwareName": "Chrome", "version": "140", "computerName": "RECEPTION", "tenantId": 8}
		if computerID != nil {
			row["computerId"] = computerID
		}
		return row
	}
	for _, tc := range []struct {
		name                      string
		rows                      map[string][]map[string]any
		actions, software         int
		unidActions, unidSoftware int
	}{
		{"a name two tenants carry", map[string][]map[string]any{"computers": {contoso, fabrikam},
			"maintenance-actions": {action(nil, 8)}, "tenants-software-from-inventory-dx": {software(nil)}}, 0, 0, 1, 1},
		{"a name another computer carries only in software rows", map[string][]map[string]any{"computers": {contoso},
			"maintenance-actions": {action(nil, nil)}, "tenants-software-from-inventory-dx": {software(77)}}, 0, 0, 1, 0},
		{"a name only this computer carries, in its own tenant", map[string][]map[string]any{"computers": {contoso},
			"maintenance-actions": {action(nil, 7)}}, 0, 0, 1, 0},
		{"rows carrying this computer's id", map[string][]map[string]any{"computers": {contoso, fabrikam},
			"maintenance-actions": {action(42, 7)}, "tenants-software-from-inventory-dx": {software(42)}}, 1, 1, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, err := runCLI(t, "computer-dossier", "42", "--json", "--db", seedStore(t, tc.rows))
			if err != nil {
				t.Fatalf("computer-dossier: %v\n%s", err, out)
			}
			v := decode[dossier](t, out)
			if v.ComputerID != "42" || v.ActionCount != tc.actions || v.FailedActions != tc.actions || v.SoftwareCount != tc.software ||
				v.UnidentifiedActions != tc.unidActions || v.UnidentifiedSoftware != tc.unidSoftware {
				t.Fatalf("dossier = %+v, want %d action(s) and %d software for computer 42, %d/%d unidentified:\n%s",
					v, tc.actions, tc.software, tc.unidActions, tc.unidSoftware, out)
			}
		})
	}
}

// A Computer rule's target reaches a computer by its id, never by its name.
func TestAssignmentExplainComputerRuleMatchesByIDOnly(t *testing.T) {
	contoso := map[string]any{"id": 42, "name": "RECEPTION", "tenant": "Contoso", "tenantId": 7}
	fabrikam := map[string]any{"id": 77, "name": "RECEPTION", "tenant": "Fabrikam", "tenantId": 8}
	rule := func(target string, tenant any) map[string]any {
		return map[string]any{"id": 9001, "databaseType": "Local", "maintenanceType": "GlobalSoftware",
			"maintenanceIdentifier": "chrome", "targetName": "Chrome", "targetType": "Computer", "target": target, "tenantId": tenant}
	}
	for _, tc := range []struct {
		name      string
		computers []map[string]any
		rule      map[string]any
		want      bool
	}{
		{"name two tenants carry", []map[string]any{contoso, fabrikam}, rule("RECEPTION", 8), false},
		{"name only this computer carries, its tenant", []map[string]any{contoso}, rule("RECEPTION", 7), false},
		{"name, no tenant", []map[string]any{contoso}, rule("reception", nil), false},
		{"target is the id", []map[string]any{contoso, fabrikam}, rule("42", 7), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := seedStore(t, map[string][]map[string]any{"computers": tc.computers, "target-assignments": {tc.rule}})
			out, err := runCLI(t, "assignment-explain", "42", "--data-source", "local", "--json", "--db", db)
			if err != nil {
				t.Fatalf("assignment-explain: %v\n%s", err, out)
			}
			v := decode[rowExplain](t, out)
			got := len(v.Effective) == 1 && v.Effective[0].ID == "9001"
			if got != tc.want || v.ComputerID != "42" {
				t.Fatalf("effective = %+v (computer %q), want rule 9001 effective = %t:\n%s", v.Effective, v.ComputerID, tc.want, out)
			}
		})
	}
}
