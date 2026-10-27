// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Behavioural acceptance for time filters: a row whose time is absent or does
// not parse never passes --since or --older-than and is never bucketed as
// young; it is counted as undated instead of being dropped. Rows are shaped
// like GetOnboardingComputerResponse (updatedDate, optional) and
// GetMaintenanceActionResponse (startTime, endTime, createdDateUTC,
// updatedDateUTC). These decode by JSON field name, so they test the
// command's contract rather than its Go structs.

package cli

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"immybot-pp-cli/internal/store"
)

type timeOnboardingOut struct {
	StalledCount int            `json:"stalled_count"`
	Undated      int            `json:"undated"`
	BucketCounts map[string]int `json:"bucket_counts"`
	Computers    []struct {
		ComputerName string   `json:"computer_name"`
		AgeDays      *float64 `json:"age_days"`
		AgeBucket    string   `json:"age_bucket"`
	} `json:"computers"`
	Note string `json:"note"`
}

type timeTriageOut struct {
	MatchedActions int `json:"matched_actions"`
	FailedActions  int `json:"failed_actions"`
	UndatedActions int `json:"undated_actions"`
	Clusters       []struct {
		Computers []string `json:"computers"`
		FirstSeen string   `json:"first_seen"`
		LastSeen  string   `json:"last_seen"`
	} `json:"clusters"`
	Note string `json:"note"`
}

func TestOnboardingStalledUndatedRowNeverPassesOlderThan(t *testing.T) {
	now := time.Now().UTC()
	db := seedStore(t, map[string][]map[string]any{
		"computers-onboarding": {
			// updatedDate is optional in GetOnboardingComputerResponse: absent here.
			{"id": 42, "computerName": "WS-42", "onboardingFailed": false},
			{"id": 43, "computerName": "WS-43", "onboardingFailed": false, "updatedDate": "not a date"},
			// default(DateTime), what a never-set .NET DateTime serializes as.
			{"id": 44, "computerName": "WS-44", "onboardingFailed": false, "updatedDate": "0001-01-01T00:00:00"},
			{"id": 45, "computerName": "WS-OLD", "onboardingFailed": false, "updatedDate": now.Add(-10 * 24 * time.Hour).Format(time.RFC3339)},
			{"id": 46, "computerName": "WS-NEW", "onboardingFailed": false, "updatedDate": now.Add(-2 * time.Hour).Format(time.RFC3339)},
		},
	})

	out, err := runCLI(t, "onboarding-stalled", "--older-than", "3d", "--json", "--db", db)
	if err != nil {
		t.Fatalf("onboarding-stalled: %v\n%s", err, out)
	}
	v := decode[timeOnboardingOut](t, out)
	names := make([]string, 0, len(v.Computers))
	for _, c := range v.Computers {
		names = append(names, c.ComputerName)
	}
	if v.StalledCount != 1 || !slices.Equal(names, []string{"WS-OLD"}) {
		t.Fatalf("--older-than 3d listed %v (stalled %d), want only WS-OLD: a row with no readable updatedDate has no age:\n%s", names, v.StalledCount, out)
	}
	if v.BucketCounts["<1d"] != 0 || v.BucketCounts["7-30d"] != 1 || v.BucketCounts["30d+"] != 0 {
		t.Fatalf("bucket_counts = %v, want only 7-30d: 1 (an undated row is not young, and default(DateTime) is not ancient):\n%s", v.BucketCounts, out)
	}
	if v.Undated != 3 {
		t.Fatalf("undated = %d, want 3 (absent, unparseable and default(DateTime) updatedDate are counted, not dropped):\n%s", v.Undated, out)
	}
	if !strings.Contains(v.Note, "3") || !strings.Contains(v.Note, "updatedDate") {
		t.Fatalf("note = %q, want it to say 3 computers had no usable updatedDate", v.Note)
	}

	// With no age filter every queued row is listed, and an undated one sits in
	// the unknown bucket with no age, after the dated rows.
	out, err = runCLI(t, "onboarding-stalled", "--limit", "0", "--json", "--db", db)
	if err != nil {
		t.Fatalf("onboarding-stalled: %v\n%s", err, out)
	}
	v = decode[timeOnboardingOut](t, out)
	if v.StalledCount != 5 || v.Undated != 0 {
		t.Fatalf("no filter: stalled %d undated %d, want 5 and 0 (nothing was left out):\n%s", v.StalledCount, v.Undated, out)
	}
	if v.BucketCounts["unknown"] != 3 || v.BucketCounts["<1d"] != 1 || v.BucketCounts["7-30d"] != 1 {
		t.Fatalf("no filter: bucket_counts = %v, want unknown 3, <1d 1, 7-30d 1:\n%s", v.BucketCounts, out)
	}
	for i, c := range v.Computers {
		undated := c.ComputerName == "WS-42" || c.ComputerName == "WS-43" || c.ComputerName == "WS-44"
		if undated && (c.AgeBucket != "unknown" || c.AgeDays != nil || i < 2) {
			t.Fatalf("undated %s at %d: bucket %q age_days %v, want unknown, null, after the dated rows:\n%s", c.ComputerName, i, c.AgeBucket, c.AgeDays, out)
		}
		if !undated && (c.AgeBucket == "unknown" || c.AgeDays == nil) {
			t.Fatalf("dated %s: bucket %q age_days %v, want a real age:\n%s", c.ComputerName, c.AgeBucket, c.AgeDays, out)
		}
	}
}

