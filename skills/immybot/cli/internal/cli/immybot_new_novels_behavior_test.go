// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Behavioural acceptance for schedule-orphans and provider-link-health. The
// fixtures use the field shapes the vendor documents (GetScheduleResponse,
// GetProviderLinkResponse) and assert on output content.

package cli

import (
	"strings"
	"testing"

	"immybot-pp-cli/internal/cliutil/testenv"
)

func TestNovelScheduleOrphansListsWhatImmyBotFlags(t *testing.T) {
	testenv.Isolate(t)
	db := seedStore(t, map[string][]map[string]any{
		"tenants": {{"id": 7, "name": "Contoso"}, {"id": 8, "name": "Fabrikam"}},
		"schedules": {
			{"id": 1, "name": "Nightly patch", "tenantId": 7, "targetMissing": true, "maintenanceType": 2},
			{"id": 2, "name": "Old onboarding", "tenantId": 8, "tenantMissing": true, "targetTypeMissing": true, "disabled": true},
			{"id": 3, "name": "Paused", "tenantId": 7, "disabled": true, "maintenanceType": "GlobalSoftware"},
			{"id": 4, "name": "Healthy", "tenantId": 7},
			{"id": 5, "name": "Global orphan", "tenantId": nil, "targetMissing": true},
		},
	})
	out, err := runCLI(t, "schedule-orphans", "--db", db, "--json")
	if err != nil {
		t.Fatalf("schedule-orphans: %v\n%s", err, out)
	}
	v := decode[scheduleOrphansView](t, out)
	if v.Scanned != 5 || v.MatchedCount != 4 || v.OrphanedCount != 3 || v.DisabledCount != 2 {
		t.Fatalf("counts: %+v", v)
	}
	if len(v.Schedules) != 4 || !v.Schedules[0].Orphaned || v.Schedules[3].ID != "3" {
		t.Fatalf("orphans must sort first, the disabled-only schedule last: %+v", v.Schedules)
	}
	for _, s := range v.Schedules {
		if s.ID == "4" {
			t.Fatalf("a healthy schedule was listed: %+v", s)
		}
		if s.ID == "1" && (s.Tenant != "Contoso" || s.MaintenanceType != "2" || strings.Join(s.Reasons, ",") != "target missing") {
			t.Fatalf("schedule 1: %+v", s)
		}
		if s.ID == "2" && strings.Join(s.Reasons, ",") != "tenant missing,target type missing,disabled" {
			t.Fatalf("schedule 2 reasons: %v", s.Reasons)
		}
		if s.ID == "5" && (s.TenantID != "" || s.Tenant != "") {
			t.Fatalf("a global schedule has no tenant: %+v", s)
		}
	}

	out, err = runCLI(t, "schedule-orphans", "--db", db, "--json", "--orphaned-only", "--limit", "1")
	if err != nil {
		t.Fatal(err)
	}
	v = decode[scheduleOrphansView](t, out)
	if v.MatchedCount != 3 || len(v.Schedules) != 1 {
		t.Fatalf("--limit must cut the list, not the totals: %+v", v)
	}
	out, err = runCLI(t, "schedule-orphans", "--db", db, "--json", "--tenant", "fabri")
	if err != nil {
		t.Fatal(err)
	}
	if v = decode[scheduleOrphansView](t, out); v.MatchedCount != 1 || v.Schedules[0].ID != "2" {
		t.Fatalf("--tenant: %+v", v)
	}
}

func TestNovelScheduleOrphansCountsUnreadableRows(t *testing.T) {
	testenv.Isolate(t)
	db := seedStore(t, map[string][]map[string]any{
		"schedules": {{"id": 1, "name": "Bad", "targetMissing": "yes"}, {"id": 2, "targetMissing": true}},
	})
	out, err := runCLI(t, "schedule-orphans", "--db", db, "--json")
	if err != nil {
		t.Fatal(err)
	}
	v := decode[scheduleOrphansView](t, out)
	if v.Unreadable != 1 || v.OrphanedCount != 1 || !strings.Contains(v.Note, "could not be read") {
		t.Fatalf("an unreadable row must be counted and named, never dropped silently: %+v", v)
	}
}

func TestNovelProviderLinkHealthFlagsEveryFailureSignal(t *testing.T) {
	testenv.Isolate(t)
	db := seedStore(t, map[string][]map[string]any{
		"tenants": {{"id": 1, "name": "MSP"}},
		"provider-links": {
			{"id": 10, "name": "NinjaOne", "healthStatus": 0, "healthStatusMessage": "token expired", "ownerTenantId": 1},
			{"id": 11, "name": "CW Manage", "healthStatus": "Degraded"},
			{"id": 12, "name": "Halo", "healthStatus": "Healthy", "includeClientsFailedMessage": "HTTP 500 listing clients"},
			{"id": 13, "name": "Old RMM", "healthStatus": 2, "disabled": true},
			{"id": 14, "name": "Fine", "healthStatus": 2, "healthStatusMessage": "OK"},
			{"id": 15, "name": "Future", "healthStatus": 7},
			{"id": 16, "name": "Silent"},
		},
	})
	out, err := runCLI(t, "provider-link-health", "--db", db, "--json")
	if err != nil {
		t.Fatalf("provider-link-health: %v\n%s", err, out)
	}
	v := decode[providerLinkHealthView](t, out)
	if v.Scanned != 7 || v.Healthy != 1 || v.FlaggedCount != 6 || v.Unhealthy != 1 || v.Degraded != 1 ||
		v.Disabled != 1 || v.HealthUnreported != 1 {
		t.Fatalf("counts: %+v", v)
	}
	order := []string{}
	for _, l := range v.Links {
		order = append(order, l.ID)
	}
	if strings.Join(order, ",") != "10,11,15,12,13,16" {
		t.Fatalf("severity order: %v", order)
	}
	got := map[string]providerLinkIssue{}
	for _, l := range v.Links {
		got[l.ID] = l
	}
	if l := got["10"]; l.Health != "Unhealthy" || l.OwnerTenant != "MSP" || l.Messages["health"] != "token expired" {
		t.Fatalf("integer 0 is Unhealthy (vendor-documented): %+v", l)
	}
	if l := got["12"]; l.Health != "Healthy" || l.Messages["include_clients"] == "" {
		t.Fatalf("a failure message flags the link whatever the health field says: %+v", l)
	}
	if l := got["15"]; l.Health != "unknown" {
		t.Fatalf("an unrecognised status must be flagged, not read as healthy: %+v", l)
	}
	if _, listed := got["14"]; listed {
		t.Fatal("a healthy link with a benign status message was listed")
	}

	out, err = runCLI(t, "provider-link-health", "--db", db, "--json", "--include-disabled=false")
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range decode[providerLinkHealthView](t, out).Links {
		if l.ID == "13" {
			t.Fatal("--include-disabled=false still listed a link that is only disabled")
		}
	}
}

func TestImmyPrintableStripsTerminalControls(t *testing.T) {
	if got := immyPrintable("ok\x1b[31mred\x07\u009b"); strings.ContainsAny(got, "\x1b\x07\u009b") {
		t.Fatalf("control characters survived: %q", got)
	}
}
