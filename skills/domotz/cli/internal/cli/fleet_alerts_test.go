// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Seeded-store behavioral test for fleet alerts.

package cli

import "testing"

func TestFleetAlertsCommand(t *testing.T) {
	dbPath := seedFleetStore(t)
	flags := &rootFlags{asJSON: true}
	rows := runFleetJSONArray(t, newNovelFleetAlertsCmd(flags), dbPath)

	if len(rows) != 2 {
		t.Fatalf("alerts returned %d rows, want 2", len(rows))
	}
	// Enabled-first ordering: the enabled "Down Alert" must lead.
	if rows[0]["name"] != "Down Alert" {
		t.Fatalf("first alert = %v, want Down Alert (enabled first)", rows[0]["name"])
	}
}

func TestFleetAlertsEnabledOnly(t *testing.T) {
	dbPath := seedFleetStore(t)
	flags := &rootFlags{asJSON: true}
	rows := runFleetJSONArray(t, newNovelFleetAlertsCmd(flags), dbPath, "--enabled-only")
	if len(rows) != 1 {
		t.Fatalf("enabled-only returned %d rows, want 1", len(rows))
	}
	if rows[0]["name"] != "Down Alert" {
		t.Fatalf("enabled alert = %v, want Down Alert", rows[0]["name"])
	}
}
