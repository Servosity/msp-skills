// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"hubspot-pp-cli/internal/store"
)

// seedOwner inserts one owner row into hubspot_owners_crm. archived=1 marks a
// deactivated owner. Shared across the deals_* novel tests.
func seedOwner(t *testing.T, s *store.Store, id string, archived int, email string) {
	t.Helper()
	data, err := json.Marshal(map[string]any{"id": id, "email": email, "archived": archived == 1})
	if err != nil {
		t.Fatalf("marshal owner %s: %v", id, err)
	}
	if _, err := s.DB().Exec(
		`INSERT INTO hubspot_owners_crm (id, data, archived, email) VALUES (?, ?, ?, ?)`,
		id, string(data), archived, email,
	); err != nil {
		t.Fatalf("insert owner %s: %v", id, err)
	}
}

func TestQueryDealsUnowned_HappyPath(t *testing.T) {
	ctx := context.Background()
	s := newNovelTestStore(t)
	now := time.Now().UTC()

	seedOwner(t, s, "100", 0, "active@example.com") // active owner
	seedOwner(t, s, "200", 1, "former@example.com") // deactivated owner

	// d1: owned by active owner -> NOT surfaced.
	seedDeal(t, s, "d1", map[string]any{
		"dealname": "Healthy", "amount": "1000",
		"dealstage": "qualifiedtobuy", "hubspot_owner_id": "100", "pipeline": "default",
	}, now)
	// d2: no owner -> unassigned.
	seedDeal(t, s, "d2", map[string]any{
		"dealname": "Orphan", "amount": "8000",
		"dealstage": "qualifiedtobuy", "pipeline": "default",
	}, now)
	// d3: owned by deactivated owner -> owner-not-found.
	seedDeal(t, s, "d3", map[string]any{
		"dealname": "Dead Rep", "amount": "5000",
		"dealstage": "appointmentscheduled", "hubspot_owner_id": "200", "pipeline": "default",
	}, now)
	// d4: owner id that doesn't exist at all -> owner-not-found.
	seedDeal(t, s, "d4", map[string]any{
		"dealname": "Ghost Owner", "amount": "3000",
		"dealstage": "qualifiedtobuy", "hubspot_owner_id": "999", "pipeline": "default",
	}, now)
	// d5: closed won -> excluded.
	seedDeal(t, s, "d5", map[string]any{
		"dealname": "Won", "amount": "9999",
		"dealstage": "closedwon", "pipeline": "default",
	}, now)

	cmd := newNovelDealsUnownedCmd(&rootFlags{})
	cmd.SetContext(ctx)
	view, err := queryDealsUnowned(cmd, s, "", 50)
	if err != nil {
		t.Fatalf("queryDealsUnowned: %v", err)
	}
	if view.Count != 3 {
		t.Fatalf("expected 3 unowned deals, got %d: %+v", view.Count, view.Deals)
	}
	// Sorted amount DESC => d2 (8000), d3 (5000), d4 (3000).
	if view.Deals[0].ID != "d2" {
		t.Errorf("expected d2 first by amount, got %q", view.Deals[0].ID)
	}
	if view.Deals[0].Reason != "unassigned" {
		t.Errorf("d2 reason expected 'unassigned', got %q", view.Deals[0].Reason)
	}
	var d3reason string
	for _, d := range view.Deals {
		if d.ID == "d3" {
			d3reason = d.Reason
		}
	}
	if d3reason != "owner-not-found" {
		t.Errorf("d3 (deactivated owner) reason expected 'owner-not-found', got %q", d3reason)
	}
	if view.TotalExposure != 16000 {
		t.Errorf("total_exposure expected 16000, got %.0f", view.TotalExposure)
	}
}

func TestQueryDealsUnowned_EmptyDB(t *testing.T) {
	ctx := context.Background()
	s := newNovelTestStore(t)

	cmd := newNovelDealsUnownedCmd(&rootFlags{})
	cmd.SetContext(ctx)
	view, err := queryDealsUnowned(cmd, s, "", 50)
	if err != nil {
		t.Fatalf("queryDealsUnowned: %v", err)
	}
	if view.Count != 0 || len(view.Deals) != 0 {
		t.Errorf("expected empty result on empty DB, got count=%d deals=%d", view.Count, len(view.Deals))
	}
	if view.TotalExposure != 0 {
		t.Errorf("total_exposure expected 0, got %.0f", view.TotalExposure)
	}
}

func TestDealsUnowned_EmptyDBJSONShape(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "empty.db")
	s, err := store.OpenWithContext(ctx, dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	s.Close()

	flags := &rootFlags{asJSON: true}
	cmd := newNovelDealsUnownedCmd(flags)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetContext(ctx)
	cmd.SetArgs([]string{"--db", dbPath})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	var view unownedView
	if err := json.Unmarshal(out.Bytes(), &view); err != nil {
		t.Fatalf("unmarshal output %q: %v", out.String(), err)
	}
	if view.Deals == nil {
		t.Errorf("deals should marshal as [] not null")
	}
}
