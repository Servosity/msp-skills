// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"testing"
	"time"
)

func TestClassifyIdentifier(t *testing.T) {
	cases := []struct{ in, kind, value string }{
		{"00:18:0A:12:34:56", "mac", "00:18:0a:12:34:56"},
		{"00-18-0a-12-34-56", "mac", "00:18:0a:12:34:56"},
		{"0018.0a12.3456", "mac", "00:18:0a:12:34:56"},
		{"00180a123456", "mac", "00:18:0a:12:34:56"},
		{"q2qn-9j8l-slpd", "serial", "Q2QN-9J8L-SLPD"},
		{"L_646829496481105433", "network_id", "L_646829496481105433"},
		{"203.0.113.10", "ip", "203.0.113.10"},
		{"2001:db8::1", "ip", "2001:db8::1"},
		{"Front Desk", "name", "Front Desk"},
		{"deadbeef", "name", "deadbeef"}, // 8 hex chars is not a MAC
	}
	for _, c := range cases {
		k, v := classifyIdentifier(c.in)
		if k != c.kind || v != c.value {
			t.Errorf("classifyIdentifier(%q) = %s %q, want %s %q", c.in, k, v, c.kind, c.value)
		}
	}
}

func TestParseMerakiTimeCotermLayout(t *testing.T) {
	got, ok := parseMerakiTime("Feb 8, 2027 UTC")
	if !ok || got.Format("2006-01-02") != "2027-02-08" {
		t.Fatalf("co-term date not parsed: %v %v", got, ok)
	}
	if _, ok := parseMerakiTime("not a date"); ok {
		t.Fatal("garbage parsed as a date")
	}
}

func hc(serial, ts, old, new, net string) availabilityChange {
	var c availabilityChange
	c.TS, c.Device.Serial, c.Network.ID = ts, serial, net
	c.Details.Old = []struct{ Name, Value string }{{"status", old}}
	c.Details.New = []struct{ Name, Value string }{{"status", new}}
	return c
}

func TestComputeFlaps(t *testing.T) {
	now := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	start := now.Add(-7 * 24 * time.Hour)
	changes := []availabilityChange{
		hc("A", "2026-10-12T10:00:00Z", "online", "offline", "N_1"),
		hc("A", "2026-10-12T10:30:00Z", "offline", "online", "N_1"),
		hc("B", "2026-10-13T11:00:00Z", "online", "offline", "N_1"), // still offline: 60 min to now
		hc("C", "2026-10-07T12:00:00Z", "offline", "online", "N_1"), // recovered: offline since window start (1 day)
	}
	got := map[string]flapRow{}
	for _, r := range computeFlaps(fleetOrg{ID: "1"}, changes, start, now) {
		got[r.Serial] = r
	}
	if got["A"].DowntimeMinutes != 30 || got["A"].Transitions != 2 || got["A"].CurrentlyOffline {
		t.Errorf("A: %+v", got["A"])
	}
	if got["B"].DowntimeMinutes != 60 || !got["B"].CurrentlyOffline {
		t.Errorf("B: %+v", got["B"])
	}
	if got["C"].DowntimeMinutes != 24*60 {
		t.Errorf("C (offline before window): %+v", got["C"])
	}
}

func TestCorrelateOutages(t *testing.T) {
	history := []availabilityChange{hc("A", "2026-10-12T10:00:00Z", "online", "offline", "N_1"), hc("A", "2026-10-12T11:00:00Z", "offline", "online", "N_1")}
	changes := []rawChange{
		{TS: "2026-10-12T09:30:00Z", Label: "suspect"},
		{TS: "2026-10-12T07:00:00Z", Label: "too early"},
		{TS: "2026-10-12T10:05:00Z", Label: "after outage"},
	}
	out := correlateOutages("1", history, map[string][]rawChange{"N_1": changes, "N_2": {{TS: "2026-10-12T09:50:00Z", Label: "other network"}}}, 2*time.Hour)
	if len(out) != 1 {
		t.Fatalf("recovery must not be an outage; got %d outages", len(out))
	}
	if out[0].SuspectCount != 1 || out[0].Suspects[0].Label != "suspect" || out[0].Suspects[0].MinutesBefore != 30 {
		t.Fatalf("suspects: %+v", out[0].Suspects)
	}
	if none := correlateOutages("1", history, nil, time.Hour); none[0].Suspects == nil {
		t.Fatal("no changes must give an empty suspects slice, not null")
	}
}

