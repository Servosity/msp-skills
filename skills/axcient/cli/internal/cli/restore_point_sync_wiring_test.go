// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-authored regression for issues #86 and #84: the live x360Recover
// per-device restore_point endpoint returns restore points GROUPED BY CLOUD
// VAULT — a top-level array of {vault_id, restore_point:[...]} wrappers whose
// timestamp-string restore_point_id ("YYYY_MM_DD_HH_MM_SS") lives ONLY on the
// inner array elements, with NO device_id and NO numeric id. The store-layer
// batch test (internal/store/restore_point_id_test.go) proves UpsertBatch
// composes rp:<device>:<rp_id> when device_id is already on the item. THIS test
// proves the two missing seams: (1) syncDependentResource descends into the
// nested restore_point[] array (the #84 live shape — the earlier flat-array
// fixture passed while the live tenant stored zero rows), and (2) it injects
// device_id into items that arrive WITHOUT it, so the full sync wiring populates
// the typed table instead of emitting all_items_failed_id_extraction.
//
// See handfixes.json: restore-point-id-synthesis, restore-point-nested-flatten.

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

func TestSyncDependentResource_RestorePointFlattensVaultGroupsAndPopulates(t *testing.T) {
	db, err := store.OpenWithContext(context.Background(), filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer db.Close()

	// The restore_point dependent iterates the device parent table.
	if err := db.Upsert("device", "7654321", json.RawMessage(`{"id":7654321,"name":"fs01"}`)); err != nil {
		t.Fatalf("seed device: %v", err)
	}

	// Exact LIVE shape from issue #84: a top-level array of vault-group wrappers,
	// each with a vault_id and a nested restore_point[] array. The timestamp
	// restore_point_id is on the INNER elements only; the wrapper has none. Two
	// vaults here (a device replicated to local + cloud), three points total.
	client := &stubRestorePointClient{payload: json.RawMessage(`[
		{"vault_id":"vault-local-01","restore_point":[
			{"in_use":false,"restore_point_id":"2026_06_11_18_15_00","timestamp":"2026-06-12T01:15:00Z","usage_initiator":""},
			{"in_use":false,"restore_point_id":"2026_06_11_19_15_00","timestamp":"2026-06-12T02:15:00Z","usage_initiator":""}
		]},
		{"vault_id":"vault-cloud-02","restore_point":[
			{"in_use":false,"restore_point_id":"2026_06_11_20_15_00","timestamp":"2026-06-12T03:15:00Z","usage_initiator":""}
		]}
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
	if res.Count != 3 {
		t.Fatalf("stored count = %d, want 3 (nested restore_point[] not flattened — the issue #84 symptom: outer vault wrappers fail ID extraction and store 0 rows)", res.Count)
	}
	if len(client.paths) == 0 || client.paths[0] != "/device/7654321/restore_point" {
		t.Fatalf("requested paths = %v, want first call to /device/7654321/restore_point", client.paths)
	}

	var rows int
	if err := db.DB().QueryRow(`SELECT COUNT(*) FROM "restore_point"`).Scan(&rows); err != nil {
		t.Fatalf("count restore_point: %v", err)
	}
	if rows != 3 {
		t.Fatalf("restore_point typed-table rows = %d, want 3", rows)
	}

	// device_id was injected from the parent and folded into the composite key.
	var id, deviceID, data string
	if err := db.DB().QueryRow(`SELECT id, device_id, data FROM "restore_point" ORDER BY id LIMIT 1`).Scan(&id, &deviceID, &data); err != nil {
		t.Fatalf("select restore_point row: %v", err)
	}
	if id != "rp:7654321:2026_06_11_18_15_00" {
		t.Fatalf("composite id = %q, want rp:7654321:2026_06_11_18_15_00", id)
	}
	if deviceID != "7654321" {
		t.Fatalf("injected device_id = %q, want 7654321", deviceID)
	}
	// vault_id from the wrapper is carried down onto the inner point so the row
	// records which vault it came from.
	var stored map[string]any
	if err := json.Unmarshal([]byte(data), &stored); err != nil {
		t.Fatalf("unmarshal stored row: %v", err)
	}
	if stored["vault_id"] != "vault-local-01" {
		t.Fatalf("carried-down vault_id = %v, want vault-local-01", stored["vault_id"])
	}
}

// TestSyncDependentResource_RestorePointFlatArrayPassthrough proves the
// flatten is backward-compatible: an inner-element flat array (no nested
// restore_point[] wrapper — the single-device write-through shape and any
// future flat response) passes through unchanged and still populates.
func TestSyncDependentResource_RestorePointFlatArrayPassthrough(t *testing.T) {
	db, err := store.OpenWithContext(context.Background(), filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer db.Close()

	if err := db.Upsert("device", "7654321", json.RawMessage(`{"id":7654321,"name":"fs01"}`)); err != nil {
		t.Fatalf("seed device: %v", err)
	}

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
		t.Fatalf("stored count = %d, want 2 (flat inner-element array regressed)", res.Count)
	}

	var id string
	if err := db.DB().QueryRow(`SELECT id FROM "restore_point" ORDER BY id LIMIT 1`).Scan(&id); err != nil {
		t.Fatalf("select restore_point row: %v", err)
	}
	if id != "rp:7654321:2026_06_11_18_15_00" {
		t.Fatalf("composite id = %q, want rp:7654321:2026_06_11_18_15_00", id)
	}
}

// TestSyncDependentResource_DryRunDoesNotMutateSyncState is the regression for
// the v0.2.0 fix (#80): a `sync --dry-run` must not write sync-state for cascaded
// dependent resources. The dependent loop returns on the {"dry_run":true} sentinel
// BEFORE its SaveSyncState, so a pre-existing checkpoint is preserved. The 4.24.0
// re-vendor kept the guard strings; this proves the BEHAVIOR survived (the same
// string-asserted-but-untested gap that let the rp:/av: storage-key regression
// ship unseen). See handfixes.json: sync-dryrun-no-mutation.
func TestSyncDependentResource_DryRunDoesNotMutateSyncState(t *testing.T) {
	db, err := store.OpenWithContext(context.Background(), filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer db.Close()

	if err := db.Upsert("device", "7654321", json.RawMessage(`{"id":7654321,"name":"fs01"}`)); err != nil {
		t.Fatalf("seed device: %v", err)
	}
	// Pre-existing checkpoint a dry run must leave untouched.
	if err := db.SaveSyncState("restore_point", "cursor-XYZ", 99); err != nil {
		t.Fatalf("seed sync_state: %v", err)
	}

	// The client returns the dry-run sentinel the real client emits under --dry-run.
	client := &stubRestorePointClient{payload: json.RawMessage(`{"dry_run": true}`)}

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
		t.Fatalf("syncDependentResource (dry-run) returned error: %v", res.Err)
	}

	cursor, _, count, err := db.GetSyncState("restore_point")
	if err != nil {
		t.Fatalf("GetSyncState: %v", err)
	}
	if cursor != "cursor-XYZ" || count != 99 {
		t.Fatalf("dry-run mutated sync_state: cursor=%q count=%d, want cursor-XYZ/99 (the #80 regression)", cursor, count)
	}
}
