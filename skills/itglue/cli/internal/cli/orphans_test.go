// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Seeded-store tests for the orphaned-record detector.

package cli

import (
	"context"
	"encoding/json"
	"testing"

	"itglue-pp-cli/internal/store"
)

// seedOrphan adds a configuration whose organization-id does not exist in the
// seeded tenant (org 99 was never synced).
func seedOrphan(t *testing.T) {
	t.Helper()
	db, err := store.OpenWithContext(context.Background(), defaultDBPath("itglue-cli"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer db.Close()
	raw := `{"id":"31","type":"configurations","attributes":{"name":"GHOST-01","organization-id":99,"organization-name":"Deleted Org","updated-at":"2025-01-01T00:00:00Z"}}`
	if err := db.Upsert("configurations", "31", json.RawMessage(raw)); err != nil {
		t.Fatalf("upsert orphan: %v", err)
	}
}

func TestNovelOrphansCommand(t *testing.T) {
	seedITGStore(t)
	seedOrphan(t)
	out := runITGCmd(t, "orphans", "--json")

	var view struct {
		Orphans []map[string]any `json:"orphans"`
		Scanned int              `json:"scanned_records"`
		Orgs    int              `json:"organizations_synced"`
	}
	if err := json.Unmarshal([]byte(out), &view); err != nil {
		t.Fatalf("unmarshal: %v\noutput: %s", err, out)
	}
	if view.Orgs != 3 {
		t.Errorf("organizations_synced = %d, want 3", view.Orgs)
	}
	if len(view.Orphans) != 1 {
		t.Fatalf("expected exactly 1 orphan; got %d: %s", len(view.Orphans), out)
	}
	o := view.Orphans[0]
	if o["id"] != "31" || o["resource_type"] != "configurations" || o["organization_id"] != "99" {
		t.Errorf("orphan row wrong: %#v", o)
	}
	if o["name"] != "GHOST-01" {
		t.Errorf("orphan name = %v, want GHOST-01", o["name"])
	}
}

func TestNovelOrphansCleanTenant(t *testing.T) {
	seedITGStore(t)
	out := runITGCmd(t, "orphans", "--json")

	var view struct {
		Orphans []map[string]any `json:"orphans"`
	}
	if err := json.Unmarshal([]byte(out), &view); err != nil {
		t.Fatalf("unmarshal: %v\noutput: %s", err, out)
	}
	if len(view.Orphans) != 0 {
		t.Errorf("expected no orphans in the clean seeded tenant; got %v", view.Orphans)
	}
}

func TestNovelOrphansZeroOrgsNote(t *testing.T) {
	// Child records but zero synced organizations: everything would look
	// orphaned, so the command must report none and explain why via Note.
	home := t.TempDir()
	t.Setenv("HOME", home)
	db, err := store.OpenWithContext(context.Background(), defaultDBPath("itglue-cli"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	raw := `{"id":"50","type":"contacts","attributes":{"name":"Lone Contact","organization-id":7,"updated-at":"2025-01-01T00:00:00Z"}}`
	if err := db.Upsert("contacts", "50", json.RawMessage(raw)); err != nil {
		db.Close()
		t.Fatalf("upsert: %v", err)
	}
	db.Close()

	out := runITGCmd(t, "orphans", "--json")
	var view struct {
		Orphans []map[string]any `json:"orphans"`
		Orgs    int              `json:"organizations_synced"`
		Note    string           `json:"note"`
	}
	if err := json.Unmarshal([]byte(out), &view); err != nil {
		t.Fatalf("unmarshal: %v\noutput: %s", err, out)
	}
	if view.Orgs != 0 {
		t.Errorf("organizations_synced = %d, want 0", view.Orgs)
	}
	if len(view.Orphans) != 0 {
		t.Errorf("expected no orphan rows when zero orgs are synced; got %v", view.Orphans)
	}
	if view.Note == "" {
		t.Error("expected a note explaining the zero-orgs sync gap; got none")
	}
}

func TestNovelOrphansResourceFilter(t *testing.T) {
	seedITGStore(t)
	seedOrphan(t)
	out := runITGCmd(t, "orphans", "--resource", "contacts", "--json")

	var view struct {
		Orphans []map[string]any `json:"orphans"`
	}
	if err := json.Unmarshal([]byte(out), &view); err != nil {
		t.Fatalf("unmarshal: %v\noutput: %s", err, out)
	}
	if len(view.Orphans) != 0 {
		t.Errorf("contacts filter should exclude the configuration orphan; got %v", view.Orphans)
	}
}
