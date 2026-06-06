// Copyright 2026 damienstevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"

	"cipp-pp-cli/internal/store"
)

// seedStandardSnapshot drives the REAL production persistence path — the same
// appendStandardsSnapshot helper fanout's --save hook calls — so these tests
// fail if the production path ever stops producing comparable snapshots (the
// 4.19 print's bug: drift read the upsert-overwritten resources table, where a
// second save collapsed history and drift could never fire).
func seedStandardSnapshot(t *testing.T, db *store.Store, snapshotTS, tenant, standard, state string) {
	t.Helper()
	raw, _ := json.Marshal(map[string]any{
		"_tenant":  tenant,
		"Standard": standard,
		"State":    state,
	})
	if err := appendStandardsSnapshot(context.Background(), db, snapshotTS, tenant, []json.RawMessage{raw}); err != nil {
		t.Fatalf("append standards snapshot %s/%s: %v", snapshotTS, standard, err)
	}
}

func runDrift(t *testing.T, dbPath string) []driftRow {
	t.Helper()
	var flags rootFlags
	flags.asJSON = true
	cmd := newNovelStandardsDriftCmd(&flags)
	cmd.SetArgs([]string{"--db", dbPath})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute drift: %v", err)
	}
	var rows []driftRow
	if err := json.Unmarshal(out.Bytes(), &rows); err != nil {
		t.Fatalf("unmarshal drift output %q: %v", out.String(), err)
	}
	return rows
}

func TestStandardsDriftDetectsChange(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "data.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	tenant := "contoso.onmicrosoft.com"
	// Two saves of the SAME standard at different times — the production
	// shape. Snapshot 1 (earlier): MFA standard enabled.
	seedStandardSnapshot(t, db, "2026-05-01T00:00:00Z", tenant, "RequireMFA", "enabled")
	// Snapshot 2 (later): MFA standard regressed to disabled.
	seedStandardSnapshot(t, db, "2026-05-08T00:00:00Z", tenant, "RequireMFA", "disabled")
	db.Close()

	rows := runDrift(t, dbPath)
	if len(rows) != 1 {
		t.Fatalf("expected 1 drift row, got %d: %#v", len(rows), rows)
	}
	r := rows[0]
	if r.Tenant != tenant || r.Standard != "RequireMFA" || r.PreviousState != "enabled" || r.CurrentState != "disabled" {
		t.Fatalf("drift row wrong: %#v", r)
	}
	fmt.Printf("standards drift detected: %+v\n", r)
}

func TestStandardsDriftSingleSnapshotNoFabrication(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "data.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	// Only ONE snapshot exists → no drift may be fabricated.
	seedStandardSnapshot(t, db, "2026-05-01T00:00:00Z", "contoso.onmicrosoft.com", "RequireMFA", "enabled")
	db.Close()

	rows := runDrift(t, dbPath)
	if len(rows) != 0 {
		t.Fatalf("single snapshot must yield 0 drift rows, got %d: %#v", len(rows), rows)
	}
	fmt.Println("standards drift single-snapshot: 0 rows (no fabrication)")
}

func TestStandardsDriftNoHistoryTableIsEmpty(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "data.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	db.Close()

	// Fresh store, history table never created → empty result, no error.
	rows := runDrift(t, dbPath)
	if len(rows) != 0 {
		t.Fatalf("no-history store must yield 0 drift rows, got %d: %#v", len(rows), rows)
	}
	fmt.Println("standards drift no-history: 0 rows, no error")
}

func TestStandardsDriftUnchangedStateNoDrift(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "data.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	tenant := "contoso.onmicrosoft.com"
	// Two snapshots, identical state → no drift.
	seedStandardSnapshot(t, db, "2026-05-01T00:00:00Z", tenant, "RequireMFA", "enabled")
	seedStandardSnapshot(t, db, "2026-05-08T00:00:00Z", tenant, "RequireMFA", "enabled")
	db.Close()

	rows := runDrift(t, dbPath)
	if len(rows) != 0 {
		t.Fatalf("unchanged state must yield 0 drift rows, got %d: %#v", len(rows), rows)
	}
	fmt.Println("standards drift unchanged: 0 rows")
}
