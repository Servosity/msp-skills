// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored: guards the actions parent-key entry in
// resourceParentKeyColumns. See skills/halopsa/handfixes.json
// (actions-ticket-id-parent-key).

package store

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	db, err := Open(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func actionPayload(ticketID, localID int) json.RawMessage {
	return json.RawMessage(fmt.Sprintf(
		`{"id":%d,"ticket_id":%d,"note":"ticket %d action %d"}`,
		localID, ticketID, ticketID, localID,
	))
}

// TestActionsKeyOnTicketAndID is the core regression. Halo numbers actions per
// ticket, so every ticket has an action 1. Keying on the bare id let action 1
// of each ticket overwrite the previous one, which collapsed 8099 synced
// actions into 149 stored rows on a real instance.
func TestActionsKeyOnTicketAndID(t *testing.T) {
	db := openTestStore(t)

	// Three tickets that each have an action 1 and an action 2.
	for _, ticketID := range []int{13, 14, 2123} {
		for _, localID := range []int{1, 2} {
			if err := db.UpsertActions(actionPayload(ticketID, localID)); err != nil {
				t.Fatalf("upsert action %d/%d: %v", ticketID, localID, err)
			}
		}
	}

	var rows int
	if err := db.DB().QueryRow(`SELECT COUNT(*) FROM actions`).Scan(&rows); err != nil {
		t.Fatalf("count actions: %v", err)
	}
	if rows != 6 {
		t.Fatalf("stored %d actions, want 6; per-ticket action numbering collapsed onto the bare id", rows)
	}

	// Every ticket must keep both of its actions.
	for _, ticketID := range []int{13, 14, 2123} {
		var perTicket int
		if err := db.DB().QueryRow(
			`SELECT COUNT(*) FROM actions WHERE json_extract(data, '$.ticket_id') = ?`,
			ticketID,
		).Scan(&perTicket); err != nil {
			t.Fatalf("count for ticket %d: %v", ticketID, err)
		}
		if perTicket != 2 {
			t.Errorf("ticket %d kept %d actions, want 2", ticketID, perTicket)
		}
	}
}

// TestActionsUpsertIsStillIdempotent guards the other direction: re-syncing the
// same action must update in place rather than accumulate duplicates.
func TestActionsUpsertIsStillIdempotent(t *testing.T) {
	db := openTestStore(t)

	for i := 0; i < 3; i++ {
		if err := db.UpsertActions(actionPayload(14, 1)); err != nil {
			t.Fatalf("upsert: %v", err)
		}
	}
	var rows int
	if err := db.DB().QueryRow(`SELECT COUNT(*) FROM actions`).Scan(&rows); err != nil {
		t.Fatalf("count: %v", err)
	}
	if rows != 1 {
		t.Fatalf("stored %d rows for one repeated action, want 1", rows)
	}
}

// TestActionsStorageIDCarriesParent pins the mapping itself, so a reprint that
// drops the entry fails here even if the upsert paths change shape.
func TestActionsStorageIDCarriesParent(t *testing.T) {
	if got := resourceParentKeyColumns["actions"]; got != "ticket_id" {
		t.Fatalf("resourceParentKeyColumns[actions] = %q, want ticket_id", got)
	}
	obj := map[string]any{"id": 1, "ticket_id": 14}
	bare := resourceStorageID("other", "1", obj)
	composed := resourceStorageID("actions", "1", obj)
	if composed == bare {
		t.Fatalf("actions storage id %q is not parent-qualified", composed)
	}
	// Two different parents must not share a storage key.
	other := resourceStorageID("actions", "1", map[string]any{"id": 1, "ticket_id": 13})
	if composed == other {
		t.Fatalf("actions on tickets 14 and 13 share storage id %q", composed)
	}
}

// TestActionsWithoutTicketIDFallsBack covers the guard in resourceStorageID:
// an action lacking a parent reference must still store rather than be dropped
// or keyed on an empty parent.
func TestActionsWithoutTicketIDFallsBack(t *testing.T) {
	db := openTestStore(t)

	if err := db.UpsertActions(json.RawMessage(`{"id":77,"note":"no parent"}`)); err != nil {
		t.Fatalf("upsert parentless action: %v", err)
	}
	var rows int
	if err := db.DB().QueryRow(`SELECT COUNT(*) FROM actions`).Scan(&rows); err != nil {
		t.Fatalf("count: %v", err)
	}
	if rows != 1 {
		t.Fatalf("stored %d rows, want 1; a parentless action must still persist", rows)
	}
}
