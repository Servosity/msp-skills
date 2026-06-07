// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"testing"
)

func TestFleetRebootPending(t *testing.T) {
	db := seedFleetStore(t)
	out := runFleet(t, newNovelFleetRebootPendingCmd, db)

	var rows []rebootPendingRow
	if err := json.Unmarshal(out, &rows); err != nil {
		t.Fatalf("parse: %v\n%s", err, out)
	}
	// Only ep-1 (WS-CRITICAL) reports reboot_required: "Yes" in the fixture.
	if len(rows) != 1 {
		t.Fatalf("want 1 reboot-pending endpoint, got %d: %+v", len(rows), rows)
	}
	if rows[0].Name != "WS-CRITICAL" || rows[0].MissingCrit != 8 {
		t.Fatalf("expected WS-CRITICAL with 8 critical remaining, got %+v", rows[0])
	}
}

func TestFleetRebootPendingOrgFilterExcludes(t *testing.T) {
	db := seedFleetStore(t)
	// org-b has no reboot-pending endpoints; the filter must not leak org-a rows.
	out := runFleet(t, newNovelFleetRebootPendingCmd, db, "--org", "org-b")
	var rows []rebootPendingRow
	if err := json.Unmarshal(out, &rows); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("org-b should have no reboot-pending endpoints: %+v", rows)
	}
}

func TestFleetRebootPendingEmptyStore(t *testing.T) {
	db := emptyFleetStore(t)
	out := runFleet(t, newNovelFleetRebootPendingCmd, db)
	var rows []rebootPendingRow
	if err := json.Unmarshal(out, &rows); err != nil {
		t.Fatalf("parse: %v\n%s", err, out)
	}
	if len(rows) != 0 {
		t.Fatalf("empty store must yield empty list with exit 0, got %+v", rows)
	}
}
