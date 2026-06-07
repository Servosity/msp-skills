// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
//
// Behavioral tests for the reprint-added transcendence helpers (systems
// history, detections failures, inspectors coverage, health rollup). Same
// approach as novel_liongard_test.go: realistic Liongard object shapes
// (nested {ID,Name} refs, mixed timestamp formats), assert the join outputs.

package cli

import (
	"testing"
	"time"
)

func estateEnvs() []map[string]any {
	return []map[string]any{
		{"ID": 1.0, "Name": "Acme"},
		{"ID": 2.0, "Name": "Beta"},
		{"ID": 3.0, "Name": "EmptyCo"},
	}
}

func TestFilterSystemHistory(t *testing.T) {
	now := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)
	timeline := []map[string]any{
		{"ID": 1000.0, "Status": "Completed", "System": ref(10, "M365 Tenant"),
			"Launchpoint": ref(100, "M365 LP"), "Environment": ref(1, "Acme"),
			"FinishedAt": "2026/06/05 09:00:00"},
		{"ID": 1001.0, "Status": "Failed", "System": ref(10, "M365 Tenant"),
			"Environment": ref(1, "Acme"), "FinishedAt": "2026-06-01T08:00:00Z"},
		{"ID": 1002.0, "Status": "Completed", "System": ref(99, "Other Box"),
			"FinishedAt": "2026-06-05T10:00:00Z"}, // different system — excluded
	}
	detections := []map[string]any{
		{"ID": 2000.0, "Name": "Admin role added", "System": ref(10, "M365 Tenant"),
			"Environment": ref(1, "Acme"), "CreatedOn": "2026-06-06T01:00:00Z"},
		{"ID": 2001.0, "Name": "Old change", "System": ref(10, "M365 Tenant"),
			"CreatedOn": "2026-01-01T00:00:00Z"},
	}

	all := filterSystemHistory(timeline, detections, estateEnvs(), "10", 0, now, 0)
	if len(all) != 4 {
		t.Fatalf("want 4 events for system 10, got %d: %+v", len(all), all)
	}
	// Newest first: detection 2000 (06-06) > timeline 1000 (06-05) > timeline 1001 (06-01) > detection 2001 (01-01)
	if all[0].ID != "2000" || all[0].Kind != "detection" {
		t.Fatalf("newest event should be detection 2000, got %+v", all[0])
	}
	if all[1].ID != "1000" || all[1].Kind != "inspection" {
		t.Fatalf("second event should be inspection 1000 (slash-date parsed), got %+v", all[1])
	}
	if all[0].Environment != "Acme" {
		t.Fatalf("environment should resolve to Acme, got %q", all[0].Environment)
	}

	// Windowed: last 7d excludes the January detection and keeps 3.
	week := filterSystemHistory(timeline, detections, estateEnvs(), "10", 7*24*time.Hour, now, 0)
	if len(week) != 3 {
		t.Fatalf("want 3 events in 7d window, got %d: %+v", len(week), week)
	}
	// Limit caps.
	capped := filterSystemHistory(timeline, detections, estateEnvs(), "10", 0, now, 2)
	if len(capped) != 2 {
		t.Fatalf("want 2 events with limit 2, got %d", len(capped))
	}
	// Unknown system: honest empty.
	none := filterSystemHistory(timeline, detections, estateEnvs(), "777", 0, now, 0)
	if len(none) != 0 {
		t.Fatalf("unknown system should yield 0 events, got %d", len(none))
	}
}

func TestFilterFailedInspections(t *testing.T) {
	now := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)
	launchpoints := []map[string]any{
		{"ID": 100.0, "Alias": "M365 LP", "Environment": ref(1, "Acme"), "Inspector": ref(5, "Microsoft 365")},
	}
	timeline := []map[string]any{
		// Failed, env carried directly.
		{"ID": 1001.0, "Status": "Failed", "System": ref(10, "M365 Tenant"),
			"Environment": ref(1, "Acme"), "FinishedAt": "2026-06-05T08:00:00Z"},
		// Errored, env resolved via launchpoint 100.
		{"ID": 1002.0, "Status": "Error", "Launchpoint": ref(100, "M365 LP"),
			"FinishedAt": "2026-06-04T08:00:00Z"},
		// Clean run — excluded.
		{"ID": 1003.0, "Status": "Completed", "Environment": ref(1, "Acme"),
			"FinishedAt": "2026-06-05T09:00:00Z"},
		// Old failure outside the window.
		{"ID": 1004.0, "Status": "Failed", "Environment": ref(2, "Beta"),
			"FinishedAt": "2025-01-01T00:00:00Z"},
	}

	rows := filterFailedInspections(timeline, launchpoints, estateEnvs(), 7*24*time.Hour, now, "", 0)
	if len(rows) != 2 {
		t.Fatalf("want 2 failed inspections in window, got %d: %+v", len(rows), rows)
	}
	if rows[0].TimelineID != "1001" {
		t.Fatalf("newest failure first, want 1001, got %+v", rows[0])
	}
	var viaLP *failedInspectionRow
	for i := range rows {
		if rows[i].TimelineID == "1002" {
			viaLP = &rows[i]
		}
	}
	if viaLP == nil {
		t.Fatalf("errored run 1002 missing: %+v", rows)
	}
	if viaLP.Environment != "Acme" || viaLP.Inspector != "Microsoft 365" {
		t.Fatalf("1002 should resolve env+inspector via launchpoint, got %+v", viaLP)
	}

	// All-history (since=0) picks up the 2025 failure too.
	allRows := filterFailedInspections(timeline, launchpoints, estateEnvs(), 0, now, "", 0)
	if len(allRows) != 3 {
		t.Fatalf("want 3 failures with no window, got %d", len(allRows))
	}
	// envFilter restricts.
	beta := filterFailedInspections(timeline, launchpoints, estateEnvs(), 0, now, "2", 0)
	if len(beta) != 1 || beta[0].TimelineID != "1004" {
		t.Fatalf("env filter 2 should yield only 1004, got %+v", beta)
	}
}

