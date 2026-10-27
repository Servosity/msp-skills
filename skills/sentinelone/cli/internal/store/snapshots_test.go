// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

// TestEmptyCaptureIsStillACapture: a complete walk that returns zero agents is
// a real fleet state ("the last agent was removed"). If the capture leaves no
// trace, every history reader keeps diffing the previous non-empty capture and
// never reports the removal.
func TestEmptyCaptureIsStillACapture(t *testing.T) {
	s, err := OpenWithContext(context.Background(), filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.Upsert("agents", "a1", []byte(`{"id":"a1","computerName":"WS-01"}`)); err != nil {
		t.Fatal(err)
	}
	t1 := time.Now().Add(-2 * time.Hour).UTC().Truncate(time.Second)
	t2 := time.Now().Add(-1 * time.Hour).UTC().Truncate(time.Second)
	if _, err := s.CaptureFleetSnapshotFor(t1, []string{"agents"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB().Exec(`DELETE FROM resources WHERE resource_type = 'agents'`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CaptureFleetSnapshotFor(t2, []string{"agents"}); err != nil {
		t.Fatal(err)
	}
	times, err := s.FleetSnapshotTimes("agents")
	if err != nil {
		t.Fatal(err)
	}
	if len(times) != 2 || !times[1].Equal(t2) {
		t.Fatalf("capture times = %v, want [%v %v] (the empty capture must be recorded)", times, t1, t2)
	}
	latest, ok, err := s.FleetSnapshotLatest("agents")
	if err != nil || !ok || !latest.Equal(t2) {
		t.Fatalf("latest = %v ok=%v err=%v, want %v", latest, ok, err, t2)
	}
	if rows, _ := s.FleetSnapshotRowsByID("agents", t2); len(rows) != 0 {
		t.Fatalf("empty capture holds rows: %v", rows)
	}
	if before, ok, _ := s.FleetSnapshotNearestBefore("agents", t2.Add(time.Minute)); !ok || !before.Equal(t2) {
		t.Fatalf("nearest-before = %v ok=%v, want the empty capture %v", before, ok, t2)
	}
}

// TestCapturesArePrunedTogether: retention keeps the newest
// snapshotKeepCaptures capture times in both the rows and the capture log.
func TestCapturesArePrunedTogether(t *testing.T) {
	s, err := OpenWithContext(context.Background(), filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.Upsert("agents", "a1", []byte(`{"id":"a1"}`)); err != nil {
		t.Fatal(err)
	}
	base := time.Now().Add(-100 * time.Hour).UTC().Truncate(time.Second)
	for i := 0; i < snapshotKeepCaptures+3; i++ {
		if _, err := s.CaptureFleetSnapshotFor(base.Add(time.Duration(i)*time.Minute), []string{"agents"}); err != nil {
			t.Fatal(err)
		}
	}
	times, _ := s.FleetSnapshotTimes("agents")
	if len(times) != snapshotKeepCaptures || !times[0].Equal(base.Add(3*time.Minute)) {
		t.Fatalf("kept %d captures starting %v, want %d starting %v", len(times), times[0], snapshotKeepCaptures, base.Add(3*time.Minute))
	}
	var n int
	if err := s.DB().QueryRow(`SELECT COUNT(DISTINCT captured_at) FROM fleet_snapshots WHERE entity = 'agents'`).Scan(&n); err != nil || n != snapshotKeepCaptures {
		t.Fatalf("snapshot rows span %d captures (err %v), want %d", n, err, snapshotKeepCaptures)
	}
}
