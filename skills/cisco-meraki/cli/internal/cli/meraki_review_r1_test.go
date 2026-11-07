// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written (NOT generated): reprint 4.35.1 Codex round 1 classes, pinned by
// skills/cisco-meraki/handfixes.json "meraki-push-selection-exact" and
// "meraki-time-compare-parsed".

package cli

import (
	"testing"
	"time"
)

// Meraki documents network tag filtering as case-sensitive (spec.json GET
// /organizations/{organizationId}/networks, parameter tags). fleet push
// selects exactly what the operator named: org ID or name, network ID or
// name, tag and product type, with no case folding.
func TestPushSelectionIsExact(t *testing.T) {
	cases := []struct {
		name string
		f    pushTargetFilter
		id   string
		net  string
		tags []string
		pt   []string
		want bool
	}{
		{"tag differs in case", pushTargetFilter{tag: "managed"}, "N_2", "Other", []string{"Managed"}, []string{"appliance"}, false},
		{"tag exact", pushTargetFilter{tag: "managed"}, "N_1", "Main", []string{"managed"}, []string{"appliance"}, true},
		{"network name differs in case", pushTargetFilter{networks: []string{"main"}}, "N_1", "Main", nil, []string{"appliance"}, false},
		{"network id differs in case", pushTargetFilter{networks: []string{"n_1"}}, "N_1", "Main", nil, []string{"appliance"}, false},
		{"network name exact", pushTargetFilter{networks: []string{"Main"}}, "N_1", "Main", nil, []string{"appliance"}, true},
		{"product type differs in case", pushTargetFilter{productType: "Appliance"}, "N_1", "Main", nil, []string{"appliance"}, false},
	}
	for _, c := range cases {
		if got := c.f.keep(c.id, c.net, c.tags, c.pt); got != c.want {
			t.Errorf("%s: keep=%v, want %v", c.name, got, c.want)
		}
	}
	orgs := []fleetOrg{{ID: "1", Name: "Acme"}, {ID: "2", Name: "ACME"}, {ID: "3", Name: "Bayside"}}
	got := pushOrgsExact(orgs, []string{"Acme", "3"})
	if len(got) != 2 || got[0].ID != "1" || got[1].ID != "3" {
		t.Errorf("pushOrgsExact = %+v, want orgs 1 and 3 only", got)
	}
	if got := pushOrgsExact(orgs, nil); len(got) != 3 {
		t.Errorf("no --org keeps every org, got %d", len(got))
	}
}

// Meraki timestamps are RFC 3339 date-times that may carry an offset
// (spec.json admins lastActive, configurationChanges ts: format date-time), so
// "latest" and "newest first" compare instants, never text.
func TestMerakiTimesCompareAsInstants(t *testing.T) {
	now := time.Date(2026, 11, 2, 14, 0, 0, 0, time.UTC)
	org1, org2 := fleetOrg{ID: "1", Name: "A"}, fleetOrg{ID: "2", Name: "B"}
	byOrg := map[fleetOrg][]rawAdmin{
		org1: {{Email: "tech@example.com", LastActive: "2026-11-01T23:00:00+10:00", TwoFactorAuthEnabled: true}}, // 13:00Z
		org2: {{Email: "tech@example.com", LastActive: "2026-11-01T15:00:00Z", TwoFactorAuthEnabled: true}},
	}
	got := summarizeAdmins(byOrg, 24*time.Hour, now)
	if len(got) != 1 || got[0].LastActive != "2026-11-01T15:00:00Z" {
		t.Fatalf("latest sign-in = %+v, want 2026-11-01T15:00:00Z", got)
	}
	for _, f := range got[0].Flags {
		if f == "stale" {
			t.Errorf("admin active 23h ago flagged stale: %v", got[0].Flags)
		}
	}

	rows := []changeRow{
		{TS: "2026-11-01T23:00:00+10:00", OrganizationID: "1", OrganizationName: "A", AdminName: "x"}, // 13:00Z
		{TS: "2026-11-01T15:00:00Z", OrganizationID: "1", OrganizationName: "A", AdminName: "x"},
	}
	sortChangesNewestFirst(rows)
	if rows[0].TS != "2026-11-01T15:00:00Z" {
		t.Errorf("newest first = %v, want the 15:00Z change first", rows)
	}
	g := groupChanges(rows, "org")
	if len(g) != 1 || g[0].LastTS != "2026-11-01T15:00:00Z" || g[0].FirstTS != "2026-11-01T23:00:00+10:00" {
		t.Errorf("group first/last = %+v", g)
	}
}
