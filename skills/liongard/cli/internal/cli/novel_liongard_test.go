// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
//
// Behavioral tests for the Liongard store-backed transcendence commands. Because
// the build is credential-less, these seed a temporary store with realistic
// Liongard object shapes (nested {ID,Name} references, slash-format timestamps)
// and assert the join outputs — the oracle that the cross-entity joins resolve
// correctly against the real data model before any live tenant USE.

package cli

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"liongard-pp-cli/internal/store"
)

func ref(id float64, name string) map[string]any {
	return map[string]any{"ID": id, "Name": name}
}

// seedStore writes a small but representative estate into a temp store.
func seedStore(t *testing.T) *store.Store {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "data.db")
	db, err := store.OpenWithContext(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	put := func(rt, id string, obj map[string]any) {
		raw, err := json.Marshal(obj)
		if err != nil {
			t.Fatalf("marshal %s/%s: %v", rt, id, err)
		}
		if err := db.Upsert(rt, id, raw); err != nil {
			t.Fatalf("upsert %s/%s: %v", rt, id, err)
		}
	}

	put("environments", "1", map[string]any{"ID": 1.0, "Name": "Acme"})
	put("environments", "2", map[string]any{"ID": 2.0, "Name": "Beta"})
	put("environments", "3", map[string]any{"ID": 3.0, "Name": "EmptyCo"}) // no systems

	// sys 10: covered (has Launchpoint 100), inspector 5, env Acme
	put("systems", "10", map[string]any{
		"ID": 10.0, "Name": "M365 Tenant", "Status": "Active",
		"Environment": ref(1, "Acme"), "Inspector": ref(5, "Microsoft 365"),
		"Launchpoint": ref(100, "M365 LP"),
	})
	// sys 11: UNINSPECTED (no Launchpoint), inspector 6, env Acme
	put("systems", "11", map[string]any{
		"ID": 11.0, "Name": "AD Forest", "Status": "Active",
		"Environment": ref(1, "Acme"), "Inspector": ref(6, "Active Directory"),
	})

	put("launchpoints", "100", map[string]any{
		"ID": 100.0, "Alias": "M365 LP", "Status": "Active",
		"Environment": ref(1, "Acme"), "Inspector": ref(5, "Microsoft 365"),
	})
	put("launchpoints", "101", map[string]any{ // never inspected (no timeline)
		"ID": 101.0, "Alias": "Idle LP", "Status": "Active",
		"Environment": ref(1, "Acme"), "Inspector": ref(7, "DNS"),
	})

	// One old inspection for LP 100 (slash date format).
	put("timeline", "1000", map[string]any{
		"ID": 1000.0, "Status": "Completed",
		"Launchpoint": ref(100, "M365 LP"), "System": ref(10, "M365 Tenant"),
		"Environment": ref(1, "Acme"),
		"CreatedOn":   "2021/09/10 19:21:03", "FinishedAt": "2021/09/10 19:25:00",
	})

	put("agents", "200", map[string]any{
		"ID": 200.0, "Name": "agent-acme", "Status": "Active", "Hostname": "acme-01",
		"Environment": ref(1, "Acme"),
	})
	put("agents", "201", map[string]any{
		"ID": 201.0, "Name": "agent-beta", "Status": "Offline", "Hostname": "beta-01",
		"Environment": ref(2, "Beta"),
	})

	put("metrics", "400", map[string]any{
		"ID": 400.0, "Name": "MFA Enabled Count",
		"Inspector": ref(5, "Microsoft 365"), "Environment": ref(1, "Acme"),
	})
	return db
}

