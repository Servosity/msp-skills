// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"testing"
)

func TestFleetStale(t *testing.T) {
	db := seedFleetStore(t)
	// 30-day window: SRV-DARK (last seen 40d ago, offline) is stale; the two
	// recently-seen online endpoints are not.
	out := runFleet(t, newNovelFleetStaleCmd, db, "--days", "30")

	var rows []staleRow
	if err := json.Unmarshal(out, &rows); err != nil {
		t.Fatalf("parse: %v\n%s", err, out)
	}
	if len(rows) != 1 {
		t.Fatalf("want 1 stale endpoint at 30 days, got %d: %+v", len(rows), rows)
	}
	if rows[0].Name != "SRV-DARK" {
		t.Fatalf("expected SRV-DARK stale, got %s", rows[0].Name)
	}
	if rows[0].DaysSinceSeen < 39 || rows[0].DaysSinceSeen > 41 {
		t.Fatalf("days-since-seen should be ~40, got %v", rows[0].DaysSinceSeen)
	}
}

func TestFleetStaleOfflineFlagsRecentAgent(t *testing.T) {
	db := seedFleetStore(t)
	// A huge window means no agent is stale by time; SRV-DARK still flags
	// because it is offline and --include-offline defaults true.
	out := runFleet(t, newNovelFleetStaleCmd, db, "--days", "3650")
	var rows []staleRow
	if err := json.Unmarshal(out, &rows); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(rows) != 1 || rows[0].Name != "SRV-DARK" {
		t.Fatalf("offline endpoint should flag even within the time window: %+v", rows)
	}
}

func TestFleetStaleNoneWhenWindowLargeAndOfflineExcluded(t *testing.T) {
	db := seedFleetStore(t)
	out := runFleet(t, newNovelFleetStaleCmd, db, "--days", "3650", "--include-offline=false")
	var rows []staleRow
	if err := json.Unmarshal(out, &rows); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("no endpoint should be stale by time alone in a 10y window: %+v", rows)
	}
}
