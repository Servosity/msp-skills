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

// TestAbsentMeasurementsNeverReadAsZero: a mirror with one recent completed
// backup lacking time_end made `abb trend --since 1d --json` report
// median_duration_sec 0, p90 0 and overrun false with no note. Absence is
// never zero: durations that cannot be measured are null with a note,
// overrun is null whenever it cannot be judged, abb device's recent runs
// show an unreported end, duration or byte count as null (a reported 0 stays
// 0), and a good run without an end time is not "never succeeded".
func TestAbsentMeasurementsNeverReadAsZero(t *testing.T) {
	ctx := context.Background()
	n := NAS{Name: "acme"}
	trend := func(t *testing.T, res []Result, overrun float64) TrendRow {
		t.Helper()
		db := openDB(t)
		commitTasks(t, db, n, &Snapshot{TakenAt: now, ABBInstalled: true, HistoryFrom: ts(30 * day), Results: res})
		rows, err := Trend(ctx, db, []NAS{n}, now.Add(-day), false, overrun, now, 26*time.Hour)
		if err != nil || len(rows) != 1 {
			t.Fatalf("trend: %v %+v", err, rows)
		}
		return rows[0]
	}
	run := func(id int64, ago time.Duration, dur int64) Result {
		r := Result{ResultID: id, TaskID: 1, TaskName: "T", Status: StatusSuccess, JobAction: JobBackup, Start: ts(ago)}
		if dur >= 0 {
			r.End = r.Start + dur
		}
		return r
	}

	// The finding's case: one completed backup in the window, no end time.
	r := trend(t, []Result{run(1, time.Hour, -1)}, 2)
	b, _ := json.Marshal(r)
	for _, want := range []string{`"median_duration_sec":null`, `"p90_duration_sec":null`, `"last_duration_sec":null`, `"overrun":null`} {
		if !strings.Contains(string(b), want) {
			t.Errorf("no end time: %s lacks %s", b, want)
		}
	}
	if !strings.Contains(r.Note, "unknown") {
		t.Errorf("no end time: no unknown note: %q", r.Note)
	}

	// Two measured runs: durations known, overrun cannot be judged.
	r = trend(t, []Result{run(1, 3*time.Hour, 60), run(2, time.Hour, 600)}, 2)
	if r.MedianSec == nil || r.P90Sec == nil || r.Overrun != nil || !strings.Contains(r.Note, "at least 3") {
		t.Errorf("two runs: %+v", r)
	}
	// The check switched off is not a "no".
	r = trend(t, []Result{run(1, 5*time.Hour, 60), run(2, 3*time.Hour, 60), run(3, time.Hour, 600)}, 0)
	if r.Overrun != nil || !strings.Contains(r.Note, "off") {
		t.Errorf("--overrun 0: %+v", r)
	}
	// Three measured runs still judge overrun (true and false).
	if r = trend(t, []Result{run(1, 5*time.Hour, 60), run(2, 3*time.Hour, 60), run(3, time.Hour, 600)}, 2); r.Overrun == nil || !*r.Overrun {
		t.Errorf("three runs, overrun: %+v", r)
	}
	if r = trend(t, []Result{run(1, 5*time.Hour, 60), run(2, 3*time.Hour, 60), run(3, time.Hour, 60)}, 2); r.Overrun == nil || *r.Overrun {
		t.Errorf("three runs, no overrun: %+v", r)
	}

	// abb device: the fake NAS reports NEW-PC's transfer without
	// transfered_bytes; BOB-LAPTOP's report 100. A reported 0 stays 0.
	db := openDB(t)
	syncFake(t, db, n, false)
	devs, err := FindDevice(ctx, db, []NAS{n}, "new-pc", 5, now, 26*time.Hour)
	if err != nil || len(devs) != 1 || len(devs[0].RecentRuns) != 1 {
		t.Fatalf("FindDevice new-pc: %v %+v", err, devs)
	}
	b, _ = json.Marshal(devs[0].RecentRuns[0])
	if !strings.Contains(string(b), `"transferred_bytes":null`) || !strings.Contains(string(b), `"duration_sec":3600`) {
		t.Errorf("an unreported byte count read as a number: %s", b)
	}
	devs, _ = FindDevice(ctx, db, []NAS{n}, "bob", 5, now, 26*time.Hour)
	if len(devs) != 1 || len(devs[0].RecentRuns) == 0 || devs[0].RecentRuns[0].TransferredBytes == nil || *devs[0].RecentRuns[0].TransferredBytes != 100 {
		t.Fatalf("a reported byte count was lost: %+v", devs)
	}
	db2 := openDB(t)
	commitTasks(t, db2, n, &Snapshot{TakenAt: now, ABBInstalled: true, HistoryFrom: ts(30 * day),
		Tasks:      []Task{{TaskID: 1, Name: "PCs", Devices: []Device{{DeviceID: 7, HostName: "ZED"}}}},
		Results:    []Result{{ResultID: 1, TaskID: 1, TaskName: "PCs", Status: StatusSuccess, JobAction: JobBackup, Start: ts(2 * time.Hour), End: ts(time.Hour), DeviceIDs: ",7,"}},
		DeviceRuns: []DeviceRun{{DeviceResultID: 9, ResultID: 1, DeviceID: 7, DeviceName: "ZED", Status: StatusSuccess, Start: ts(2 * time.Hour), Bytes: 0}}})
	devs, _ = FindDevice(ctx, db2, []NAS{n}, "zed", 5, now, 26*time.Hour)
	if len(devs) != 1 || len(devs[0].RecentRuns) != 1 {
		t.Fatalf("FindDevice zed: %+v", devs)
	}
	b, _ = json.Marshal(devs[0].RecentRuns[0])
	if !strings.Contains(string(b), `"transferred_bytes":0`) || !strings.Contains(string(b), `"time_end":null`) || !strings.Contains(string(b), `"duration_sec":null`) {
		t.Errorf("zed run: %s", b)
	}
	// Its good run has no end time: it is not "never succeeded", and it is
	// recent, so not stale either; its start bounds the age.
	if devs[0].LastSuccess != ts(2*time.Hour) {
		t.Errorf("a good run without an end time was not counted: last_success %d", devs[0].LastSuccess)
	}
	stale, err := Stale(ctx, db2, []NAS{n}, 36*time.Hour, now, 26*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range stale {
		if s.HostName == "ZED" {
			t.Errorf("a recent good run without an end time read as %s", s.Kind)
		}
	}
}
