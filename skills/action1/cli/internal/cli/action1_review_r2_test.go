// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"testing"
	"time"

	"action1-pp-cli/internal/store"
)

// Codex r2 (action1 reprint on 4.34.2) class missing endpoint reported as
// remediated, empty-capture form: three full syncs of documented endpoint
// pages (A with 5 critical, A with 1 critical, then items:[]) and a
// patch-drift run after each. The third capture observes no endpoint, so it
// must still be the current snapshot: A is not_observed, and the earlier
// "remediated" result is never repeated.
func TestR2EmptyCaptureIsTheCurrentSnapshot(t *testing.T) {
	ep := func(crit int) map[string]any {
		return map[string]any{"id": "ep-a", "name": "ep-a", "parent_id": "org-a", "missing_updates": map[string]any{"critical": crit, "other": 0}}
	}
	dbPath := fleetReviewStore(t, map[string][]map[string]any{"endpoints_managed": {ep(5)}})
	_ = runFleet(t, newNovelFleetPatchDriftCmd, dbPath) // baseline
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB().Exec(`DELETE FROM resources WHERE resource_type = 'endpoints_managed'`); err != nil { // a full sync replaces the list
		t.Fatal(err)
	}
	seedUpsert(t, db, "endpoints_managed", "ep-a", ep(1))
	db.Close()
	time.Sleep(10 * time.Millisecond)
	var second driftResult
	if err := json.Unmarshal(runFleet(t, newNovelFleetPatchDriftCmd, dbPath), &second); err != nil {
		t.Fatal(err)
	}
	if second.Summary.Remediated != 1 {
		t.Fatalf("test setup: the second capture remediates ep-a: %+v", second)
	}
	db, err = store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB().Exec(`DELETE FROM resources WHERE resource_type = 'endpoints_managed'`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	time.Sleep(10 * time.Millisecond)
	var third driftResult
	if err := json.Unmarshal(runFleet(t, newNovelFleetPatchDriftCmd, dbPath), &third); err != nil {
		t.Fatal(err)
	}
	if third.Summary.CurrentTakenAt == second.Summary.CurrentTakenAt || third.Summary.Remediated != 0 {
		t.Fatalf("an empty capture repeated the previous drift result: %+v", third)
	}
	if third.Summary.NotObserved != 1 || len(third.Changes) != 1 || third.Changes[0].Status != "not_observed" {
		t.Fatalf("ep-a is absent from the current capture: want not_observed: %+v", third)
	}
}
