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

// seedDeal inserts one deal row into hubspot_deals_crm with the given JSON
// properties and an explicit created_at. Shared across the deals_* novel tests.
func seedDeal(t *testing.T, s *store.Store, id string, props map[string]any, createdAt time.Time) {
	t.Helper()
	data, err := json.Marshal(map[string]any{"id": id, "properties": props})
	if err != nil {
		t.Fatalf("marshal deal %s: %v", id, err)
	}
	if _, err := s.DB().Exec(
		`INSERT INTO hubspot_deals_crm (id, data, archived, created_at) VALUES (?, ?, 0, ?)`,
		id, string(data), createdAt.UTC(),
	); err != nil {
		t.Fatalf("insert deal %s: %v", id, err)
	}
}

// newNovelTestStore opens a hermetic temp-dir SQLite store for the novel tests.
func newNovelTestStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.OpenWithContext(context.Background(), filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestQueryDealsVelocity_HappyPath(t *testing.T) {
	ctx := context.Background()
	s := newNovelTestStore(t)

	now := time.Now().UTC()
	seedDeal(t, s, "d1", map[string]any{
		"dealname": "Acme Renewal", "amount": "10000",
		"dealstage": "qualifiedtobuy", "pipeline": "default",
	}, now.Add(-100*24*time.Hour))
	seedDeal(t, s, "d2", map[string]any{
		"dealname": "Beta Expansion", "amount": "5000",
		"dealstage": "appointmentscheduled", "pipeline": "default",
	}, now.Add(-50*24*time.Hour))
	// closed won — must be excluded even with history present.
	seedDeal(t, s, "d3", map[string]any{
		"dealname": "Closed One", "amount": "9999",
		"dealstage": "closedwon", "pipeline": "default",
	}, now.Add(-5*24*time.Hour))

	hist := func(id string, ts time.Time) {
		if err := s.UpsertPropertyHistoryBatch(ctx, "deals", id, []store.PropertyHistoryEntry{
			{Property: "dealstage", Value: "x", Timestamp: ts},
		}); err != nil {
			t.Fatalf("seed history %s: %v", id, err)
		}
	}
	hist("d1", now.Add(-40*24*time.Hour))
	hist("d2", now.Add(-10*24*time.Hour))
	hist("d3", now.Add(-2*24*time.Hour))

	cmd := newNovelDealsVelocityCmd(&rootFlags{})
	cmd.SetContext(ctx)

	view, err := queryDealsVelocity(cmd, s, "", "", 50)
	if err != nil {
		t.Fatalf("queryDealsVelocity: %v", err)
	}
	if view.Note != "" {
		t.Fatalf("unexpected note: %q", view.Note)
	}
	if len(view.Deals) != 2 {
		t.Fatalf("expected 2 open deals, got %d: %+v", len(view.Deals), view.Deals)
	}
	if view.Deals[0].ID != "d1" {
		t.Errorf("expected d1 first (most days in stage), got %q", view.Deals[0].ID)
	}
	if view.Deals[0].DaysInStage < 38 || view.Deals[0].DaysInStage > 42 {
		t.Errorf("d1 days_in_stage ~40 expected, got %d", view.Deals[0].DaysInStage)
	}
	if len(view.StageStats) != 2 {
		t.Fatalf("expected 2 stage_stats, got %d: %+v", len(view.StageStats), view.StageStats)
	}

	viewStage, err := queryDealsVelocity(cmd, s, "", "qualifiedtobuy", 50)
	if err != nil {
		t.Fatalf("queryDealsVelocity stage filter: %v", err)
	}
	if len(viewStage.Deals) != 1 || viewStage.Deals[0].ID != "d1" {
		t.Errorf("--stage filter expected only d1, got %+v", viewStage.Deals)
	}
}

func TestQueryDealsVelocity_NoHistoryReturnsNote(t *testing.T) {
	ctx := context.Background()
	s := newNovelTestStore(t)
	seedDeal(t, s, "d1", map[string]any{
		"dealname": "No History", "amount": "1000", "dealstage": "qualifiedtobuy",
	}, time.Now().UTC())

	cmd := newNovelDealsVelocityCmd(&rootFlags{})
	cmd.SetContext(ctx)
	view, err := queryDealsVelocity(cmd, s, "", "", 50)
	if err != nil {
		t.Fatalf("queryDealsVelocity: %v", err)
	}
	if view.Note == "" {
		t.Fatalf("expected absence note, got none")
	}
	if len(view.Deals) != 0 || len(view.StageStats) != 0 {
		t.Errorf("expected empty arrays with note, got deals=%d stats=%d", len(view.Deals), len(view.StageStats))
	}
}

func TestDealsVelocity_EmptyDBJSONShape(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "empty.db")
	s, err := store.OpenWithContext(ctx, dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	s.Close()

	flags := &rootFlags{asJSON: true}
	cmd := newNovelDealsVelocityCmd(flags)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetContext(ctx)
	cmd.SetArgs([]string{"--db", dbPath})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	var view velocityView
	if err := json.Unmarshal(out.Bytes(), &view); err != nil {
		t.Fatalf("unmarshal output %q: %v", out.String(), err)
	}
	if view.Deals == nil {
		t.Errorf("deals should marshal as [] not null")
	}
	if view.Note == "" {
		t.Errorf("empty DB should carry the no-history note")
	}
}
