// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import "testing"

func TestRestorePointsForOneGuest(t *testing.T) {
	newFixture(t)
	out, stderr, err := runCLI(t, "restore-points", "100", "--json")
	if err != nil {
		t.Fatalf("restore-points: %v\n%s", err, stderr)
	}
	rep := decodeJSON[restorePointsReport](t, out)
	if len(rep.Guests) != 1 {
		t.Fatalf("guests = %+v", rep.Guests)
	}
	g := rep.Guests[0]
	// 2h + 26h from the PVE PBS-storage listing, 50h only on the PBS API (deduped, not 5).
	if g.Name != "web" || g.Count != 3 || len(g.RestorePoints) != 3 {
		t.Fatalf("guest 100 = %+v", g)
	}
	if g.RestorePoints[0].TimeRFC3339 <= g.RestorePoints[1].TimeRFC3339 {
		t.Errorf("not newest-first: %s then %s", g.RestorePoints[0].TimeRFC3339, g.RestorePoints[1].TimeRFC3339)
	}
	if last := g.RestorePoints[2]; last.Source != "pbs:main" || last.VerifyState != "failed" {
		t.Errorf("PBS-only point = %+v", last)
	}
	if len(g.CoveredByJobs) != 1 || g.CoveredByJobs[0] != "job1" || g.NewestVerifiedOKAt != g.NewestAt {
		t.Errorf("coverage/newest = %+v", g)
	}
	// By name, verified only, limited.
	out, _, err = runCLI(t, "restore-points", "WEB", "--verified-only", "--limit", "1", "--json")
	if err != nil {
		t.Fatal(err)
	}
	g = decodeJSON[restorePointsReport](t, out).Guests[0]
	if g.Count != 2 || len(g.RestorePoints) != 1 || g.RestorePoints[0].VerifyState != "ok" {
		t.Errorf("--verified-only --limit 1 by name = %+v", g)
	}
	// Negative: unknown guest is exit 3, missing argument is exit 2.
	if _, _, err := runCLI(t, "restore-points", "999", "--json"); ExitCode(err) != 3 {
		t.Errorf("unknown guest must exit 3, got %v", err)
	}
	if _, _, err := runCLI(t, "restore-points", "--json"); ExitCode(err) != 2 {
		t.Errorf("missing guest must exit 2, got %v", err)
	}
	// A guest with no backups returns an empty list, not an error.
	out, _, err = runCLI(t, "restore-points", "105", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if g := decodeJSON[restorePointsReport](t, out).Guests[0]; g.Count != 0 || g.RestorePoints == nil {
		t.Errorf("105 = %+v (restore_points must be [] not null)", g)
	}
}
