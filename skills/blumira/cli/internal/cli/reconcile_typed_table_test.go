// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"blumira-pp-cli/internal/store"
)

// blumira handfix (sync-partition-names-pinned): the published partition names
// swap "org" (users) and "org-agents-devices" (devices) relative to the 4.35.1
// render. A full sync prunes rows the API no longer returns through
// ReconcileAll with reconcileTypedTable(resource); the typed table it names
// must be the one UpsertBatch writes for that same resource, or the prune
// fails ("no such table") and a removed device stays in the store, where
// exposure and dc-roster keep reporting it. Every synced partition: store a
// row, prune it as no longer returned, and require the prune to succeed and
// to leave no row behind in any typed table.
func TestReconcileTypedTableMatchesUpsertRoute(t *testing.T) {
	item := json.RawMessage(`{"id":"i1","account_id":"a1","finding_id":"f1","device_id":"d1","key_id":"k1","org_id":"o1","hostname":"dc01","email":"u@example.com","name":"n"}`)
	for _, resource := range knownSyncResourceNames() {
		t.Run(resource, func(t *testing.T) {
			s, err := store.Open(filepath.Join(t.TempDir(), "data.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			if stored, _, err := s.UpsertBatch(resource, []json.RawMessage{item}); err != nil || stored != 1 {
				t.Fatalf("UpsertBatch(%s) stored=%d err=%v", resource, stored, err)
			}
			deleted, err := s.ReconcileAll(resource, nil, reconcileTypedTable(resource), store.CascadeJunctionsFor(resource))
			if err != nil {
				t.Fatalf("pruning %s (typed table %q) failed: %v", resource, reconcileTypedTable(resource), err)
			}
			if deleted != 1 {
				t.Fatalf("pruning %s deleted %d rows, want 1", resource, deleted)
			}
			for _, typed := range typedTablesForTest(t, s) {
				var n int
				if err := s.DB().QueryRow(`SELECT COUNT(*) FROM "` + typed + `"`).Scan(&n); err != nil {
					t.Fatal(err)
				}
				if n != 0 {
					t.Fatalf("pruning %s left %d row(s) in typed table %q", resource, n, typed)
				}
			}
		})
	}
}

// typedTablesForTest lists every per-resource typed table the store schema
// emits (a table with id and data columns other than the shared resources
// table), so a typed table a later print adds is checked too.
func typedTablesForTest(t *testing.T, s *store.Store) []string {
	t.Helper()
	rows, err := s.DB().Query(`SELECT name FROM sqlite_master WHERE type = 'table' AND name <> 'resources' AND name NOT LIKE '%fts%'`)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		names = append(names, name)
	}
	_ = rows.Close()
	var typed []string
	for _, name := range names {
		var hasID, hasData int
		if err := s.DB().QueryRow(`SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = 'id'`, name).Scan(&hasID); err != nil {
			t.Fatal(err)
		}
		if err := s.DB().QueryRow(`SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = 'data'`, name).Scan(&hasData); err != nil {
			t.Fatal(err)
		}
		if hasID == 1 && hasData == 1 {
			typed = append(typed, name)
		}
	}
	if len(typed) < 2 {
		t.Fatalf("found typed tables %v; want at least msp and org", typed)
	}
	return typed
}