func TestCorrelateOutagesUsesEventNetwork(t *testing.T) {
	// The device moved from N_1 to N_2; each outage is matched to the changes
	// of the network it happened in.
	history := []availabilityChange{hc("A", "2026-10-10T10:00:00Z", "online", "offline", "N_1"), hc("A", "2026-10-12T10:00:00Z", "online", "offline", "N_2")}
	changes := map[string][]rawChange{"N_1": {{TS: "2026-10-10T09:00:00Z", Label: "n1 change"}}, "N_2": {{TS: "2026-10-12T09:00:00Z", Label: "n2 change"}}}
	out := correlateOutages("1", history, changes, 2*time.Hour)
	if len(out) != 2 || out[0].NetworkID != "N_2" || out[0].Suspects[0].Label != "n2 change" || out[1].NetworkID != "N_1" || out[1].Suspects[0].Label != "n1 change" {
		t.Fatalf("per-network correlation: %+v", out)
	}
}

func TestFixedWindow(t *testing.T) {
	now := time.Date(2026, 10, 13, 0, 0, 0, 0, time.UTC)
	t0, t1, clamped := fixedWindow(now, 7*24*time.Hour, maxAvailabilityLookback)
	if clamped || !t1.Equal(now) || now.Sub(t0) != 7*24*time.Hour {
		t.Fatalf("7d window: %v %v %v", t0, t1, clamped)
	}
	t0, _, clamped = fixedWindow(now, 60*24*time.Hour, maxAvailabilityLookback)
	if !clamped || now.Sub(t0) >= maxAvailabilityLookback {
		t.Fatalf("over-max window must clamp inside the lookback: %v %v", now.Sub(t0), clamped)
	}
}

func TestSummarizeAdminsGroupsByEmail(t *testing.T) {
	now := time.Date(2026, 10, 13, 0, 0, 0, 0, time.UTC)
	byOrg := map[fleetOrg][]rawAdmin{
		{ID: "1", Name: "One"}:   {{Email: "a@x.com", Name: "A", OrgAccess: "full", AccountStatus: "ok", TwoFactorAuthEnabled: true, LastActive: "2026-10-12T00:00:00Z"}},
		{ID: "2", Name: "Two"}:   {{Email: "A@X.com", Name: "A", OrgAccess: "read-only", AccountStatus: "ok", TwoFactorAuthEnabled: false, HasAPIKey: true, LastActive: "2026-10-10T00:00:00Z"}},
		{ID: "3", Name: "Three"}: {{Email: "b@x.com", Name: "B", OrgAccess: "full", AccountStatus: "ok", TwoFactorAuthEnabled: true, LastActive: "2026-01-01T00:00:00Z"}},
	}
	people := summarizeAdmins(byOrg, 90*24*time.Hour, now)
	if len(people) != 2 {
		t.Fatalf("want 2 people, got %d", len(people))
	}
	a := people[0]
	if a.OrgCount != 2 || a.FullAccessOrgs != 1 || a.No2FAOrgs != 1 || !a.HasAPIKey || a.Stale {
		t.Errorf("a: %+v", a)
	}
	if b := people[1]; !b.Stale || len(b.Orgs) != 1 {
		t.Errorf("b should be stale: %+v", b)
	}
}

