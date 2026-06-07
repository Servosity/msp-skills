// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored behavioral tests for the pipeline stale novel feature.

package cli

import (
	"testing"
	"time"
)

func TestPipelineStaleFindsOldOpenOpps(t *testing.T) {
	db, path := newNovelTestDB(t)
	now := time.Now().UTC()
	seedNovelResource(t, db, rtOpportunities, "o-old", map[string]any{
		"id": "o-old", "name": "Stalled migration", "status": "OPEN",
		"ownerUser":    map[string]any{"email": "sofia@reseller.example"},
		"customerUser": map[string]any{"email": "it@acme.example"},
		"createdOn":    epochMS(now.Add(-30 * 24 * time.Hour)),
	})
	seedNovelResource(t, db, rtOpportunities, "o-fresh", map[string]any{
		"id": "o-fresh", "name": "Fresh deal", "status": "OPEN",
		"createdOn": epochMS(now.Add(-1 * 24 * time.Hour)),
	})
	// CLOSED and ancient: excluded despite its age.
	seedNovelResource(t, db, rtOpportunities, "o-closed", map[string]any{
		"id": "o-closed", "name": "Done deal", "status": "CLOSED",
		"createdOn": epochMS(now.Add(-90 * 24 * time.Hour)),
	})

	out, err := runNovelCmd(t, newNovelPipelineStaleCmd, "--days", "14", "--db", path)
	if err != nil {
		t.Fatalf("pipeline stale: %v\n%s", err, out)
	}
	view := decodeNovelJSON(t, out)
	stale := novelList(t, view, "stale")
	if len(stale) != 1 {
		t.Fatalf("stale = %d entries, want 1\n%s", len(stale), out)
	}
	entry := stale[0].(map[string]any)
	if entry["id"] != "o-old" || entry["ownerEmail"] != "sofia@reseller.example" {
		t.Fatalf("stale entry = %v, want o-old owned by sofia", entry)
	}
	if entry["ageDays"].(float64) < 29 {
		t.Fatalf("ageDays = %v, want >= 29", entry["ageDays"])
	}
	if view["total_open"].(float64) != 2 {
		t.Fatalf("total_open = %v, want 2", view["total_open"])
	}
}

func TestPipelineStaleRejectsNonPositiveDays(t *testing.T) {
	_, path := newNovelTestDB(t)
	_, err := runNovelCmd(t, newNovelPipelineStaleCmd, "--days", "0", "--db", path)
	if err == nil {
		t.Fatal("expected usage error for --days 0")
	}
}

func TestPipelineStaleEmptyHasNote(t *testing.T) {
	db, path := newNovelTestDB(t)
	seedNovelResource(t, db, rtOpportunities, "o1", map[string]any{
		"id": "o1", "status": "OPEN",
		"createdOn": epochMS(time.Now().UTC().Add(-24 * time.Hour)),
	})
	out, err := runNovelCmd(t, newNovelPipelineStaleCmd, "--days", "14", "--db", path)
	if err != nil {
		t.Fatalf("pipeline stale: %v", err)
	}
	view := decodeNovelJSON(t, out)
	if note, _ := view["note"].(string); note == "" {
		t.Fatal("expected honest empty-result note")
	}
}
