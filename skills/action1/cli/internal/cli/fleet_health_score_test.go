// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"testing"
)

func TestFleetHealthScore(t *testing.T) {
	db := seedFleetStore(t)
	out := runFleet(t, newNovelFleetHealthScoreCmd, db)

	var rows []healthScoreRow
	if err := json.Unmarshal(out, &rows); err != nil {
		t.Fatalf("parse: %v\n%s", err, out)
	}
	if len(rows) != 3 {
		t.Fatalf("want 3 endpoints, got %d", len(rows))
	}
	// WS-CRITICAL: 8 crit updates*10 + 3 crit vulns*12 + 12 other + reboot 5 = 133 risk → highest.
	if rows[0].Name != "WS-CRITICAL" {
		t.Fatalf("WS-CRITICAL should be highest risk, got %s (%+v)", rows[0].Name, rows[0])
	}
	if rows[0].RiskScore <= rows[1].RiskScore {
		t.Fatalf("rows not sorted by risk desc: %+v", rows)
	}
	if rows[0].Health < 0 {
		t.Fatalf("health must clamp at 0, got %d", rows[0].Health)
	}
	// Healthiest endpoint (WS-HEALTHY) ranks last with a high health value.
	last := rows[len(rows)-1]
	if last.Name != "WS-HEALTHY" || last.Health < 90 {
		t.Fatalf("WS-HEALTHY should rank last with high health: %+v", last)
	}
}
