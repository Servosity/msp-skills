// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"testing"
	"time"
)

func TestRPOEvidence(t *testing.T) {
	newFixture(t)
	out, stderr, err := runCLI(t, "rpo", "--days", "7", "--json")
	if err != nil {
		t.Fatalf("rpo: %v\n%s", err, stderr)
	}
	rep := decodeJSON[rpoReport](t, out)
	rows := map[int64]rpoRow{}
	for _, r := range rep.Rows {
		rows[r.VMID] = r
	}
	if len(rows) != 6 {
		t.Fatalf("want the 6 guests covered by enabled job1, got %d: %+v", len(rows), rep.Rows)
	}
	for _, uncovered := range []int64{102, 103, 106} {
		if _, ok := rows[uncovered]; ok {
			t.Errorf("guest %d has no enabled job and must not be scored", uncovered)
		}
	}
	if r := rows[101]; r.MissedRuns != 2 || r.MetSchedule || !r.WindowCapped || r.LargestGapHours < 49 {
		t.Errorf("101 (only a 50h-old point) = %+v", r)
	}
	// Retention capped the window: no misses seen, but compliance is unproven.
	// The run whose window holds the oldest point (50h) is judged, and hit.
	if r := rows[100]; r.MissedRuns != 0 || r.MetSchedule || r.Evidence != "partial" || r.ScheduledRuns != 3 || r.VerifiedOKDays != 2 {
		t.Errorf("100 = %+v", r)
	}
	// Retention cap: runs older than the oldest retained point are not called missed.
	if r := rows[104]; r.MissedRuns != 0 || !r.WindowCapped {
		t.Errorf("104 = %+v", r)
	}
	if r := rows[105]; r.MissedRuns != 7 || r.MetSchedule || r.Note == "" {
		t.Errorf("105 (covered, never backed up) = %+v", r)
	}
	if rep.Rows[0].VMID != 105 {
		t.Errorf("rows must sort by missed runs, first = %d", rep.Rows[0].VMID)
	}
	if len(rep.Clients) != 1 || rep.Clients[0].Guests != 6 || rep.Clients[0].GuestsMet != 0 || rep.Clients[0].GuestsPartial != 5 || rep.Clients[0].MissedRuns != 9 {
		t.Errorf("client rollup = %+v", rep.Clients)
	}
	if len(rep.FetchFailures) != 0 {
		t.Errorf("schedule expansion must page within the 100-iteration cap: %+v", rep.FetchFailures)
	}
	out, _, err = runCLI(t, "rpo", "--guest", "101", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if r := decodeJSON[rpoReport](t, out).Rows; len(r) != 1 || r[0].VMID != 101 {
		t.Errorf("--guest 101 = %+v", r)
	}
	for _, bad := range [][]string{{"rpo", "--days", "0"}, {"rpo", "--guest", "web"}} {
		if _, _, err := runCLI(t, append(bad, "--json")...); ExitCode(err) != 2 {
			t.Errorf("%v must exit 2, got %v", bad, err)
		}
	}
}

func TestEvaluateRPOEdges(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	start := now.Add(-72 * time.Hour)
	runs := []int64{now.Add(-48 * time.Hour).Unix(), now.Add(-24 * time.Hour).Unix(), now.Add(-2 * time.Hour).Unix()}
	// A point before the window: not capped, both full runs checked, the 2h-old run is in progress.
	pts := []restorePoint{{Time: now.Add(-80 * time.Hour).Unix()}, {Time: now.Add(-47 * time.Hour).Unix(), VerifyState: "ok"}}
	r := evaluateRPO(pts, runs, start, now)
	if r.WindowCapped || r.ScheduledRuns != 2 || r.MissedRuns != 1 || r.LargestGapHours != 47 {
		t.Errorf("edge = %+v", r)
	}
	// Only identical timestamps merge: runs 30 minutes apart each need a point.
	if got, jobs := dedupeRuns([]int64{100, 100, 1900}); len(got) != 2 || jobs[0] != 2 || jobs[1] != 1 {
		t.Errorf("dedupeRuns = %v %v", got, jobs)
	}
	// Full evidence (a point before the window) and every run met, the
	// in-progress newest one included.
	full := []restorePoint{{Time: now.Add(-80 * time.Hour).Unix()}, {Time: now.Add(-47 * time.Hour).Unix()}, {Time: now.Add(-23 * time.Hour).Unix()}, {Time: now.Add(-time.Hour).Unix()}}
	if r := evaluateRPO(full, runs, start, now); !r.MetSchedule || r.Evidence != "full" || r.MissedRuns != 0 {
		t.Errorf("full evidence = %+v", r)
	}
	// The newest run within the grace and no point for it yet: not judged, not met.
	if r := evaluateRPO(full[:3], runs, start, now); r.MetSchedule || r.MissedRuns != 0 || r.Note == "" {
		t.Errorf("pending newest run = %+v", r)
	}
}
