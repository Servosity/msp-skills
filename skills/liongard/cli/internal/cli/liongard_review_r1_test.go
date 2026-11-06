// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// Codex review round 1 (liongard 4.34.2 reprint): classes found in the
// transport guard and the store-backed novels. Each test was seen failing on
// the pre-fix code (seen/r1-<class>-seen-failing.log in the run dir).

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"liongard-pp-cli/internal/store"
)

// r1Store opens a temp store with the given records ({resource: {id: object}}).
func r1Store(t *testing.T, records map[string]map[string]map[string]any) *store.Store {
	t.Helper()
	db, err := store.OpenWithContext(context.Background(), filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	for rt, objs := range records {
		for id, obj := range objs {
			raw, _ := json.Marshal(obj)
			if err := db.Upsert(rt, id, raw); err != nil {
				t.Fatalf("upsert %s/%s: %v", rt, id, err)
			}
		}
	}
	return db
}

// Class incomplete-snapshot-read-as-complete: launchpoints stale / run-stale
// read "no timeline row" as "never inspected" whether or not the timeline was
// ever synced, and ignored the launchpoint's own documented last-run
// timestamps (components/schemas/Launchpoints LastScheduledRun /
// LastAdHocRun), so run-stale --confirm triggered an inspection on a
// launchpoint that ran an hour ago.
func TestR1StaleNeedsACompleteSnapshot(t *testing.T) {
	recent := time.Now().UTC().Add(-time.Hour).Format("2006/01/02 15:04:05")
	old := time.Now().UTC().Add(-30 * 24 * time.Hour).Format("2006-01-02T15:04:05.000Z")
	lps := map[string]map[string]any{
		"1203": {"ID": 1203, "Alias": "AD", "Environment": map[string]any{"ID": 42}, "Inspector": map[string]any{"ID": 13},
			"LastScheduledRun": recent, "LastAdHocRun": nil, "TotalScheduledRuns": 12, "TotalAdHocRuns": 0},
		"1204": {"ID": 1204, "Alias": "M365", "Environment": map[string]any{"ID": 42}, "Inspector": map[string]any{"ID": 14},
			"LastScheduledRun": nil, "LastAdHocRun": old, "TotalScheduledRuns": 0, "TotalAdHocRuns": 1},
	}

	// Launchpoints in the store (a live read cached them), timeline never synced.
	db := r1Store(t, map[string]map[string]map[string]any{"launchpoints": lps})
	stale, err := computeStaleLaunchpoints(db, 7*24*time.Hour, "")
	for _, s := range stale {
		if s.LaunchpointID == "1203" {
			t.Errorf("unsynced timeline: launchpoint 1203 (ran an hour ago) planned as stale: %+v (err %v)", s, err)
		}
	}

	// Every resource synced, the timeline empty: the launchpoint's own
	// documented last-run timestamps decide.
	db = r1Store(t, map[string]map[string]map[string]any{"launchpoints": lps})
	markFullSnapshot(t, db, rtLaunchpoints, rtTimeline, rtEnvironments)
	stale, err = computeStaleLaunchpoints(db, 7*24*time.Hour, "")
	if err != nil {
		t.Fatalf("complete snapshot refused: %v", err)
	}
	var sawOld bool
	for _, s := range stale {
		switch s.LaunchpointID {
		case "1203":
			t.Errorf("launchpoint 1203 ran an hour ago (LastScheduledRun) but is stale: %+v", s)
		case "1204":
			sawOld = true
			if s.NeverInspected || s.DaysStale < 29 {
				t.Errorf("launchpoint 1204 last ran 30 days ago (LastAdHocRun): %+v", s)
			}
		}
	}
	if !sawOld {
		t.Errorf("launchpoint 1204 (last ran 30 days ago) is not stale: %+v", stale)
	}
}

// Every incomplete-snapshot signal refuses: no sync record, a sync that did
// not finish, a finished flag without a sync time, a finished sync not
// recorded as a full snapshot (r2), and a resource sync cannot fetch at all.
func TestR1EveryIncompleteSnapshotSignalRefuses(t *testing.T) {
	db := r1Store(t, map[string]map[string]map[string]any{"agents": {"7": {"ID": 7, "Status": "Offline", "Environment": nil, "LastHeartbeat": nil}}})
	if _, err := loadObjs(db, rtAgents); err == nil {
		t.Errorf("agents with no sync record were read")
	}
	if err := db.SaveSyncProgress(rtAgents, "c", 1); err != nil {
		t.Fatal(err)
	}
	if _, err := loadObjs(db, rtAgents); err == nil {
		t.Errorf("agents whose last sync did not finish were read")
	}
	if _, err := db.DB().Exec(`UPDATE sync_state SET last_attempt_complete = 1, last_synced_at = NULL WHERE resource_type = ?`, rtAgents); err != nil {
		t.Fatal(err)
	}
	if _, err := loadObjs(db, rtAgents); err == nil {
		t.Errorf("agents marked complete with no sync time were read")
	}
	// A generated writer's "complete" does not say the walk was full.
	if err := db.SaveSyncState(rtAgents, "", 1); err != nil {
		t.Fatal(err)
	}
	if _, err := loadObjs(db, rtAgents); err == nil || !strings.Contains(err.Error(), "narrowed") {
		t.Errorf("agents finished without a full-snapshot verdict were read: %v", err)
	}
	markFullSnapshot(t, db, rtAgents)
	if objs, err := loadObjs(db, rtAgents); err != nil || len(objs) != 1 {
		t.Errorf("a complete agents snapshot was refused: %v %v", objs, err)
	}
	if _, err := loadObjs(db, "groups"); err == nil || !strings.Contains(err.Error(), "does not fetch") {
		t.Errorf("groups (sync cannot fetch them) were read: %v", err)
	}
}

// The newest known run per launchpoint takes the timeline and the
// launchpoint's documented LastScheduledRun / LastAdHocRun, whichever is
// newer.
func TestR1LastRunByLaunchpointUsesDocumentedFields(t *testing.T) {
	timeline := []map[string]any{{"ID": 1, "Launchpoint": map[string]any{"ID": 9}, "FinishedAt": "2026-10-01T00:00:00Z"}}
	lps := []map[string]any{
		{"ID": 9, "LastScheduledRun": "2026/10/20 08:00:00"},
		{"ID": 10, "LastAdHocRun": "2026-10-21T08:00:00.000Z", "LastScheduledRun": "2026/09/01 00:00:00"},
		{"ID": 11},
	}
	got := lastRunByLaunchpoint(timeline, lps)
	if got["9"].Format("2006-01-02") != "2026-10-20" || got["10"].Format("2006-01-02") != "2026-10-21" {
		t.Errorf("newest run per launchpoint: %v", got)
	}
	if _, ok := got["11"]; ok {
		t.Errorf("a launchpoint with no run evidence got a last run: %v", got)
	}
}

// A launchpoint whose documented run counters say it ran, with no run time
// anywhere, is not "never inspected" and is not a run target.
func TestR1RunCountersAreRunEvidence(t *testing.T) {
	db := r1Store(t, map[string]map[string]map[string]any{"launchpoints": {
		"1301": {"ID": 1301, "Environment": nil, "Inspector": nil, "LastScheduledRun": nil, "LastAdHocRun": nil, "TotalScheduledRuns": 249, "TotalAdHocRuns": 0},
		"1302": {"ID": 1302, "Environment": nil, "Inspector": nil, "LastScheduledRun": nil, "LastAdHocRun": nil, "TotalScheduledRuns": 0, "TotalAdHocRuns": 0},
	}})
	markFullSnapshot(t, db, rtLaunchpoints, rtTimeline, rtEnvironments)
	stale, err := computeStaleLaunchpoints(db, 7*24*time.Hour, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(stale) != 1 || stale[0].LaunchpointID != "1302" || !stale[0].NeverInspected {
		t.Errorf("want only 1302 (no runs) never inspected, got %+v", stale)
	}
}

// Class documented-agent-shape-never-classified (Codex r1 P2 raised to P1 by
// the coordinator under the work order's scope: components/schemas/Agents
// documents an integer Status and a LastHeartbeat timestamp): the offline
// classifier read only invented booleans and status words, so a documented
// agent whose last heartbeat is days old was never offline in `agents
// offline`, the `health` count or the environments overview.
func TestR1DocumentedAgentShapeIsClassified(t *testing.T) {
	now := time.Now().UTC()
	envs := []map[string]any{{"ID": 42, "Name": "Acme"}}
	silent := map[string]any{"ID": 1511, "Environment": map[string]any{"ID": 42}, "Name": "Acme Agent", "Status": 1,
		"Suspended": false, "LastHeartbeat": now.Add(-72 * time.Hour).Format("2006/01/02 15:04:05")}
	alive := map[string]any{"ID": 1512, "Environment": map[string]any{"ID": 42}, "Name": "Acme Agent 2", "Status": 1,
		"Suspended": false, "LastHeartbeat": now.Add(-2 * time.Minute).Format("2006-01-02T15:04:05.000Z")}
	if !agentIsOffline(silent) {
		t.Errorf("an agent silent for three days (documented LastHeartbeat) is not offline")
	}
	if agentIsOffline(alive) {
		t.Errorf("an agent that checked in two minutes ago is offline")
	}
	rows := filterOfflineAgents([]map[string]any{silent, alive}, envs, "")
	if len(rows) != 1 || rows[0].AgentID != "1511" {
		t.Errorf("agents offline / health want only agent 1511, got %+v", rows)
	}
}