func TestInspectionFailed(t *testing.T) {
	cases := map[string]bool{
		"Failed": true, "Error": true, "Errored": true, "failure": true,
		"Completed": false, "Success": false, "Running": false, "": false,
	}
	for in, want := range cases {
		if got := inspectionFailed(in); got != want {
			t.Errorf("inspectionFailed(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestComputeInspectorCoverage(t *testing.T) {
	inspectors := []map[string]any{
		{"ID": 5.0, "Name": "Microsoft 365"},
		{"ID": 6.0, "Name": "Active Directory"},
	}
	launchpoints := []map[string]any{
		// M365 bound in Acme only.
		{"ID": 100.0, "Environment": ref(1, "Acme"), "Inspector": ref(5, "Microsoft 365")},
	}

	rows := computeInspectorCoverage(inspectors, launchpoints, estateEnvs(), "")
	if len(rows) != 2 {
		t.Fatalf("want 2 inspector rows, got %d", len(rows))
	}
	// AD has 3 missing (never bound) and sorts first; M365 has 2 missing.
	if rows[0].Inspector != "Active Directory" || rows[0].EnvironmentsMissing != 3 || rows[0].EnvironmentsCovered != 0 {
		t.Fatalf("AD should lead with 3 missing, got %+v", rows[0])
	}
	if rows[1].Inspector != "Microsoft 365" || rows[1].EnvironmentsCovered != 1 || rows[1].EnvironmentsMissing != 2 {
		t.Fatalf("M365 should cover 1 / miss 2, got %+v", rows[1])
	}
	// Missing list excludes the covered env and is alphabetical.
	for _, e := range rows[1].MissingEnvironments {
		if e.Environment == "Acme" {
			t.Fatalf("Acme is covered for M365, must not be in missing: %+v", rows[1])
		}
	}

	// Query filter narrows by case-insensitive substring.
	m365 := computeInspectorCoverage(inspectors, launchpoints, estateEnvs(), "microsoft")
	if len(m365) != 1 || m365[0].Inspector != "Microsoft 365" {
		t.Fatalf("query 'microsoft' should match only M365, got %+v", m365)
	}
	// No match: honest empty.
	none := computeInspectorCoverage(inspectors, launchpoints, estateEnvs(), "zzz")
	if len(none) != 0 {
		t.Fatalf("query 'zzz' should match nothing, got %+v", none)
	}
}

func TestSummarizeHealth(t *testing.T) {
	healthy := summarizeHealth(0, 0, 0, 0, 0, "7d", "7d")
	if healthy.Status != "healthy" || healthy.TotalIssues != 0 {
		t.Fatalf("zero counts should be healthy, got %+v", healthy)
	}
	sick := summarizeHealth(2, 1, 3, 0, 1, "24h", "")
	if sick.Status != "issues" || sick.TotalIssues != 7 {
		t.Fatalf("want issues/7, got %+v", sick)
	}
	if sick.StaleOlderThan != "24h" {
		t.Fatalf("threshold echo lost: %+v", sick)
	}
}

// TestCanonicalTimestampKeyLists locks the shared timestamp-key vars: objects
// dated ONLY via CompletedAt (inspection) or UpdatedOn (detection) — the two
// keys that drifted out of the environments-overview copies before the lists
// were promoted to package vars — must resolve at every consumer.
func TestCanonicalTimestampKeyLists(t *testing.T) {
	now := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)
	timeline := []map[string]any{
		{"ID": 1100.0, "Status": "Failed", "System": ref(10, "M365 Tenant"),
			"Environment": ref(1, "Acme"), "CompletedAt": "2026-06-05T08:00:00Z"},
	}
	detections := []map[string]any{
		{"ID": 2100.0, "Name": "Drift via UpdatedOn", "System": ref(10, "M365 Tenant"),
			"Environment": ref(1, "Acme"), "UpdatedOn": "2026-06-05T09:00:00Z"},
	}
	events := filterSystemHistory(timeline, detections, estateEnvs(), "10", 7*24*time.Hour, now, 0)
	if len(events) != 2 {
		t.Fatalf("CompletedAt-only and UpdatedOn-only events must both resolve, got %d: %+v", len(events), events)
	}
	failed := filterFailedInspections(timeline, nil, estateEnvs(), 7*24*time.Hour, now, "", 0)
	if len(failed) != 1 || failed[0].FinishedAt == "" {
		t.Fatalf("CompletedAt-only failed inspection must resolve a timestamp, got %+v", failed)
	}
	if got, ok := lgTime(detections[0], tsKeysDetection...); !ok || got.IsZero() {
		t.Fatalf("tsKeysDetection must include UpdatedOn")
	}
	if got, ok := lgTime(timeline[0], tsKeysInspection...); !ok || got.IsZero() {
		t.Fatalf("tsKeysInspection must include CompletedAt")
	}
}

// TestLgTimeNonUTCOffset locks the lgTime UTC normalization: an RFC3339 input
// carrying a non-Z offset must resolve to the UTC instant so formatted sort
// keys order by instant, not by zoned-string lexicography.
func TestLgTimeNonUTCOffset(t *testing.T) {
	obj := map[string]any{"FinishedAt": "2021-09-10T19:21:03-05:00"}
	ts, ok := lgTime(obj, tsKeysInspection...)
	if !ok || ts.Format(time.RFC3339) != "2021-09-11T00:21:03Z" {
		t.Fatalf("non-Z offset must normalize to the UTC instant, got %v ok=%v", ts, ok)
	}
}