func TestDiffSettings(t *testing.T) {
	current := map[string]any{"servers": []any{}, "untouched": 1.0}
	changed, cur := diffSettings(current, map[string]any{"servers": []any{map[string]any{"host": "h"}}})
	if len(changed) != 1 || changed[0] != "servers" || len(cur) != 1 {
		t.Fatalf("changed=%v cur=%v", changed, cur)
	}
	if same, _ := diffSettings(current, map[string]any{"servers": []any{}}); len(same) != 0 {
		t.Fatalf("equal value reported as changed: %v", same)
	}
}

func TestPathValidators(t *testing.T) {
	if p, err := normalizeFanoutPath("/api/v1/organizations/{organizationId}/networks"); err != nil || p != "/organizations/{organizationId}/networks" {
		t.Errorf("fanout path: %q %v", p, err)
	}
	for _, bad := range []string{"networks/{networkId}/clients", "organizations/{organizationId}/networks/{networkId}", "organizations/{organizationId}/x?y=1"} {
		if _, err := normalizeFanoutPath(bad); err == nil {
			t.Errorf("fanout accepted %q", bad)
		}
	}
	if _, err := validatePushPath("/networks/{networkId}/syslogServers"); err != nil {
		t.Errorf("push path rejected: %v", err)
	}
	for _, bad := range []string{"/organizations/{organizationId}/admins", "/networks/{networkId}/ssids/{number}"} {
		if _, err := validatePushPath(bad); err == nil {
			t.Errorf("push accepted %q", bad)
		}
	}
}

func TestRenewalHelpers(t *testing.T) {
	now := time.Date(2026, 10, 13, 0, 0, 0, 0, time.UTC)
	org := fleetOrg{ID: "1", Name: "O", LicensingModel: "per-device"}
	raws := []json.RawMessage{
		json.RawMessage(`{"licenseType":"MX","state":"active","expirationDate":"2026-11-12T00:00:00Z"}`),
		json.RawMessage(`{"licenseType":"MX","state":"active","expirationDate":"2026-11-12T00:00:00Z"}`),
		json.RawMessage(`{"licenseType":"MS","state":"unused","expirationDate":"2026-10-20T00:00:00Z"}`),
	}
	rows := renewalsFromLicenses(org, raws, now)
	if len(rows) != 1 || rows[0].DeviceCount != 2 || rows[0].DaysRemaining != 30 {
		t.Fatalf("licenses grouped wrong: %+v", rows)
	}
	all := append(rows, renewalRow{Kind: "end-of-support", DaysRemaining: -5, at: now.Add(-5 * 24 * time.Hour)}, renewalRow{Kind: "license", DaysRemaining: 400, at: now.Add(400 * 24 * time.Hour)})
	kept := filterRenewals(all, now, 90*24*time.Hour, true, nil)
	if len(kept) != 2 || kept[0].DaysRemaining != -5 {
		t.Fatalf("filter/sort: %+v", kept)
	}
	// Exact cutoff: a 36h deadline is outside --within 24h even though it rounds to 1 day.
	near := []renewalRow{{Kind: "license", DaysRemaining: 1, at: now.Add(36 * time.Hour)}}
	if got := filterRenewals(near, now, 24*time.Hour, true, nil); len(got) != 0 {
		t.Fatalf("36h deadline leaked into --within 24h: %+v", got)
	}
	if none := filterRenewals(all, now, 90*24*time.Hour, false, map[string]bool{"subscription": true}); len(none) != 0 {
		t.Fatalf("kind filter leaked: %+v", none)
	}
}

func TestGroupChanges(t *testing.T) {
	rows := []changeRow{
		{TS: "2026-10-01T00:00:00Z", AdminEmail: "a@x", OrganizationName: "O1", NetworkID: "N_1"},
		{TS: "2026-10-03T00:00:00Z", AdminEmail: "a@x", OrganizationName: "O2", NetworkID: "N_2"},
		{TS: "2026-10-02T00:00:00Z", AdminEmail: "b@x", OrganizationName: "O1"},
	}
	g := groupChanges(rows, "admin")
	if len(g) != 2 || g[0].Key != "a@x" || g[0].Changes != 2 || len(g[0].Organizations) != 2 || g[0].Networks != 2 || g[0].FirstTS != "2026-10-01T00:00:00Z" || g[0].LastTS != "2026-10-03T00:00:00Z" {
		t.Fatalf("group: %+v", g)
	}
}

