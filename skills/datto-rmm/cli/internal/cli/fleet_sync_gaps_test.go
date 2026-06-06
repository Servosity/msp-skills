// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import "testing"

func TestComputeSyncGaps(t *testing.T) {
	rowCounts := map[string]int{
		"account-devices":     100,
		"account-alerts-open": 40,
		"account":             12,
	}
	states := map[string]syncStateRow{
		"account-devices":     {lastSyncedAt: "2026-06-05T00:00:00Z", totalCount: 100},
		"account-alerts-open": {lastSyncedAt: "2026-06-05T00:00:00Z", totalCount: 55},
		"activity-logs":       {lastSyncedAt: "2026-06-01T00:00:00Z", totalCount: 9},
	}

	rows := computeSyncGaps(rowCounts, states)
	byName := map[string]syncGapRow{}
	for _, r := range rows {
		byName[r.Resource] = r
	}

	if r := byName["account-devices"]; r.Status != "ok" || r.Drift != 0 {
		t.Fatalf("devices row wrong: %+v", r)
	}
	if r := byName["account-alerts-open"]; r.Status != "drift" || r.Drift != -15 {
		t.Fatalf("alerts row wrong: %+v", r)
	}
	if r := byName["account"]; r.Status != "never-synced" {
		t.Fatalf("sites row wrong: %+v", r)
	}
	// Synced-then-emptied resource still surfaces (union of both sides).
	if r := byName["activity-logs"]; r.StoredRows != 0 || r.Status != "drift" || r.Drift != -9 {
		t.Fatalf("activity-logs row wrong: %+v", r)
	}
}

func TestComputeSyncGapsEmptyStoreIsHonest(t *testing.T) {
	rows := computeSyncGaps(map[string]int{}, map[string]syncStateRow{})
	if len(rows) != 0 {
		t.Fatalf("empty inputs fabricated rows: %+v", rows)
	}
}