func TestComputeStaleLaunchpoints(t *testing.T) {
	db := seedStore(t)
	stale, err := computeStaleLaunchpoints(db, time.Hour, "")
	if err != nil {
		t.Fatalf("computeStaleLaunchpoints: %v", err)
	}
	// LP 100 inspected in 2021 (stale) + LP 101 never inspected = 2 results.
	if len(stale) != 2 {
		t.Fatalf("want 2 stale launchpoints, got %d: %+v", len(stale), stale)
	}
	var never, old *staleLaunchpoint
	for i := range stale {
		switch stale[i].LaunchpointID {
		case "101":
			never = &stale[i]
		case "100":
			old = &stale[i]
		}
	}
	if never == nil || !never.NeverInspected {
		t.Fatalf("LP 101 should be never-inspected: %+v", stale)
	}
	if old == nil || old.NeverInspected || old.DaysStale < 365 {
		t.Fatalf("LP 100 should be stale by >365 days: %+v", old)
	}
	if old != nil && old.Environment != "Acme" {
		t.Fatalf("LP 100 environment should resolve to Acme, got %q", old.Environment)
	}

	// envFilter restricts; both LPs are in env 1, none in env 2.
	none, _ := computeStaleLaunchpoints(db, time.Hour, "2")
	if len(none) != 0 {
		t.Fatalf("env filter 2 should yield 0, got %d", len(none))
	}
}

func TestBuildMetricRows(t *testing.T) {
	db := seedStore(t)
	rows, matched, anyValue, err := buildMetricRows(db, "mfa")
	if err != nil {
		t.Fatalf("buildMetricRows: %v", err)
	}
	if matched != 1 {
		t.Fatalf("want 1 matched metric definition, got %d", matched)
	}
	// Metric 400 (inspector 5, env 1) covers system 10 (inspector 5, env 1) only.
	if len(rows) != 1 {
		t.Fatalf("want 1 covered-system row, got %d: %+v", len(rows), rows)
	}
	if rows[0].SystemID != "10" || rows[0].System != "M365 Tenant" {
		t.Fatalf("pivot row should be system 10 / M365 Tenant, got %+v", rows[0])
	}
	if anyValue {
		t.Fatalf("definitions carry no per-system value; anyValue should be false")
	}
	// A non-matching query returns nothing.
	if r, m, _, _ := buildMetricRows(db, "no-such-metric"); m != 0 || len(r) != 0 {
		t.Fatalf("non-matching query should yield 0 matched/0 rows, got %d/%d", m, len(r))
	}
}

func TestFilterDriftRows(t *testing.T) {
	now := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	envs := []map[string]any{ref(1, "Acme"), ref(2, "Beta")}
	detections := []map[string]any{
		{ // recent, in window
			"ID": 300.0, "Name": "M365 admin added",
			"CreatedOn":   now.Add(-2 * time.Hour).Format("2006/01/02 15:04:05"),
			"Environment": ref(1, "Acme"), "System": ref(10, "M365 Tenant"),
		},
		{ // old, outside 24h window
			"ID": 301.0, "Name": "ancient change",
			"CreatedOn":   "2020/01/01 00:00:00",
			"Environment": ref(2, "Beta"),
		},
	}
	rows := filterDriftRows(detections, envs, 24*time.Hour, now, "", 0)
	if len(rows) != 1 {
		t.Fatalf("want 1 in-window detection, got %d: %+v", len(rows), rows)
	}
	if rows[0].DetectionID != "300" || rows[0].Environment != "Acme" || rows[0].System != "M365 Tenant" {
		t.Fatalf("drift row mis-resolved: %+v", rows[0])
	}
	// env filter excludes Acme detections.
	if r := filterDriftRows(detections, envs, 24*time.Hour, now, "2", 0); len(r) != 0 {
		t.Fatalf("env filter 2 should exclude the Acme detection, got %d", len(r))
	}
}

func TestFindCoverageGaps(t *testing.T) {
	envs := []map[string]any{ref(1, "Acme"), ref(2, "Beta"), ref(3, "EmptyCo")}
	systems := []map[string]any{
		{"ID": 10.0, "Name": "M365", "Environment": ref(1, "Acme"), "Launchpoint": ref(100, "LP")},
		{"ID": 11.0, "Name": "AD", "Environment": ref(1, "Acme")}, // no launchpoint
		{"ID": 12.0, "Name": "FW", "Environment": ref(2, "Beta")}, // no launchpoint
	}
	uninspected, emptyEnvs := findCoverageGaps(systems, envs, "")
	if len(uninspected) != 2 {
		t.Fatalf("want 2 uninspected systems, got %d: %+v", len(uninspected), uninspected)
	}
	// Env 3 (EmptyCo) has no systems.
	if len(emptyEnvs) != 1 || emptyEnvs[0].EnvironmentID != "3" {
		t.Fatalf("want EmptyCo (env 3) as the only empty env, got %+v", emptyEnvs)
	}
}

