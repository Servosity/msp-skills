// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"testing"

	"syncro-pp-cli/internal/store"
)

func TestNovelAssetsPatchGapsCommand(t *testing.T) {
	tests := []struct {
		name        string
		seed        func(t *testing.T, db *store.Store)
		args        []string
		wantAssets  int
		wantMissing int
		topAssetID  string
		topMissing  int
	}{
		{
			name: "empty store",
			seed: func(t *testing.T, db *store.Store) {},
		},
		{
			name: "counts missing patches per asset; installed excluded",
			seed: func(t *testing.T, db *store.Store) {
				seedResource(t, db, "customers", "C1", map[string]any{"id": "C1", "business_name": "Acme"})
				seedCustomerAsset(t, db, "A1", map[string]any{"id": "A1", "name": "host-1", "customer_id": "C1"})
				seedCustomerAsset(t, db, "A2", map[string]any{"id": "A2", "name": "host-2", "customer_id": "C1"})
				// A1: 2 missing
				seedPatch(t, db, "P1", "A1", map[string]any{"status": "missing", "kb": "KB1", "severity": "Critical"})
				seedPatch(t, db, "P2", "A1", map[string]any{"status": "pending", "kb": "KB2", "severity": "Important"})
				// A1: installed -> excluded
				seedPatch(t, db, "P3", "A1", map[string]any{"status": "installed", "kb": "KB3"})
				// A2: 1 missing
				seedPatch(t, db, "P4", "A2", map[string]any{"status": "available", "kb": "KB4", "severity": "Critical"})
			},
			args:        []string{},
			wantAssets:  2,
			wantMissing: 3,
			topAssetID:  "A1",
			topMissing:  2,
		},
		{
			name: "severity filter narrows results",
			seed: func(t *testing.T, db *store.Store) {
				seedCustomerAsset(t, db, "A1", map[string]any{"id": "A1", "name": "host-1", "customer_id": "C1"})
				seedPatch(t, db, "P1", "A1", map[string]any{"status": "missing", "kb": "KB1", "severity": "Critical"})
				seedPatch(t, db, "P2", "A1", map[string]any{"status": "missing", "kb": "KB2", "severity": "Low"})
			},
			args:        []string{"--severity", "critical"},
			wantAssets:  1,
			wantMissing: 1,
			topAssetID:  "A1",
			topMissing:  1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			db, dbPath := newNovelTestStore(t)
			tc.seed(t, db)

			cmd := newNovelAssetsPatchGapsCmd(&rootFlags{asJSON: true})
			args := append([]string{"--db", dbPath}, tc.args...)
			parsed, _ := execNovel(t, cmd, args)

			if got := int(jsonFloat(t, parsed, "total_assets")); got != tc.wantAssets {
				t.Errorf("total_assets = %d, want %d", got, tc.wantAssets)
			}
			if got := int(jsonFloat(t, parsed, "total_missing")); got != tc.wantMissing {
				t.Errorf("total_missing = %d, want %d", got, tc.wantMissing)
			}
			items := jsonItems(t, parsed)
			if tc.topAssetID != "" {
				if len(items) == 0 {
					t.Fatalf("expected items, got none")
				}
				top := items[0]
				if top["asset_id"] != tc.topAssetID {
					t.Errorf("top asset = %v, want %s", top["asset_id"], tc.topAssetID)
				}
				if int(top["missing_patches"].(float64)) != tc.topMissing {
					t.Errorf("top missing = %v, want %d", top["missing_patches"], tc.topMissing)
				}
			}
		})
	}
}
