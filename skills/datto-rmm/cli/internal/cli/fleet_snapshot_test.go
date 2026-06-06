// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"path/filepath"
	"testing"

	"datto-rmm-pp-cli/internal/store"
)

func openSnapshotTestStore(t *testing.T) *store.Store {
	t.Helper()
	db, err := store.OpenWithContext(context.Background(), filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestCreateAndListFleetSnapshots(t *testing.T) {
	db := openSnapshotTestStore(t)
	ctx := context.Background()

	if err := db.Upsert("account-devices", "d1", rawJSON(`{"uid":"d1","hostname":"alpha","siteName":"Acme"}`)); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := db.Upsert("account-devices", "d2", rawJSON(`{"uid":"d2","hostname":"bravo","siteName":"Acme"}`)); err != nil {
		t.Fatalf("seed: %v", err)
	}

	meta, err := db.CreateFleetSnapshot(ctx, "q1", "baseline")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if meta.Label != "q1" || meta.RowCount != 2 || meta.Note != "baseline" {
		t.Fatalf("meta wrong: %+v", meta)
	}

	// Labels are immutable receipts.
	if _, err := db.CreateFleetSnapshot(ctx, "q1", "again"); err == nil {
		t.Fatalf("duplicate label should fail")
	}

	metas, err := db.ListFleetSnapshots(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(metas) != 1 || metas[0].Label != "q1" {
		t.Fatalf("list wrong: %+v", metas)
	}

	raws, err := db.SnapshotResourceData(ctx, "q1", "account-devices")
	if err != nil {
		t.Fatalf("rows: %v", err)
	}
	if len(raws) != 2 {
		t.Fatalf("snapshot rows = %d, want 2", len(raws))
	}

	devices := decodeFleetDevices(raws)
	if len(devices) != 2 {
		t.Fatalf("decoded = %d, want 2", len(devices))
	}

	ok, err := db.SnapshotExists(ctx, "q1")
	if err != nil || !ok {
		t.Fatalf("exists(q1) = %v, %v", ok, err)
	}
	ok, err = db.SnapshotExists(ctx, "missing")
	if err != nil || ok {
		t.Fatalf("exists(missing) = %v, %v", ok, err)
	}
}

func TestSnapshotIsFrozenAgainstLaterChanges(t *testing.T) {
	db := openSnapshotTestStore(t)
	ctx := context.Background()

	if err := db.Upsert("account-devices", "d1", rawJSON(`{"uid":"d1","hostname":"alpha"}`)); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := db.CreateFleetSnapshot(ctx, "before", ""); err != nil {
		t.Fatalf("create: %v", err)
	}
	// Mutate the live store after the snapshot.
	if err := db.Upsert("account-devices", "d2", rawJSON(`{"uid":"d2","hostname":"bravo"}`)); err != nil {
		t.Fatalf("seed: %v", err)
	}

	raws, err := db.SnapshotResourceData(ctx, "before", "account-devices")
	if err != nil {
		t.Fatalf("rows: %v", err)
	}
	if len(raws) != 1 {
		t.Fatalf("snapshot leaked later writes: %d rows, want 1", len(raws))
	}
}
