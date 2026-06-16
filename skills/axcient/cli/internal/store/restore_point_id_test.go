// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-authored regression tests for the restore_point ID-extraction hand-fix.
// The live x360Recover restore-point endpoint returns items keyed by a
// per-device timestamp string (restore_point_id, "YYYY_MM_DD_HH_MM_SS") with no
// numeric id. Before the fix, ExtractResourceID found nothing, UpsertBatch
// skipped every item, and the restore_point table stored zero rows
// (all_items_failed_id_extraction). See handfixes.json:restore-point-id-synthesis.

package store

import (
	"encoding/json"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

// realRestorePointItem mirrors the live API item shape (no id; restore_point_id
// is a timestamp string). device_id/parent_id are injected by the sync loop.
func realRestorePointItem(rpID, deviceID string) json.RawMessage {
	return json.RawMessage(`{"in_use":false,"restore_point_id":"` + rpID +
		`","timestamp":"2026-06-12T01:15:00Z","usage_initiator":"","device_id":"` +
		deviceID + `","parent_id":"` + deviceID + `"}`)
}

func TestExtractResourceID_RestorePointTimestampKey(t *testing.T) {
	cases := []struct {
		name         string
		resourceType string
		obj          map[string]any
		want         string
	}{
		{
			name:         "sync path composes device_id prefix",
			resourceType: "restore_point",
			obj:          map[string]any{"restore_point_id": "2026_06_11_18_15_00", "device_id": "7654321"},
			want:         "rp:7654321:2026_06_11_18_15_00",
		},
		{
			name:         "parent_id used when device_id absent",
			resourceType: "restore_point",
			obj:          map[string]any{"restore_point_id": "2026_06_11_18_15_00", "parent_id": "7654321"},
			want:         "rp:7654321:2026_06_11_18_15_00",
		},
		{
			name:         "live single-device path falls back to bare restore_point_id",
			resourceType: "restore_point",
			obj:          map[string]any{"restore_point_id": "2026_06_11_18_15_00"},
			want:         "2026_06_11_18_15_00",
		},
		{
			name:         "hyphenated live resourceType alias also resolves",
			resourceType: "restore-point",
			obj:          map[string]any{"restore_point_id": "2026_06_11_18_15_00", "device_id": "7654321"},
			want:         "rp:7654321:2026_06_11_18_15_00",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ExtractResourceID(tc.resourceType, tc.obj); got != tc.want {
				t.Fatalf("ExtractResourceID(%q) = %q, want %q", tc.resourceType, got, tc.want)
			}
		})
	}
}

// TestUpsertBatch_RestorePointRealShapePopulates is the regression for the
// reported defect: the real item shape must land rows, not return zero.
func TestUpsertBatch_RestorePointRealShapePopulates(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()

	items := []json.RawMessage{
		realRestorePointItem("2026_06_11_18_15_00", "7654321"),
		realRestorePointItem("2026_06_11_19_15_00", "7654321"),
		realRestorePointItem("2026_06_11_20_15_00", "7654321"),
	}
	stored, extractFailures, err := s.UpsertBatch("restore_point", items)
	if err != nil {
		t.Fatalf("UpsertBatch: %v", err)
	}
	if stored != len(items) {
		t.Fatalf("stored = %d, want %d (restore_point items failed ID extraction)", stored, len(items))
	}
	if extractFailures != 0 {
		t.Fatalf("extractFailures = %d, want 0", extractFailures)
	}

	var typed int
	if err := s.DB().QueryRow(`SELECT COUNT(*) FROM "restore_point"`).Scan(&typed); err != nil {
		t.Fatalf("count restore_point: %v", err)
	}
	if typed != len(items) {
		t.Fatalf("restore_point typed-table count = %d, want %d", typed, len(items))
	}

	var gotID string
	if err := s.DB().QueryRow(`SELECT id FROM "restore_point" WHERE device_id = ? ORDER BY id LIMIT 1`, "7654321").Scan(&gotID); err != nil {
		t.Fatalf("select id: %v", err)
	}
	if gotID != "rp:7654321:2026_06_11_18_15_00" {
		t.Fatalf("stored id = %q, want composite rp:7654321:2026_06_11_18_15_00", gotID)
	}
}

// TestUpsertBatch_RestorePointNoCrossDeviceCollision proves the composite key
// keeps same-timestamp restore points on different devices as distinct rows.
// A bare restore_point_id key would collapse them via ON CONFLICT("id").
func TestUpsertBatch_RestorePointNoCrossDeviceCollision(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()

	items := []json.RawMessage{
		realRestorePointItem("2026_06_11_18_15_00", "111"),
		realRestorePointItem("2026_06_11_18_15_00", "222"),
	}
	if _, _, err := s.UpsertBatch("restore_point", items); err != nil {
		t.Fatalf("UpsertBatch: %v", err)
	}

	var typed int
	if err := s.DB().QueryRow(`SELECT COUNT(*) FROM "restore_point"`).Scan(&typed); err != nil {
		t.Fatalf("count restore_point: %v", err)
	}
	if typed != 2 {
		t.Fatalf("restore_point count = %d, want 2 (same-timestamp points on two devices collided)", typed)
	}
}
