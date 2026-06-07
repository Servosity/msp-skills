// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func openCoveTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestInsertAndQueryCoveSnapshots(t *testing.T) {
	s := openCoveTestStore(t)
	ctx := context.Background()

	t0 := time.Now().Add(-48 * time.Hour).UTC().Truncate(time.Second)
	t1 := time.Now().UTC().Truncate(time.Second)

	id0, err := s.InsertCoveSnapshot(ctx, t0, 1234, []CoveDeviceStat{
		{AccountID: 1, PartnerID: 10, DeviceName: "host-a", Customer: "Acme", UsedStorage: 100, LastStatus: 5, LastSuccessAt: t0.Unix(), SKU: "DOC"},
		{AccountID: 2, PartnerID: 11, DeviceName: "host-b", Customer: "Globex", UsedStorage: 200, LastStatus: 2, SKU: "SRV"},
	})
	if err != nil {
		t.Fatalf("insert snapshot 0: %v", err)
	}
	id1, err := s.InsertCoveSnapshot(ctx, t1, 1234, []CoveDeviceStat{
		{AccountID: 1, PartnerID: 10, DeviceName: "host-a", Customer: "Acme", UsedStorage: 150, LastStatus: 2, LastSuccessAt: t0.Unix(), SKU: "DOC", PrevSKU: "DOC"},
		{AccountID: 3, PartnerID: 11, DeviceName: "host-c", Customer: "Globex", UsedStorage: 50, LastStatus: 5, SKU: "WKS"},
	})
	if err != nil {
		t.Fatalf("insert snapshot 1: %v", err)
	}
	if id1 <= id0 {
		t.Fatalf("expected increasing snapshot ids, got %d then %d", id0, id1)
	}

	latest, ok, err := s.LatestCoveSnapshot(ctx)
	if err != nil || !ok || latest.ID != id1 || latest.DeviceCount != 2 {
		t.Fatalf("latest: %+v ok=%v err=%v", latest, ok, err)
	}

	base, ok, err := s.CoveSnapshotBefore(ctx, t1.Add(-time.Hour))
	if err != nil || !ok || base.ID != id0 {
		t.Fatalf("before: %+v ok=%v err=%v", base, ok, err)
	}

	oldest, ok, err := s.OldestCoveSnapshotSince(ctx, t0.Add(-time.Hour))
	if err != nil || !ok || oldest.ID != id0 {
		t.Fatalf("oldest since: %+v ok=%v err=%v", oldest, ok, err)
	}

	rows, err := s.CoveDeviceStats(ctx, id1)
	if err != nil {
		t.Fatalf("device stats: %v", err)
	}
	if len(rows) != 2 || rows[1].UsedStorage != 150 || rows[3].DeviceName != "host-c" {
		t.Fatalf("unexpected rows: %+v", rows)
	}

	n, err := s.CoveSnapshotCount(ctx)
	if err != nil || n != 2 {
		t.Fatalf("count: %d err=%v", n, err)
	}
}

func TestCoveSnapshotEmptyStates(t *testing.T) {
	s := openCoveTestStore(t)
	ctx := context.Background()

	if _, ok, err := s.LatestCoveSnapshot(ctx); err != nil || ok {
		t.Fatalf("expected no latest snapshot, ok=%v err=%v", ok, err)
	}
	if _, ok, err := s.CoveSnapshotBefore(ctx, time.Now()); err != nil || ok {
		t.Fatalf("expected no snapshot before now, ok=%v err=%v", ok, err)
	}
	if n, err := s.CoveSnapshotCount(ctx); err != nil || n != 0 {
		t.Fatalf("expected zero count, n=%d err=%v", n, err)
	}
}

func TestMarshalSettings(t *testing.T) {
	if got := MarshalSettings(nil); got != "{}" {
		t.Fatalf("nil settings: %s", got)
	}
	got := MarshalSettings(map[string]string{"I1": "host"})
	if got != `{"I1":"host"}` {
		t.Fatalf("settings json: %s", got)
	}
}
