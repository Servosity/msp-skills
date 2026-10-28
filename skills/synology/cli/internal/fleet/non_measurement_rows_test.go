// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// HAND-AUTHORED.

package fleet

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// TestNonMeasurementRowsAreNull: abb trend's nas_not_synced, abb_missing,
// abb_not_installed and partial_history rows carried id 0, runs 0,
// successes 0 and success_rate_pct 0, so "lowest success rate" ranked a NAS
// that was never synced at 0%; fleet status rows for such a NAS carried
// tasks 0, tasks_ok 0 ... stale_devices 0. Absence is never zero: on a row
// that is not a measurement those fields are null. Measured rows keep
// their numbers.
func TestNonMeasurementRowsAreNull(t *testing.T) {
	ctx := context.Background()
	db := openDB(t)
	on, off := true, false
	synced := NAS{Name: "acme"}
	commitTasks(t, db, synced, &Snapshot{TakenAt: now, ABBInstalled: true, HistoryFrom: ts(2 * day), Results: []Result{
		{ResultID: 1, TaskID: 1, TaskName: "T", Status: StatusSuccess, JobAction: JobBackup, Start: ts(time.Hour), End: ts(time.Hour) + 60}}})
	partial := NAS{Name: "late"} // synced, but its history starts inside the window and holds no run
	commitTasks(t, db, partial, &Snapshot{TakenAt: now, ABBInstalled: true, HistoryFrom: ts(time.Hour)})
	never := NAS{Name: "never"}
	gone := NAS{Name: "gone"}
	_ = CommitSync(ctx, db, gone, now.Add(-time.Hour), &Snapshot{TakenAt: now.Add(-time.Hour), ABBInstalled: true, Storage: vol(), StorageOK: true}, SystemInfo{ABBInstalled: &on}, nil)
	_ = CommitSync(ctx, db, gone, now, &Snapshot{TakenAt: now, ABBAbsent: true, Storage: vol(), StorageOK: true}, SystemInfo{ABBInstalled: &off}, nil)
	noABB := NAS{Name: "files"}
	_ = CommitSync(ctx, db, noABB, now, &Snapshot{TakenAt: now, ABBAbsent: true, Storage: vol(), StorageOK: true}, SystemInfo{ABBInstalled: &off}, nil)
	all := []NAS{synced, partial, never, gone, noABB}

	trend, err := Trend(ctx, db, all, now.Add(-day), false, 2, now, 26*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string]bool{}
	for _, r := range trend {
		b, _ := json.Marshal(r)
		kinds[r.Kind] = true
		if r.Kind == "task" {
			if r.Runs == nil || *r.Runs != 1 || r.SuccessRatePct == nil || *r.SuccessRatePct != 100 || r.ID == nil || *r.ID != 1 {
				t.Errorf("a measured row lost its numbers: %s", b)
			}
			continue
		}
		for _, k := range []string{"id", "runs", "successes", "partials", "failures", "success_rate_pct"} {
			if !strings.Contains(string(b), `"`+k+`":null`) {
				t.Errorf("%s row %s: %s is not null: %s", r.Kind, r.NAS, k, b)
			}
		}
	}
	for _, k := range []string{"task", "partial_history", "nas_not_synced", "abb_missing", "abb_not_installed"} {
		if !kinds[k] {
			t.Fatalf("fixture: no %s row in %+v", k, trend)
		}
	}

	status, err := Status(ctx, db, all, now.Add(-day), 36*time.Hour, 85, 55, now, 26*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	counts := []string{"tasks", "tasks_ok", "tasks_partial", "tasks_failed", "tasks_no_run", "tasks_running", "tasks_running_too_long", "tasks_unscheduled", "stale_devices"}
	for _, r := range status {
		b, _ := json.Marshal(r)
		measured := r.NAS == "acme" || r.NAS == "late"
		for _, k := range counts {
			isNull := strings.Contains(string(b), `"`+k+`":null`)
			if measured == isNull {
				t.Errorf("fleet status %s (measured=%v): %s null=%v: %s", r.NAS, measured, k, isNull, b)
			}
		}
	}
}
