// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"testing"
)

func TestFleetPatchPosture(t *testing.T) {
	db := seedFleetStore(t)
	out := runFleet(t, newNovelFleetPatchPostureCmd, db)

	var rows []patchPostureRow
	if err := json.Unmarshal(out, &rows); err != nil {
		t.Fatalf("parse output: %v\n%s", err, out)
	}
	if len(rows) != 3 {
		t.Fatalf("want 3 endpoints, got %d", len(rows))
	}
	// WS-CRITICAL has the most missing updates (8 critical) → ranked first.
	if rows[0].Name != "WS-CRITICAL" || rows[0].Critical != 8 || rows[0].TotalMissing != 20 {
		t.Fatalf("expected WS-CRITICAL first with 8 critical / 20 total, got %+v", rows[0])
	}
	// Sorted by critical desc: ep-3 (2 critical) before ep-2 (0 critical).
	if rows[1].Critical < rows[2].Critical {
		t.Fatalf("rows not sorted by critical desc: %+v", rows)
	}
	if rows[2].Name != "WS-HEALTHY" {
		t.Fatalf("healthiest endpoint should rank last, got %s", rows[2].Name)
	}
}

func TestFleetPatchPostureOrgFilter(t *testing.T) {
	db := seedFleetStore(t)
	out := runFleet(t, newNovelFleetPatchPostureCmd, db, "--org", "org-b")
	var rows []patchPostureRow
	if err := json.Unmarshal(out, &rows); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(rows) != 1 || rows[0].OrganizationID != "org-b" {
		t.Fatalf("org filter failed: %+v", rows)
	}
}

func TestFleetPatchPostureEmptyStore(t *testing.T) {
	db := emptyFleetStore(t)
	out := runFleet(t, newNovelFleetPatchPostureCmd, db)
	var rows []patchPostureRow
	if err := json.Unmarshal(out, &rows); err != nil {
		t.Fatalf("empty store should emit [] not error: %v\n%s", err, out)
	}
	if len(rows) != 0 {
		t.Fatalf("empty store should yield 0 rows, got %d", len(rows))
	}
}
