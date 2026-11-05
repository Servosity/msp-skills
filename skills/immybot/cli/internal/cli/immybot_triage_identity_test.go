// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"testing"
	"time"
)

// TestSessionTriageCountsComputersAndTenantsByID: two machines named WS-01 in
// two tenants both named Contoso (GetMaintenanceActionResponse.computerId,
// tenantId) are two computers in two tenants, not one.
func TestSessionTriageCountsComputersAndTenantsByID(t *testing.T) {
	now := time.Now().UTC().Format(time.RFC3339)
	row := func(id, computer, tenant int) map[string]any {
		return map[string]any{"id": id, "maintenanceSessionId": 700 + id, "computerId": computer, "computerName": "WS-01",
			"tenantId": tenant, "tenantName": "Contoso", "maintenanceDisplayName": "Patch Windows", "result": 2, "status": 4,
			"resultReasonMessage": "Reboot pending", "startTime": now}
	}
	db := seedStore(t, map[string][]map[string]any{"maintenance-actions": {row(1, 101, 7), row(2, 201, 8)}})
	out, err := runCLI(t, "session-triage", "--json", "--db", db)
	if err != nil {
		t.Fatalf("session-triage: %v\n%s", err, out)
	}
	v := decode[struct {
		Clusters []struct {
			ComputerCount int `json:"computer_count"`
			TenantCount   int `json:"tenant_count"`
		} `json:"clusters"`
	}](t, out)
	if len(v.Clusters) != 1 || v.Clusters[0].ComputerCount != 2 || v.Clusters[0].TenantCount != 2 {
		t.Fatalf("clusters = %+v, want one cluster with computer_count 2 and tenant_count 2:\n%s", v.Clusters, out)
	}
}
