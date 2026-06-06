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

// seedContact inserts one contact row into hubspot_contacts_crm. Shared across
// the contacts_* novel tests.
func seedContact(t *testing.T, s *store.Store, id string, props map[string]any, createdAt time.Time) {
	t.Helper()
	data, err := json.Marshal(map[string]any{"id": id, "properties": props})
	if err != nil {
		t.Fatalf("marshal contact %s: %v", id, err)
	}
	if _, err := s.DB().Exec(
		`INSERT INTO hubspot_contacts_crm (id, data, archived, created_at) VALUES (?, ?, 0, ?)`,
		id, string(data), createdAt.UTC(),
	); err != nil {
		t.Fatalf("insert contact %s: %v", id, err)
	}
}

func stageCount(view funnelView, name string) (int64, bool) {
	for _, s := range view.Stages {
		if s.Stage == name {
			return s.Count, true
		}
	}
	return 0, false
}

func TestQueryContactsFunnel_HappyPath(t *testing.T) {
	ctx := context.Background()
	s := newNovelTestStore(t)
	now := time.Now().UTC()

	// 4 leads, 2 MQLs, 1 customer, 1 weird non-canonical, 1 unset.
	for i, st := range []string{
		"lead", "lead", "lead", "lead",
		"marketingqualifiedlead", "marketingqualifiedlead",
		"customer",
	} {
		seedContact(t, s, "c"+string(rune('a'+i)), map[string]any{"lifecyclestage": st}, now)
	}
	seedContact(t, s, "weird", map[string]any{"lifecyclestage": "partner"}, now)
	seedContact(t, s, "blank", map[string]any{"firstname": "No"}, now) // no lifecyclestage

	cmd := newNovelContactsFunnelCmd(&rootFlags{})
	cmd.SetContext(ctx)
	view, err := queryContactsFunnel(cmd, s, "")
	if err != nil {
		t.Fatalf("queryContactsFunnel: %v", err)
	}
	if view.Total != 9 {
		t.Errorf("total expected 9, got %d", view.Total)
	}
	if c, _ := stageCount(view, "lead"); c != 4 {
		t.Errorf("lead count expected 4, got %d", c)
	}
	if c, _ := stageCount(view, "marketingqualifiedlead"); c != 2 {
		t.Errorf("MQL count expected 2, got %d", c)
	}
	if c, ok := stageCount(view, "partner"); !ok || c != 1 {
		t.Errorf("non-canonical 'partner' bucket expected count 1, got %d ok=%v", c, ok)
	}
	if c, ok := stageCount(view, "(unset)"); !ok || c != 1 {
		t.Errorf("(unset) bucket expected count 1, got %d ok=%v", c, ok)
	}

	// conversion_pct on MQL = MQL / lead * 100 = 2/4 = 50.0
	var mqlPct *float64
	for _, st := range view.Stages {
		if st.Stage == "marketingqualifiedlead" {
			mqlPct = st.ConversionPct
		}
	}
	if mqlPct == nil {
		t.Fatalf("expected conversion_pct on MQL")
	}
	if *mqlPct != 50.0 {
		t.Errorf("MQL conversion_pct expected 50.0, got %.1f", *mqlPct)
	}

	// Canonical block stays in the documented order, first.
	wantOrder := canonicalLifecycleStages
	for i, want := range wantOrder {
		if view.Stages[i].Stage != want {
			t.Errorf("canonical order[%d] expected %q, got %q", i, want, view.Stages[i].Stage)
		}
	}
}

func TestQueryContactsFunnel_EmptyDBZeroCanonicalStages(t *testing.T) {
	ctx := context.Background()
	s := newNovelTestStore(t)

	cmd := newNovelContactsFunnelCmd(&rootFlags{})
	cmd.SetContext(ctx)
	view, err := queryContactsFunnel(cmd, s, "")
	if err != nil {
		t.Fatalf("queryContactsFunnel: %v", err)
	}
	if view.Total != 0 {
		t.Errorf("expected total 0 on empty DB, got %d", view.Total)
	}
	if len(view.Stages) != len(canonicalLifecycleStages) {
		t.Fatalf("expected exactly the %d canonical zero-count stages, got %d",
			len(canonicalLifecycleStages), len(view.Stages))
	}
	for _, st := range view.Stages {
		if st.Count != 0 {
			t.Errorf("stage %q expected zero count on empty DB, got %d", st.Stage, st.Count)
		}
	}
}

func TestContactsFunnel_EmptyDBJSONShape(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "empty.db")
	s, err := store.OpenWithContext(ctx, dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	s.Close()

	flags := &rootFlags{asJSON: true}
	cmd := newNovelContactsFunnelCmd(flags)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetContext(ctx)
	cmd.SetArgs([]string{"--db", dbPath})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	var view funnelView
	if err := json.Unmarshal(out.Bytes(), &view); err != nil {
		t.Fatalf("unmarshal output %q: %v", out.String(), err)
	}
	if view.Stages == nil {
		t.Errorf("stages should marshal as [] not null")
	}
}
