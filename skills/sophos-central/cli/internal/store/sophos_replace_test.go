// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: pinned by skills/sophos-central/handfixes.json under
// "fleet-rows-replace".

package store

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestReplaceBatchReplacesAtomically(t *testing.T) {
	db, err := OpenWithContext(context.Background(), filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	a := `{"id": "00000000-0000-0000-0000-000000000001", "tenant": {"id": "T1"}, "assignedProducts": [{"code": "deviceEncryption"}]}`
	if _, _, err := db.UpsertBatch("endpoints", []json.RawMessage{json.RawMessage(a)}); err != nil {
		t.Fatal(err)
	}
	// A write that fails mid-batch rolls back the whole batch.
	if _, err := db.DB().Exec(`CREATE TRIGGER fail_two BEFORE INSERT ON resources WHEN NEW.id = '00000000-0000-0000-0000-000000000002' BEGIN SELECT RAISE(ABORT, 'boom'); END`); err != nil {
		t.Fatal(err)
	}
	cleared := `{"id": "00000000-0000-0000-0000-000000000001", "tenant": {"id": "T1"}, "assignedProducts": []}`
	two := `{"id": "00000000-0000-0000-0000-000000000002", "tenant": {"id": "T1"}}`
	if _, _, err := db.ReplaceBatch("endpoints", []json.RawMessage{json.RawMessage(cleared), json.RawMessage(two)}); err == nil {
		t.Fatal("the trigger should have failed the batch")
	}
	if raw, _ := db.Get("endpoints", "00000000-0000-0000-0000-000000000001"); !strings.Contains(string(raw), "deviceEncryption") {
		t.Fatalf("a failed replace must leave the stored row: %s", raw)
	}
	if _, err := db.DB().Exec(`DROP TRIGGER fail_two`); err != nil {
		t.Fatal(err)
	}
	if _, _, err := db.ReplaceBatch("endpoints", []json.RawMessage{json.RawMessage(cleared)}); err != nil {
		t.Fatal(err)
	}
	if raw, _ := db.Get("endpoints", "00000000-0000-0000-0000-000000000001"); strings.Contains(string(raw), "deviceEncryption") {
		t.Fatalf("a replace must drop the cleared field: %s", raw)
	}
	// The merge is back for ordinary writes.
	if _, _, err := db.UpsertBatch("endpoints", []json.RawMessage{json.RawMessage(a)}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := db.UpsertBatch("endpoints", []json.RawMessage{json.RawMessage(cleared)}); err != nil {
		t.Fatal(err)
	}
	if raw, _ := db.Get("endpoints", "00000000-0000-0000-0000-000000000001"); !strings.Contains(string(raw), "deviceEncryption") {
		t.Fatalf("UpsertBatch keeps its keep-richer merge after a ReplaceBatch: %s", raw)
	}
}