func TestSessionTriageUndatedActionNeverPassesSince(t *testing.T) {
	now := time.Now().UTC()
	old := now.Add(-30 * 24 * time.Hour).Format(time.RFC3339)
	failed := func(id int, computer string, times map[string]any) map[string]any {
		row := map[string]any{"id": id, "maintenanceSessionId": 700 + id, "computerName": computer, "tenantName": "Contoso",
			"maintenanceDisplayName": "Install Chrome", "status": "Complete", "statusName": "Complete",
			"result": "Failed", "resultName": "Failed", "resultReasonMessage": "Installer failed"}
		for k, v := range times {
			row[k] = v
		}
		return row
	}
	db := seedStore(t, map[string][]map[string]any{"maintenance-actions": {
		// Created and last written 30 days ago, never given a start or end time.
		failed(1, "WS-OLD", map[string]any{"createdDateUTC": old, "updatedDateUTC": old}),
		// No timestamp at all.
		failed(2, "WS-NODATE", nil),
		// endTime is when the outcome was decided: a later write to the row is
		// not a new failure.
		failed(3, "WS-ENDED", map[string]any{"endTime": old, "updatedDateUTC": now.Format(time.RFC3339)}),
		// Still running when mirrored: default(DateTime) endTime falls through to startTime.
		failed(4, "WS-RECENT", map[string]any{"startTime": now.Add(-time.Hour).Format(time.RFC3339), "endTime": "0001-01-01T00:00:00"}),
		// A start time that does not parse.
		failed(5, "WS-BADTIME", map[string]any{"startTime": "yesterday"}),
	}})

	out, err := runCLI(t, "session-triage", "--since", "24h", "--json", "--db", db)
	if err != nil {
		t.Fatalf("session-triage: %v\n%s", err, out)
	}
	v := decode[timeTriageOut](t, out)
	var computers []string
	for _, c := range v.Clusters {
		computers = append(computers, c.Computers...)
	}
	if v.MatchedActions != 1 || v.FailedActions != 1 || !slices.Equal(computers, []string{"WS-RECENT"}) {
		t.Fatalf("--since 24h matched %d / failed %d across %v, want 1 / 1 on WS-RECENT only:\n%s", v.MatchedActions, v.FailedActions, computers, out)
	}
	if v.UndatedActions != 2 {
		t.Fatalf("undated_actions = %d, want 2 (WS-NODATE and WS-BADTIME are counted, not dropped):\n%s", v.UndatedActions, out)
	}
	if !strings.Contains(v.Note, "2") || !strings.Contains(v.Note, "endTime") {
		t.Fatalf("note = %q, want it to name the 2 actions with no usable timestamp", v.Note)
	}

	// Without --since nothing is filtered on time, so nothing is undated.
	out, err = runCLI(t, "session-triage", "--tenant", "Contoso", "--json", "--db", db)
	if err != nil {
		t.Fatalf("session-triage: %v\n%s", err, out)
	}
	if v := decode[timeTriageOut](t, out); v.MatchedActions != 5 || v.UndatedActions != 0 {
		t.Fatalf("no --since: matched %d undated %d, want 5 and 0:\n%s", v.MatchedActions, v.UndatedActions, out)
	}
}

