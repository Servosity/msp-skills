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

func kinds[T any](rows []T, kind func(T) string) string {
	var ks []string
	for _, r := range rows {
		ks = append(ks, kind(r))
	}
	return strings.Join(ks, ",")
}

// TestUnprotectedFindsWhatDroppedOut: a device removed from its task, a
// deleted task and an unscheduled task holding devices each surface; a device
// still protected, a run outside the window and a non-backup task result do not (sync
// refuses device runs under a non-backup result, so none is seeded).
func TestUnprotectedFindsWhatDroppedOut(t *testing.T) {
	ctx := context.Background()
	db := openDB(t)
	n := NAS{Name: "acme", Client: "Acme"}
	commitTasks(t, db, n, &Snapshot{TakenAt: now, ABBInstalled: true, HistoryFrom: ts(60 * day),
		Tasks: []Task{
			{TaskID: 1, Name: "Office", Scheduled: true, Devices: []Device{{DeviceID: 10, HostName: "PC-A"}}},
			{TaskID: 2, Name: "Old", Scheduled: false, Devices: []Device{{DeviceID: 11, HostName: "PC-B"}}},
		},
		Results: []Result{
			{ResultID: 100, TaskID: 1, TaskName: "Office", Status: StatusSuccess, JobAction: JobBackup, Start: ts(day), End: ts(day) + 60},
			{ResultID: 101, TaskID: 3, TaskName: "Gone", Status: StatusSuccess, JobAction: JobBackup, Start: ts(2 * day), End: ts(2*day) + 60},
			{ResultID: 102, TaskID: 4, TaskName: "Ancient", Status: StatusSuccess, JobAction: JobBackup, Start: ts(50 * day), End: ts(50*day) + 60},
			{ResultID: 103, TaskID: 5, TaskName: "Restore", Status: StatusSuccess, JobAction: 2, Start: ts(day), End: ts(day) + 60},
			{ResultID: 104, TaskID: 3, TaskName: "Gone", Status: StatusSuccess, JobAction: JobBackup, Start: ts(3 * day), End: ts(3*day) + 60},
		},
		DeviceRuns: []DeviceRun{
			{DeviceResultID: 1, ResultID: 100, DeviceID: 10, DeviceName: "PC-A", Status: StatusSuccess, Start: ts(day), End: ts(day) + 60, Bytes: 5},
			{DeviceResultID: 2, ResultID: 101, DeviceID: 12, DeviceName: "PC-C", Status: StatusFailed, Start: ts(2 * day), End: ts(2*day) + 60, Bytes: 5},
			{DeviceResultID: 3, ResultID: 104, DeviceID: 13, DeviceName: "PC-A", Status: StatusSuccess, Start: ts(3 * day), End: ts(3*day) + 60, Bytes: 5},
			{DeviceResultID: 4, ResultID: 102, DeviceID: 14, DeviceName: "PC-D", Status: StatusSuccess, Start: ts(50 * day), End: ts(50*day) + 60, Bytes: 5},
		},
	})
	rows, err := Unprotected(ctx, db, []NAS{n, {Name: "ghost"}}, now.Add(-30*day), now, 26*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	got := kinds(rows, func(r UnprotectedRow) string { return r.Kind + ":" + firstNonEmpty(r.HostName, r.TaskName, r.NAS) })
	want := "device_dropped:PC-A,device_dropped:PC-C,task_deleted:Gone,task_unscheduled:Old,nas_not_synced:ghost"
	if got != want {
		t.Fatalf("rows = %s\nwant   %s", got, want)
	}
	if !strings.Contains(rows[0].Note, "device_id 10") || rows[0].DeviceID != 13 {
		t.Fatalf("re-registered agent hint missing: %+v", rows[0])
	}
	if rows[1].LastSuccess != nil || rows[1].LastRunAt == nil {
		t.Fatalf("a device that only failed has last_success null and a last run: %+v", rows[1])
	}
	if rows[3].Devices == nil || *rows[3].Devices != 1 {
		t.Fatalf("unscheduled task device count: %+v", rows[3])
	}
	for _, r := range rows {
		if !r.IsFinding() {
			t.Fatalf("%s is not a finding", r.Kind)
		}
	}
	b, _ := json.Marshal(rows[1])
	if !strings.Contains(string(b), `"last_success":null`) {
		t.Fatalf("absence must be null, not 0: %s", b)
	}

	// The window starting before the mirrored history is said, not hidden.
	rows, err = Unprotected(ctx, db, []NAS{n}, now.Add(-90*day), now, 26*time.Hour)
	if err != nil || len(rows) == 0 || rows[0].HistoryFrom == 0 || !strings.Contains(rows[0].Note, "history is mirrored only since") {
		t.Fatalf("partial history: %v %+v", err, rows)
	}
}

// TestUnprotectedCleanNASIsEmptyAndPartialIsSaid: nothing dropped on a NAS
// whose history covers the window is an empty answer; with no history it is a
// partial_history row, which is not a finding.
func TestUnprotectedCleanNASIsEmptyAndPartialIsSaid(t *testing.T) {
	ctx := context.Background()
	db := openDB(t)
	n := NAS{Name: "acme"}
	commitTasks(t, db, n, &Snapshot{TakenAt: now, ABBInstalled: true, HistoryFrom: ts(60 * day),
		Tasks: []Task{{TaskID: 1, Name: "Office", Scheduled: true, Devices: []Device{{DeviceID: 10, HostName: "PC-A"}}}}})
	if rows, err := Unprotected(ctx, db, []NAS{n}, now.Add(-30*day), now, 26*time.Hour); err != nil || len(rows) != 0 {
		t.Fatalf("clean NAS: %v %+v", err, rows)
	}
	db = openDB(t)
	commitTasks(t, db, n, &Snapshot{TakenAt: now, ABBInstalled: true,
		Tasks: []Task{{TaskID: 1, Name: "Office", Scheduled: true, Devices: []Device{{DeviceID: 10, HostName: "PC-A"}}}}})
	rows, err := Unprotected(ctx, db, []NAS{n}, now.Add(-30*day), now, 26*time.Hour)
	if err != nil || len(rows) != 1 || rows[0].Kind != "partial_history" || rows[0].IsFinding() {
		t.Fatalf("no history: %v %+v", err, rows)
	}
}

// TestCapacityRunway: growth and days to full from retained snapshots, soonest
// first; a flat volume has no fill date; unknown capacity is a finding.
func TestCapacityRunway(t *testing.T) {
	ctx := context.Background()
	db := openDB(t)
	n := NAS{Name: "acme"}
	gb := int64(1 << 30)
	old := now.Add(-10 * day)
	if err := CommitSync(ctx, db, n, old, &Snapshot{TakenAt: old, ABBInstalled: true, StorageOK: true, Storage: []StorageItem{
		{Kind: "volume", ID: "v1", Name: "Volume 1", Status: "normal", SizeTotal: 100 * gb, SizeUsed: 50 * gb},
		{Kind: "volume", ID: "v2", Name: "Volume 2", Status: "normal", SizeTotal: 100 * gb, SizeUsed: 10 * gb},
		{Kind: "volume", ID: "v4", Name: "Volume 4", Status: "normal", SizeTotal: 100 * gb, SizeUsed: 80 * gb},
	}}, SystemInfo{}, nil); err != nil {
		t.Fatal(err)
	}
	commitTasks(t, db, n, &Snapshot{TakenAt: now, ABBInstalled: true, StorageOK: true, Storage: []StorageItem{
		{Kind: "volume", ID: "v1", Name: "Volume 1", Status: "normal", SizeTotal: 100 * gb, SizeUsed: 60 * gb}, // 1 GB/day, 40 days
		{Kind: "volume", ID: "v2", Name: "Volume 2", Status: "normal", SizeTotal: 100 * gb, SizeUsed: 10 * gb}, // flat
		{Kind: "volume", ID: "v3", Name: "Volume 3", Status: "normal"},                                         // capacity unknown
		{Kind: "volume", ID: "v4", Name: "Volume 4", Status: "normal", SizeTotal: 100 * gb, SizeUsed: 90 * gb}, // 1 GB/day, 10 days
	}})
	rows, err := Capacity(ctx, db, []NAS{n}, 30*day, now, 26*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if got := kinds(rows, func(r CapacityRow) string { return r.ID }); got != "v4,v1,v2,v3" {
		t.Fatalf("order = %s, want soonest first then by fill", got)
	}
	if d := rows[0].DaysToFull; d == nil || *d != 10 || !rows[0].WithinHorizon || !rows[0].IsFinding() {
		t.Fatalf("v4: %+v", rows[0])
	}
	if d := rows[1].DaysToFull; d == nil || *d != 40 || rows[1].WithinHorizon || rows[1].IsFinding() {
		t.Fatalf("v1 (outside the 30-day horizon): %+v", rows[1])
	}
	if rows[2].DaysToFull != nil || rows[2].GrowthBytesDay == nil || *rows[2].GrowthBytesDay != 0 || rows[2].IsFinding() {
		t.Fatalf("v2 flat: %+v", rows[2])
	}
	if !rows[3].CapacityUnknown || rows[3].UsedPct != nil || !rows[3].IsFinding() {
		t.Fatalf("v3 unknown capacity: %+v", rows[3])
	}
	b, _ := json.Marshal(rows[3])
	if !strings.Contains(string(b), `"days_to_full":null`) || !strings.Contains(string(b), `"used_pct":null`) {
		t.Fatalf("absence must be null: %s", b)
	}

	// One snapshot only: growth unknown, never 0, and not a finding.
	db2 := openDB(t)
	commitTasks(t, db2, n, &Snapshot{TakenAt: now, ABBInstalled: true, StorageOK: true, Storage: []StorageItem{
		{Kind: "volume", ID: "v1", Status: "normal", SizeTotal: 100, SizeUsed: 99}}})
	rows, err = Capacity(ctx, db2, []NAS{n, {Name: "ghost"}}, 30*day, now, 26*time.Hour)
	if err != nil || len(rows) != 2 || rows[0].GrowthBytesDay != nil || rows[0].DaysToFull != nil || rows[0].IsFinding() ||
		rows[1].Kind != "nas_not_synced" || !rows[1].IsFinding() {
		t.Fatalf("single snapshot + unsynced NAS: %v %+v", err, rows)
	}
}

// TestChangeRateSpikeAndFlat: a device far above its own median and one that
// went to zero bytes are flagged; a device without a baseline, unknown byte
// counts and an ordinary run are not.
func TestChangeRateSpikeAndFlat(t *testing.T) {
	ctx := context.Background()
	db := openDB(t)
	n := NAS{Name: "acme"}
	var runs []DeviceRun
	id := int64(0)
	add := func(dev int64, name string, ago time.Duration, bytes int64) {
		id++
		runs = append(runs, DeviceRun{DeviceResultID: id, DeviceID: dev, DeviceName: name, Status: StatusSuccess, Start: ts(ago), End: ts(ago) + 60, Bytes: bytes})
	}
	for i := 0; i < 6; i++ {
		ago := time.Duration(20-i) * day
		add(10, "SPIKE", ago, 100)
		add(11, "FLAT", ago, 100)
		add(14, "UNKNOWN", ago, -1)
		add(15, "STEADY", ago, 100)
	}
	add(10, "SPIKE", day, 5000)
	for i := 3; i > 0; i-- {
		add(11, "FLAT", time.Duration(i)*time.Hour, 0)
	}
	add(14, "UNKNOWN", day, 99999)
	add(15, "STEADY", day, 900)
	for i := 0; i < 3; i++ {
		add(12, "NEW", time.Duration(3-i)*day, 100)
	}
	add(12, "NEW", time.Hour, 99999)
	commitTasks(t, db, n, &Snapshot{TakenAt: now, ABBInstalled: true, HistoryFrom: ts(60 * day), DeviceRuns: runs})
	rows, err := ChangeRate(ctx, db, []NAS{n, {Name: "ghost"}}, now.Add(-30*day), 10, 3, now, 26*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if got := kinds(rows, func(r ChangeRateRow) string { return r.Kind + ":" + firstNonEmpty(r.HostName, r.NAS) }); got != "spike:SPIKE,flat:FLAT,nas_not_synced:ghost" {
		t.Fatalf("rows = %s", got)
	}
	if r := rows[0]; r.Ratio == nil || *r.Ratio != 50 || *r.MedianBytes != 100 || *r.NASSpikeCount != 1 {
		t.Fatalf("spike row: %+v", r)
	}
	if r := rows[1]; *r.ZeroRuns != 3 || *r.LatestBytes != 0 {
		t.Fatalf("flat row: %+v", r)
	}
	// --spike 0 and --flat-runs 0 turn the checks off.
	if rows, err = ChangeRate(ctx, db, []NAS{n}, now.Add(-30*day), 0, 0, now, 26*time.Hour); err != nil || len(rows) != 0 {
		t.Fatalf("checks off: %v %+v", err, rows)
	}
}
