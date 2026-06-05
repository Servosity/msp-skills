// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"mspbots-pp-cli/internal/store"
)

// TestSnapshotStoreRoundTrip exercises the store layer the snapshot command
// writes through: insert with identity keys, list newest-first, load rows.
func TestSnapshotStoreRoundTrip(t *testing.T) {
	ctx := context.Background()
	db, err := store.OpenWithContext(ctx, filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.EnsureMspbotsSchema(ctx); err != nil {
		t.Fatal(err)
	}

	rows := []json.RawMessage{
		json.RawMessage(`{"id":1,"Status":"Open"}`),
		json.RawMessage(`{"id":2,"Status":"Closed"}`),
	}
	keys := make([]string, len(rows))
	for i, r := range rows {
		keys[i] = rowIdentityKey(r)
	}
	id1, err := db.SnapshotInsert(ctx, store.SnapshotMeta{
		Alias: "open-tickets", ResourceID: "1534956341424005122", ResourceType: "dataset",
		TakenAt: "2026-06-01T00:00:00Z", RowCount: len(rows), Pages: 1,
	}, rows, keys)
	if err != nil {
		t.Fatalf("insert 1: %v", err)
	}
	id2, err := db.SnapshotInsert(ctx, store.SnapshotMeta{
		Alias: "open-tickets", ResourceID: "1534956341424005122", ResourceType: "dataset",
		TakenAt: "2026-06-02T00:00:00Z", RowCount: 1, Pages: 1,
	}, rows[:1], keys[:1])
	if err != nil {
		t.Fatalf("insert 2: %v", err)
	}

	snaps, err := db.SnapshotsForAlias(ctx, "open-tickets", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(snaps) != 2 || snaps[0].ID != id2 || snaps[1].ID != id1 {
		t.Fatalf("snapshots not newest-first: %+v", snaps)
	}

	gotKeys, gotRows, err := db.SnapshotRows(ctx, id1)
	if err != nil {
		t.Fatal(err)
	}
	if len(gotRows) != 2 || gotKeys[0] != "id:1" || gotKeys[1] != "id:2" {
		t.Fatalf("rows round-trip: keys=%v rows=%d", gotKeys, len(gotRows))
	}

	// Mismatched rows/keys must be rejected, not silently truncated.
	if _, err := db.SnapshotInsert(ctx, store.SnapshotMeta{Alias: "x"}, rows, keys[:1]); err == nil {
		t.Fatal("rows/keys mismatch should error")
	}
}