func TestFilterOfflineAgents(t *testing.T) {
	envs := []map[string]any{ref(1, "Acme"), ref(2, "Beta")}
	agents := []map[string]any{
		{"ID": 200.0, "Name": "a", "Status": "Active", "Environment": ref(1, "Acme")},
		{"ID": 201.0, "Name": "b", "Status": "Offline", "Environment": ref(2, "Beta")},
		{"ID": 202.0, "Name": "c", "Online": false, "Environment": ref(2, "Beta")},
	}
	rows := filterOfflineAgents(agents, envs, "")
	if len(rows) != 2 {
		t.Fatalf("want 2 offline agents (201 by status, 202 by Online:false), got %d: %+v", len(rows), rows)
	}
}

func TestLgRefHelpers(t *testing.T) {
	obj := map[string]any{
		"Environment":   map[string]any{"ID": 7.0, "Name": "Globex"},
		"EnvironmentID": 99.0, // object form should win when both present
		"Inspector":     "13", // scalar reference
	}
	if got := lgRefID(obj, "Environment", "EnvironmentID"); got != "7" {
		t.Fatalf("lgRefID nested object: got %q want 7", got)
	}
	if got := lgRefName(obj, "Environment"); got != "Globex" {
		t.Fatalf("lgRefName nested object: got %q want Globex", got)
	}
	if got := lgRefID(obj, "Inspector"); got != "13" {
		t.Fatalf("lgRefID scalar: got %q want 13", got)
	}
	if got := lgRefID(obj, "Missing", "EnvironmentID"); got != "99" {
		t.Fatalf("lgRefID fallback to scalar EnvironmentID: got %q want 99", got)
	}
}

func TestLgTimeFormats(t *testing.T) {
	cases := map[string]bool{
		"2021/09/10 19:21:03":      true, // Liongard slash form
		"2021-09-10T19:21:03.195Z": true, // ISO
		"2021-09-10T19:21:03":      true,
		"not a date":               false,
	}
	for in, wantOK := range cases {
		_, ok := lgTime(map[string]any{"CreatedOn": in}, "CreatedOn")
		if ok != wantOK {
			t.Fatalf("lgTime(%q) ok=%v, want %v", in, ok, wantOK)
		}
	}
}

func TestParseLookbackDuration(t *testing.T) {
	cases := []struct {
		in   string
		want time.Duration
		ok   bool
	}{
		{"24h", 24 * time.Hour, true},
		{"7d", 7 * 24 * time.Hour, true},
		{"90m", 90 * time.Minute, true},
		{"", 0, false},
		{"banana", 0, false},
	}
	for _, c := range cases {
		got, err := parseLookbackDuration(c.in)
		if (err == nil) != c.ok {
			t.Fatalf("parseLookbackDuration(%q) ok=%v, want %v", c.in, err == nil, c.ok)
		}
		if c.ok && got != c.want {
			t.Fatalf("parseLookbackDuration(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestAgentIsOfflineAndCompare(t *testing.T) {
	if !agentIsOffline(map[string]any{"Status": "Offline"}) {
		t.Fatal("Status Offline should be offline")
	}
	if agentIsOffline(map[string]any{"Status": "Active"}) {
		t.Fatal("Status Active should not be offline")
	}
	if !agentIsOffline(map[string]any{"Online": false}) {
		t.Fatal("Online:false should be offline")
	}
	if agentIsOffline(map[string]any{"Status": ""}) {
		t.Fatal("empty status should not be flagged offline (no positive evidence)")
	}
	if !compareMetric(40, "gt", 30) || compareMetric(20, "gt", 30) {
		t.Fatal("compareMetric gt broken")
	}
	if !compareMetric(5, "lt", 10) || compareMetric(15, "lt", 10) {
		t.Fatal("compareMetric lt broken")
	}
}