func TestSessionTriageClusterWindowComparesInstants(t *testing.T) {
	now := time.Now().UTC()
	// Three hours ago in UTC, and one hour ago written at UTC-10: as text the
	// later one sorts first.
	earlier := now.Add(-3 * time.Hour).Format(time.RFC3339)
	later := now.Add(-time.Hour).In(time.FixedZone("", -10*3600)).Format(time.RFC3339)
	row := func(id int, computer, end string) map[string]any {
		return map[string]any{"id": id, "computerName": computer, "tenantName": "Contoso", "maintenanceDisplayName": "Patch",
			"result": "Failed", "resultName": "Failed", "resultReasonMessage": "Reboot pending", "endTime": end}
	}
	db := seedStore(t, map[string][]map[string]any{"maintenance-actions": {row(1, "WS-01", earlier), row(2, "WS-02", later)}})
	out, err := runCLI(t, "session-triage", "--since", "24h", "--json", "--db", db)
	if err != nil {
		t.Fatalf("session-triage: %v\n%s", err, out)
	}
	v := decode[timeTriageOut](t, out)
	if len(v.Clusters) != 1 || v.Clusters[0].FirstSeen != earlier || v.Clusters[0].LastSeen != later {
		t.Fatalf("clusters = %+v, want one cluster first_seen %s last_seen %s:\n%s", v.Clusters, earlier, later, out)
	}
}

func TestFleetDiffUndatedSnapshotNeverMeetsSince(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "immy.db")
	fleetUpsert(t, dbPath, map[string][]map[string]any{
		"computers": {{"id": "1", "name": "WS-01", "tenantId": "t1", "online": true}},
	})
	recent := decode[struct {
		Taken string `json:"snapshot_taken"`
	}](t, mustRunCLI(t, "fleet-diff", "--snapshot", "--json", "--db", dbPath)).Taken
	// A snapshot whose taken_at does not parse sorts below every real time.
	const undated = "2025-13-45T99:99:99Z"
	writeFleetSnapshotAt(t, dbPath, undated)

	type out struct {
		Baseline         string `json:"baseline_snapshot"`
		SinceUnmet       bool   `json:"since_unmet"`
		UndatedSnapshots int    `json:"undated_snapshots"`
		Note             string `json:"note"`
	}
	v := decode[out](t, mustRunCLI(t, "fleet-diff", "--since", "24h", "--json", "--db", dbPath))
	if v.Baseline != recent || !v.SinceUnmet || v.UndatedSnapshots != 1 {
		t.Fatalf("baseline %q since_unmet %v undated %d, want the dated %q flagged since_unmet with 1 undated (an unknown time never stands in for --since):\n%+v",
			v.Baseline, v.SinceUnmet, v.UndatedSnapshots, recent, v)
	}
	if !strings.Contains(v.Note, "24h") {
		t.Fatalf("note = %q, want it to say no snapshot is at least 24h old", v.Note)
	}

	// A dated snapshot old enough is chosen and nothing is flagged.
	older := time.Now().UTC().Add(-48 * time.Hour).Format("2006-01-02T15:04:05.000000000Z07:00")
	writeFleetSnapshotAt(t, dbPath, older)
	v = decode[out](t, mustRunCLI(t, "fleet-diff", "--since", "24h", "--json", "--db", dbPath))
	if v.Baseline != older || v.SinceUnmet {
		t.Fatalf("baseline %q since_unmet %v, want %q and met:\n%+v", v.Baseline, v.SinceUnmet, older, v)
	}
}

func mustRunCLI(t *testing.T, args ...string) string {
	t.Helper()
	out, err := runCLI(t, args...)
	if err != nil {
		t.Fatalf("%v: %v\n%s", args, err, out)
	}
	return out
}

// writeFleetSnapshotAt records an empty baseline in this version's shape at an
// arbitrary taken_at.
func writeFleetSnapshotAt(t *testing.T, dbPath, takenAt string) {
	t.Helper()
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer db.Close()
	if err := db.WriteImmyFleetSnapshot(t.Context(), takenAt, fleetKindShapes(), nil); err != nil {
		t.Fatalf("write snapshot %s: %v", takenAt, err)
	}
}
