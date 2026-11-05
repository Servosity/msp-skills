// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored tests for the fleet-diff snapshot store: every snapshot this
// version writes records its kinds, and a store from before kinds were
// recorded still reads, read-only, as unversioned.

package store

import (
	"path/filepath"
	"slices"
	"testing"
)

func TestImmyFleetSnapshotRefusesUnversionedWrite(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "immy.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()
	ctx := t.Context()
	row := ImmyFleetRow{ResourceType: "computer", ID: "1", Fingerprint: "f"}
	for name, kinds := range map[string]map[string]string{
		"no kinds":               nil,
		"a kind without shape":   {"computer": ""},
		"row of unrecorded kind": {"deployment": "shape"},
	} {
		if err := s.WriteImmyFleetSnapshot(ctx, "2026-10-01T00:00:00.000000000Z", kinds, []ImmyFleetRow{row}); err == nil {
			t.Fatalf("%s: write succeeded, want refusal", name)
		}
	}
	times, err := s.ImmyFleetSnapshotTimes(ctx)
	if err != nil || len(times) != 0 {
		t.Fatalf("times = %v, %v; a refused write must leave no generation", times, err)
	}
}

func TestImmyFleetSnapshotKindsOnPreKindsStore(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "immy.db")
	s, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	ctx := t.Context()
	// Exactly what immybot v0.1.x left behind: the snapshot table alone.
	for _, stmt := range []string{immyFleetSnapshotDDL,
		`INSERT INTO immy_fleet_snapshots (taken_at, resource_type, id, fingerprint, label) VALUES ('2026-10-01T00:00:00Z', 'deployment', '5', 'f', 'Chrome')`} {
		if _, err := s.DB().ExecContext(ctx, stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	_ = s.Close()

	ro, err := OpenReadOnlyContext(ctx, dbPath)
	if err != nil {
		t.Fatalf("open read-only: %v", err)
	}
	times, err := ro.ImmyFleetSnapshotTimes(ctx)
	if err != nil || !slices.Equal(times, []string{"2026-10-01T00:00:00Z"}) {
		t.Fatalf("read-only times = %v, %v; want the legacy generation", times, err)
	}
	kinds, versioned, err := ro.ReadImmyFleetSnapshotKinds(ctx, times[0])
	if err != nil || versioned || len(kinds) != 0 {
		t.Fatalf("read-only kinds = %v versioned=%v err=%v; want unversioned without error", kinds, versioned, err)
	}
	_ = ro.Close()

	// A new generation adds the kinds table; the legacy one stays unversioned.
	rw, err := Open(dbPath)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer rw.Close()
	const newAt = "2026-10-02T00:00:00.000000000Z"
	if err := rw.WriteImmyFleetSnapshot(ctx, newAt, map[string]string{"computer": "c", "deployment": "d"}, nil); err != nil {
		t.Fatalf("write: %v", err)
	}
	times, err = rw.ImmyFleetSnapshotTimes(ctx)
	if err != nil || !slices.Equal(times, []string{newAt, "2026-10-01T00:00:00Z"}) {
		t.Fatalf("times = %v, %v; want the empty new generation listed before the legacy one", times, err)
	}
	if _, versioned, err := rw.ReadImmyFleetSnapshotKinds(ctx, "2026-10-01T00:00:00Z"); err != nil || versioned {
		t.Fatalf("legacy generation versioned=%v err=%v, want unversioned", versioned, err)
	}
	kinds, versioned, err = rw.ReadImmyFleetSnapshotKinds(ctx, newAt)
	if err != nil || !versioned || kinds["computer"] != "c" || kinds["deployment"] != "d" || len(kinds) != 2 {
		t.Fatalf("new generation kinds = %v versioned=%v err=%v", kinds, versioned, err)
	}
}
