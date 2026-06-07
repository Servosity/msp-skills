// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"testing"
)

func TestFleetSoftwareRollup(t *testing.T) {
	db := seedFleetStore(t)
	out := runFleet(t, newNovelFleetSoftwareRollupCmd, db)

	var rows []softwareRollupRow
	if err := json.Unmarshal(out, &rows); err != nil {
		t.Fatalf("parse: %v\n%s", err, out)
	}
	if len(rows) != 2 {
		t.Fatalf("want 2 distinct app/version rows, got %d: %+v", len(rows), rows)
	}
	// Chrome installed on two endpoints across two orgs ranks first.
	if rows[0].Name != "Google Chrome" || rows[0].Installs != 2 {
		t.Fatalf("Chrome should aggregate 2 installs first: %+v", rows[0])
	}
	if rows[0].Orgs != 2 || rows[0].Endpoints != 2 {
		t.Fatalf("Chrome should span 2 orgs / 2 endpoints: %+v", rows[0])
	}
}

func TestFleetSoftwareRollupNameFilter(t *testing.T) {
	db := seedFleetStore(t)
	out := runFleet(t, newNovelFleetSoftwareRollupCmd, db, "--name", "chrome")
	var rows []softwareRollupRow
	if err := json.Unmarshal(out, &rows); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(rows) != 1 || rows[0].Name != "Google Chrome" {
		t.Fatalf("--name chrome should match only Chrome (case-insensitive): %+v", rows)
	}
}
