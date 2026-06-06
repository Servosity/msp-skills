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

// createAssociations creates the hubspot_associations table (not part of the
// generated migrations) so association-joining novel commands have something to
// query. Mirrors the columns engagements_of.go / nurture_mine.go read.
func createAssociations(t *testing.T, s *store.Store) {
	t.Helper()
	if _, err := s.DB().Exec(`CREATE TABLE IF NOT EXISTS hubspot_associations (
		from_type TEXT NOT NULL,
		from_id   TEXT NOT NULL,
		to_type   TEXT NOT NULL,
		to_id     TEXT NOT NULL
	)`); err != nil {
		t.Fatalf("create hubspot_associations: %v", err)
	}
}

func seedAssociation(t *testing.T, s *store.Store, fromType, fromID, toType, toID string) {
	t.Helper()
	if _, err := s.DB().Exec(
		`INSERT INTO hubspot_associations (from_type, from_id, to_type, to_id) VALUES (?, ?, ?, ?)`,
		fromType, fromID, toType, toID,
	); err != nil {
		t.Fatalf("insert association: %v", err)
	}
}

func TestQueryContactsWinBack_HappyPath(t *testing.T) {
	ctx := context.Background()
	s := newNovelTestStore(t)
	createAssociations(t, s)
	now := time.Now().UTC()

	// c1: on a closed-won deal, cold 120 days -> surfaced.
	seedContact(t, s, "c1", map[string]any{
		"firstname": "Cold", "lastname": "Customer", "email": "cold@example.com",
		"hubspot_owner_id":     "100",
		"notes_last_contacted": now.Add(-120 * 24 * time.Hour).Format(time.RFC3339),
	}, now.Add(-300*24*time.Hour))
	// c2: on a closed-won deal but recently contacted -> below cold-days, excluded.
	seedContact(t, s, "c2", map[string]any{
		"firstname": "Warm", "lastname": "Customer", "email": "warm@example.com",
		"notes_last_contacted": now.Add(-5 * 24 * time.Hour).Format(time.RFC3339),
	}, now.Add(-300*24*time.Hour))
	// c3: cold but only on an OPEN deal -> excluded (not closed won).
	seedContact(t, s, "c3", map[string]any{
		"firstname": "Open", "lastname": "Prospect", "email": "open@example.com",
		"notes_last_contacted": now.Add(-200 * 24 * time.Hour).Format(time.RFC3339),
	}, now.Add(-300*24*time.Hour))

	seedDeal(t, s, "won1", map[string]any{
		"dealname": "Acme Won", "amount": "25000", "dealstage": "closedwon",
	}, now.Add(-200*24*time.Hour))
	seedDeal(t, s, "won2", map[string]any{
		"dealname": "Beta Won", "amount": "9000", "dealstage": "closedwon",
	}, now.Add(-200*24*time.Hour))
	seedDeal(t, s, "open1", map[string]any{
		"dealname": "Gamma Open", "amount": "4000", "dealstage": "qualifiedtobuy",
	}, now.Add(-200*24*time.Hour))

	seedAssociation(t, s, "contacts", "c1", "deals", "won1")
	seedAssociation(t, s, "contacts", "c2", "deals", "won2")
	seedAssociation(t, s, "contacts", "c3", "deals", "open1")

	cmd := newNovelContactsWinBackCmd(&rootFlags{})
	cmd.SetContext(ctx)
	items, err := queryContactsWinBack(cmd, s, "", 90, 50)
	if err != nil {
		t.Fatalf("queryContactsWinBack: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected exactly 1 win-back row (c1), got %d: %+v", len(items), items)
	}
	got := items[0]
	if got.ID != "c1" {
		t.Errorf("expected c1, got %q", got.ID)
	}
	if got.WonDealID != "won1" || got.WonDealName != "Acme Won" {
		t.Errorf("won deal join wrong: %+v", got)
	}
	if got.WonAmount != 25000 {
		t.Errorf("won_amount expected 25000, got %.0f", got.WonAmount)
	}
	if got.IdleDays < 118 || got.IdleDays > 122 {
		t.Errorf("idle_days ~120 expected, got %d", got.IdleDays)
	}

	// owner filter that matches.
	itemsOwner, err := queryContactsWinBack(cmd, s, "100", 90, 50)
	if err != nil {
		t.Fatalf("queryContactsWinBack owner: %v", err)
	}
	if len(itemsOwner) != 1 {
		t.Errorf("owner filter expected 1 row, got %d", len(itemsOwner))
	}
}

func TestQueryContactsWinBack_NoAssociationsReturnsEmpty(t *testing.T) {
	ctx := context.Background()
	s := newNovelTestStore(t)
	// No associations table at all — the command must tolerate its absence and
	// return an empty list rather than erroring.
	seedContact(t, s, "c1", map[string]any{
		"email":                "x@example.com",
		"notes_last_contacted": time.Now().Add(-200 * 24 * time.Hour).UTC().Format(time.RFC3339),
	}, time.Now().UTC())

	cmd := newNovelContactsWinBackCmd(&rootFlags{})
	cmd.SetContext(ctx)
	items, err := queryContactsWinBack(cmd, s, "", 90, 50)
	if err != nil {
		t.Fatalf("queryContactsWinBack: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("expected empty result with no associations, got %d", len(items))
	}
}

func TestContactsWinBack_EmptyDBJSONShape(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "empty.db")
	s, err := store.OpenWithContext(ctx, dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	s.Close()

	flags := &rootFlags{asJSON: true}
	cmd := newNovelContactsWinBackCmd(flags)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetContext(ctx)
	cmd.SetArgs([]string{"--db", dbPath})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	var items []winBackRow
	if err := json.Unmarshal(out.Bytes(), &items); err != nil {
		t.Fatalf("unmarshal output %q: %v", out.String(), err)
	}
	if items == nil {
		t.Errorf("win-back should marshal as [] not null")
	}
}
