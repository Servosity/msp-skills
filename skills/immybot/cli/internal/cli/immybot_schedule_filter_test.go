// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Behavioural acceptance for schedule-orphans --tenant on rows shaped like the
// vendor's GetScheduleResponse (tenantId nullable int32, targetMissing bool):
// a tenant filter that cannot resolve a schedule's tenant name never yields an
// all-clear.

package cli

import (
	"strings"
	"testing"
)

func TestScheduleOrphansTenantFilterNeverClearsUnresolvedTenants(t *testing.T) {
	db := seedStore(t, map[string][]map[string]any{
		// tenants never synced: tenant 1 has no name in the mirror.
		"schedules": {
			{"id": 61, "name": "Nightly Contoso", "tenantId": 1, "targetMissing": true, "tenantMissing": false,
				"targetTypeMissing": false, "disabled": false, "targetType": "Computer", "maintenanceType": "GlobalSoftware"},
			{"id": 62, "name": "Weekly", "tenantId": 2, "targetMissing": false, "tenantMissing": false,
				"targetTypeMissing": false, "disabled": false},
		},
	})
	out, err := runCLI(t, "schedule-orphans", "--tenant", "Contoso", "--json", "--db", db)
	if err != nil {
		t.Fatalf("schedule-orphans: %v\n%s", err, out)
	}
	v := decode[scheduleOrphansView](t, out)
	if strings.Contains(v.Note, "none is orphaned") {
		t.Fatalf("note = %q: an all-clear over a schedule whose tenant could not be resolved", v.Note)
	}
	if v.TenantUnresolved != 1 || !strings.Contains(v.Note, "sync --resources tenants") {
		t.Fatalf("tenant_unresolved = %d, note = %q; want 1 orphan with an unresolved tenant and a tenants sync hint", v.TenantUnresolved, v.Note)
	}
}

func TestScheduleOrphansCountsWhatTheTenantFilterDropped(t *testing.T) {
	db := seedStore(t, map[string][]map[string]any{
		"tenants": {{"id": 1, "name": "Contoso"}, {"id": 2, "name": "Fabrikam"}},
		"schedules": {
			{"id": 61, "name": "Nightly Contoso", "tenantId": 1, "targetMissing": true},
			{"id": 63, "name": "Nightly Fabrikam", "tenantId": 2, "targetMissing": true},
		},
	})
	out, err := runCLI(t, "schedule-orphans", "--tenant", "Contoso", "--json", "--db", db)
	if err != nil {
		t.Fatalf("schedule-orphans: %v\n%s", err, out)
	}
	v := decode[scheduleOrphansView](t, out)
	if v.MatchedCount != 1 || v.FilteredOut != 1 || v.TenantUnresolved != 0 {
		t.Fatalf("matched %d, filtered_out %d, tenant_unresolved %d; want 1, 1, 0:\n%s", v.MatchedCount, v.FilteredOut, v.TenantUnresolved, out)
	}
}
