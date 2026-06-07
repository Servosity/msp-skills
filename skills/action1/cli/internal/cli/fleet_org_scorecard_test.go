// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"testing"
)

func TestFleetOrgScorecard(t *testing.T) {
	db := seedFleetStore(t)
	out := runFleet(t, newNovelFleetOrgScorecardCmd, db)

	var rows []orgScorecardRow
	if err := json.Unmarshal(out, &rows); err != nil {
		t.Fatalf("parse: %v\n%s", err, out)
	}
	if len(rows) != 2 {
		t.Fatalf("want 2 org rows, got %d: %+v", len(rows), rows)
	}
	// org-a leads: it carries the KEV CVE (worst posture sorts first).
	a, b := rows[0], rows[1]
	if a.OrganizationID != "org-a" || b.OrganizationID != "org-b" {
		t.Fatalf("expected org-a (KEV) first then org-b, got %s then %s", a.OrganizationID, b.OrganizationID)
	}
	if a.Endpoints != 2 || a.MissingCrit != 8 || a.MissingOther != 13 {
		t.Fatalf("org-a rollup wrong: %+v", a)
	}
	if a.OpenCVEs != 1 || a.KEVCVEs != 1 || a.RebootPending != 1 || a.StaleEndpoints != 0 {
		t.Fatalf("org-a cve/kev/reboot/stale wrong: %+v", a)
	}
	if b.Endpoints != 1 || b.MissingCrit != 2 || b.OpenCVEs != 1 || b.KEVCVEs != 0 {
		t.Fatalf("org-b rollup wrong: %+v", b)
	}
	// SRV-DARK last checked in 40 days ago — stale at the 14-day default.
	if b.StaleEndpoints != 1 {
		t.Fatalf("org-b should count 1 stale endpoint: %+v", b)
	}
}

func TestFleetOrgScorecardStaleWindow(t *testing.T) {
	db := seedFleetStore(t)
	// A 60-day window means even SRV-DARK (40d) is not stale.
	out := runFleet(t, newNovelFleetOrgScorecardCmd, db, "--stale-days", "60")
	var rows []orgScorecardRow
	if err := json.Unmarshal(out, &rows); err != nil {
		t.Fatalf("parse: %v", err)
	}
	for _, r := range rows {
		if r.StaleEndpoints != 0 {
			t.Fatalf("no endpoint should be stale at 60 days: %+v", r)
		}
	}
}

func TestFleetOrgScorecardEmptyStore(t *testing.T) {
	db := emptyFleetStore(t)
	out := runFleet(t, newNovelFleetOrgScorecardCmd, db)
	var rows []orgScorecardRow
	if err := json.Unmarshal(out, &rows); err != nil {
		t.Fatalf("parse: %v\n%s", err, out)
	}
	if len(rows) != 0 {
		t.Fatalf("empty store must yield empty scorecard, got %+v", rows)
	}
}
