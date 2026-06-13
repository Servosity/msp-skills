// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-authored regression for issue #86: the live x360Recover per-device
// restore_point endpoint returns items keyed by a timestamp string
// (restore_point_id, "YYYY_MM_DD_HH_MM_SS") with NO device_id and NO numeric
// id. The store-layer batch test (internal/store/restore_point_id_test.go)
// proves UpsertBatch composes rp:<device>:<rp_id> when device_id is already on
// the item. THIS test proves the missing seam: syncDependentResource injects
// device_id into items that arrive WITHOUT it (as the API returns them) before
// the batch upsert, so the full sync wiring populates the typed table instead
// of emitting all_items_failed_id_extraction and storing zero rows.
//
// See handfixes.json: restore-point-id-synthesis.

package cli

import (
	"context"
	"encoding/json"
	"io"
	"path/filepath"
	"testing"

	"axcient-pp-cli/internal/store"
)

// stubRestorePointClient returns a fixed per-device restore_point payload,
// matching the live API shape (no device_id, no numeric id).
type stubRestorePointClient struct {
	payload json.RawMessage
	paths   []string
}

func (s *stubRestorePointClient) Get(_ context.Context, path string, _ map[string]string) (json.RawMessage, error) {
	s.paths = append(s.paths, path)
	return s.payload, nil
}

func (s *stubRestorePointClient) RateLimit() float64 { return 0 }

func TestSyncDependentResource_RestorePointInjectsDeviceIDAndPopulates(t *testing.T) {
	db, err := store.OpenWithContext(context.Background(), filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer db.Close()

	// The restore_point dependent iterates the device parent table.
	if err := db.Upsert("device", "7654321", json.RawMessage(`{"id":7654321,"name":"fs01"}`)); err != nil {
		t.Fatalf("seed device: %v", err)
	}

	// Exact item shape from issue #86 — timestamp-string id, no device_id.
	client := &stubRestorePointClient{payload: json.RawMessage(`[
		{"in_use":false,"restore_point_id":"2026_06_11_18_15_00","timestamp":"2026-06-12T01:15:00Z","usage_initiator":""},
		{"in_use":false,"restore_point_id":"2026_06_11_19_15_00","timestamp":"2026-06-12T02:15:00Z","usage_initiator":""}
	]`)}

	var dep dependentResourceDef
	for _, d := range dependentResourceDefs() {
		if d.Name == "restore_point" {
			dep = d
			break
		}
	}
	if dep.Name == "" {
		t.Fatal("restore_point dependent definition not found")
	}

	res := syncDependentResource(context.Background(), client, db, dep, "", false, 0, false, &syncUserParams{}, io.Discard)
	if res.Err != nil {
		t.Fatalf("syncDependentResource returned error: %v", res.Err)
	}
	if res.Count != 2 {
		t.Fatalf("stored count = %d, want 2 (restore_point items failed ID extraction in the sync wiring — the issue #86 symptom)", res.Count)
	}
	if len(client.paths) == 0 || client.paths[0] != "/device/7654321/restore_point" {
		t.Fatalf("requested paths = %v, want first call to /device/7654321/restore_point", client.paths)
	}

	var rows int
	if err := db.DB().QueryRow(`SELECT COUNT(*) FROM "restore_point"`).Scan(&rows); err != nil {
		t.Fatalf("count restore_point: %v", err)
	}
	if rows != 2 {
		t.Fatalf("restore_point typed-table rows = %d, want 2", rows)
	}

	// device_id was injected from the parent and folded into the composite key.
	var id, deviceID string
	if err := db.DB().QueryRow(`SELECT id, device_id FROM "restore_point" ORDER BY id LIMIT 1`).Scan(&id, &deviceID); err != nil {
		t.Fatalf("select restore_point row: %v", err)
	}
	if id != "rp:7654321:2026_06_11_18_15_00" {
		t.Fatalf("composite id = %q, want rp:7654321:2026_06_11_18_15_00", id)
	}
	if deviceID != "7654321" {
		t.Fatalf("injected device_id = %q, want 7654321", deviceID)
	}
}
