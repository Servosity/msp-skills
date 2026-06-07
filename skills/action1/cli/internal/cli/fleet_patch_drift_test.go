// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"testing"

	"action1-pp-cli/internal/store"
)

func TestFleetPatchDriftBaselineThenDiff(t *testing.T) {
	db := seedFleetStore(t)

	// First run records a baseline and reports no diff yet.
	out := runFleet(t, newNovelFleetPatchDriftCmd, db)
	var first driftResult
	if err := json.Unmarshal(out, &first); err != nil {
		t.Fatalf("parse first: %v\n%s", err, out)
	}
	if first.Summary.Note == "" {
		t.Fatalf("first run should note a baseline capture, got %+v", first.Summary)
	}

	// Remediate WS-CRITICAL (8+12 -> 1+0) and worsen WS-HEALTHY (0+1 -> 5+1),
	// then run again: the diff should classify both.
	st, err := store.Open(db)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	updateEndpointMissing(t, st, "ep-1", 1, 0)
	updateEndpointMissing(t, st, "ep-2", 5, 1)
	st.Close()

	out = runFleet(t, newNovelFleetPatchDriftCmd, db)
	var second driftResult
	if err := json.Unmarshal(out, &second); err != nil {
		t.Fatalf("parse second: %v\n%s", err, out)
	}
	if second.Summary.Remediated < 1 {
		t.Fatalf("expected a remediated endpoint, got %+v", second.Summary)
	}
	if second.Summary.Regressed < 1 {
		t.Fatalf("expected a regressed endpoint, got %+v", second.Summary)
	}
	var sawRemediated bool
	for _, c := range second.Changes {
		if c.EndpointID == "ep-1" && c.Status == "remediated" && c.Delta == -19 {
			sawRemediated = true
		}
	}
	if !sawRemediated {
		t.Fatalf("ep-1 should be remediated with delta -19: %+v", second.Changes)
	}
}

func updateEndpointMissing(t *testing.T, db *store.Store, id string, critical, other int) {
	t.Helper()
	obj := map[string]any{
		"id": id, "name": id, "organization_id": "org-a",
		"missing_updates": map[string]any{"critical": critical, "other": other},
	}
	raw, _ := json.Marshal(obj)
	if err := db.Upsert("endpoints", id, raw); err != nil {
		t.Fatalf("update endpoint: %v", err)
	}
}