func TestDownRows(t *testing.T) {
	now := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	org := fleetOrg{ID: "1", Name: "O"}
	nets := map[string]string{"N_1": "Main"}
	rows := downRowsFromStatuses(org, nets, []downStatus{
		{Serial: "A", Status: "offline", NetworkID: "N_1", LastReportedAt: "2026-10-13T10:00:00Z"},
		{Serial: "B", Status: "online", NetworkID: "N_1"},
		{Serial: "C", Status: "alerting", NetworkID: "N_1", LastReportedAt: "2026-10-13T11:59:00Z"},
	}, now)
	if len(rows) != 2 || rows[0].DownMinutes == nil || *rows[0].DownMinutes != 120 || rows[0].NetworkName != "Main" || rows[1].DownMinutes != nil {
		t.Fatalf("status rows: %+v", rows)
	}
	var dev downUplinkDevice
	_ = json.Unmarshal([]byte(`{"serial":"M","networkId":"N_1","uplinks":[{"interface":"wan1","status":"failed"},{"interface":"wan2","status":"not connected"},{"interface":"wan3","status":"active"}]}`), &dev)
	if got := downRowsFromUplinks(org, nets, []downUplinkDevice{dev}, false, now); len(got) != 1 || got[0].Interface != "wan1" {
		t.Fatalf("uplinks default: %+v", got)
	}
	if got := downRowsFromUplinks(org, nets, []downUplinkDevice{dev}, true, now); len(got) != 2 {
		t.Fatalf("uplinks with not-connected: %+v", got)
	}
}

func TestRenewalGroupKeepsEarliestDeadline(t *testing.T) {
	now := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	org := fleetOrg{ID: "1", Name: "O"}
	a := json.RawMessage(`{"licenseType":"MX","state":"active","expirationDate":"2026-11-12T01:00:00Z"}`)
	b := json.RawMessage(`{"licenseType":"MX","state":"active","expirationDate":"2026-11-12T23:00:00Z"}`)
	x := renewalsFromLicenses(org, []json.RawMessage{a, b}, now)
	y := renewalsFromLicenses(org, []json.RawMessage{b, a}, now)
	if len(x) != 1 || x[0].DaysRemaining != y[0].DaysRemaining {
		t.Fatalf("input order changed days_remaining: %+v vs %+v", x, y)
	}
}

func TestComputeFlapsUsesNewestMetadata(t *testing.T) {
	now := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	newer := hc("A", "2026-10-12T10:00:00Z", "online", "offline", "N_new")
	older := hc("A", "2026-10-11T10:00:00Z", "offline", "online", "N_old")
	rows := computeFlaps(fleetOrg{ID: "1"}, []availabilityChange{newer, older}, now.Add(-7*24*time.Hour), now)
	if len(rows) != 1 || rows[0].NetworkID != "N_new" {
		t.Fatalf("metadata must come from the newest event: %+v", rows)
	}
}

func TestGroupChangesKeepsSameNamedEntitiesApart(t *testing.T) {
	rows := []changeRow{
		{TS: "2026-10-01T00:00:00Z", OrganizationID: "1", OrganizationName: "Acme", NetworkID: "N_1", NetworkName: "Main Office"},
		{TS: "2026-10-02T00:00:00Z", OrganizationID: "2", OrganizationName: "Acme", NetworkID: "N_2", NetworkName: "Main Office"},
	}
	if g := groupChanges(rows, "org"); len(g) != 2 {
		t.Fatalf("two orgs named Acme merged: %+v", g)
	}
	if g := groupChanges(rows, "network"); len(g) != 2 || g[0].ID == g[1].ID {
		t.Fatalf("two networks named Main Office merged: %+v", g)
	}
}
