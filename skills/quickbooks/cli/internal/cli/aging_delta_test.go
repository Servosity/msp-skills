// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"quickbooks-pp-cli/internal/analytics"
	"quickbooks-pp-cli/internal/store"
)

// TestAgingSnapshotRoundTrip proves the aging_snapshots table migration runs
// and that save → load returns the same snapshot (the substrate aging-delta
// diffs against).
func TestAgingSnapshotRoundTrip(t *testing.T) {
	ctx := context.Background()
	db, err := store.OpenWithContext(ctx, filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	// First load on a fresh store: no snapshot, no error.
	prev, err := loadLatestAgingSnapshot(ctx, db)
	if err != nil {
		t.Fatalf("first load: %v", err)
	}
	if prev != nil {
		t.Fatalf("fresh store must have no snapshot, got %+v", prev)
	}

	snap := analytics.AgingSnapshot{
		TakenAt: "2026-06-06T12:00:00Z",
		AR: analytics.AgingReport{
			AsOf: "2026-06-06", Total: 150,
			Buckets:  map[string]float64{"0-30": 150, "31-60": 0, "61-90": 0, "90+": 0},
			Entities: []analytics.AgingEntityRow{{Name: "Acme", ID: "c1", Total: 150, Buckets: map[string]float64{"0-30": 150}}},
		},
		AP: analytics.AgingReport{Buckets: map[string]float64{}},
	}
	if err := saveAgingSnapshot(ctx, db, snap); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := loadLatestAgingSnapshot(ctx, db)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got == nil || got.TakenAt != snap.TakenAt || got.AR.Total != 150 || len(got.AR.Entities) != 1 {
		t.Fatalf("round trip mismatch: %+v", got)
	}

	// Later snapshot wins.
	snap2 := snap
	snap2.TakenAt = "2026-06-07T12:00:00Z"
	snap2.AR.Total = 200
	if err := saveAgingSnapshot(ctx, db, snap2); err != nil {
		t.Fatalf("save 2: %v", err)
	}
	latest, err := loadLatestAgingSnapshot(ctx, db)
	if err != nil {
		t.Fatalf("load 2: %v", err)
	}
	if latest.TakenAt != snap2.TakenAt || latest.AR.Total != 200 {
		t.Fatalf("latest snapshot not returned: %+v", latest)
	}
}

// TestLoadResourcesDecodesSeededRows proves the loadResources helper the new
// novels share decodes seeded store rows (json.Number-safe) for analytics.
func TestLoadResourcesDecodesSeededRows(t *testing.T) {
	ctx := context.Background()
	db, err := store.OpenWithContext(ctx, filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	doc := map[string]any{"Id": "i1", "Balance": 123.45, "CustomerRef": map[string]any{"value": "c1", "name": "Acme"}}
	raw, _ := json.Marshal(doc)
	if err := db.Upsert("invoices", "i1", raw); err != nil {
		t.Fatalf("seed: %v", err)
	}
	rows, err := loadResources(ctx, db, "invoices")
	if err != nil {
		t.Fatalf("loadResources: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("want 1 row, got %d", len(rows))
	}
	if analytics.Num(rows[0], "Balance") != 123.45 {
		t.Fatalf("balance decode wrong: %+v", rows[0])
	}
	if analytics.Nested(rows[0], "CustomerRef", "name") != "Acme" {
		t.Fatalf("nested ref decode wrong: %+v", rows[0])
	}
}
