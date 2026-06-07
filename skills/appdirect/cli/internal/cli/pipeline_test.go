// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored behavioral tests for the pipeline novel feature.

package cli

import (
	"testing"
	"time"
)

func seedPipelineOpps(t *testing.T) (string, time.Time) {
	t.Helper()
	db, path := newNovelTestDB(t)
	now := time.Now().UTC()
	seedNovelResource(t, db, rtOpportunities, "o1", map[string]any{
		"id": "o1", "name": "Migrate Acme to M365", "status": "OPEN",
		"ownerUser": map[string]any{"id": "u1", "email": "sofia@reseller.example"},
		"createdOn": epochMS(now.Add(-30 * 24 * time.Hour)),
	})
	seedNovelResource(t, db, rtOpportunities, "o2", map[string]any{
		"id": "o2", "name": "Globex security bundle", "status": "OPEN",
		"ownerUser": map[string]any{"id": "u1", "email": "sofia@reseller.example"},
		"createdOn": epochMS(now.Add(-1 * 24 * time.Hour)),
	})
	seedNovelResource(t, db, rtOpportunities, "o3", map[string]any{
		"id": "o3", "name": "Closed deal", "status": "CLOSED",
		"ownerUser": map[string]any{"id": "u2", "email": "max@reseller.example"},
		"createdOn": epochMS(now.Add(-60 * 24 * time.Hour)),
	})
	return path, now
}

func TestPipelineGroupByStatus(t *testing.T) {
	path, _ := seedPipelineOpps(t)
	out, err := runNovelCmd(t, newNovelPipelineCmd, "--group-by", "status", "--db", path)
	if err != nil {
		t.Fatalf("pipeline: %v\n%s", err, out)
	}
	view := decodeNovelJSON(t, out)
	groups := novelList(t, view, "groups")
	if len(groups) != 2 {
		t.Fatalf("groups = %d, want 2 (OPEN, CLOSED)\n%s", len(groups), out)
	}
	first := groups[0].(map[string]any)
	if first["key"] != "OPEN" || first["count"].(float64) != 2 || first["openCount"].(float64) != 2 {
		t.Fatalf("first group = %v, want OPEN count=2 openCount=2", first)
	}
	if view["total"].(float64) != 3 {
		t.Fatalf("total = %v, want 3", view["total"])
	}
}

func TestPipelineGroupByOwner(t *testing.T) {
	path, _ := seedPipelineOpps(t)
	out, err := runNovelCmd(t, newNovelPipelineCmd, "--group-by", "owner", "--db", path)
	if err != nil {
		t.Fatalf("pipeline --group-by owner: %v", err)
	}
	view := decodeNovelJSON(t, out)
	groups := novelList(t, view, "groups")
	keys := map[string]float64{}
	for _, g := range groups {
		gm := g.(map[string]any)
		keys[gm["key"].(string)] = gm["count"].(float64)
	}
	if keys["sofia@reseller.example"] != 2 || keys["max@reseller.example"] != 1 {
		t.Fatalf("owner grouping = %v, want sofia=2 max=1", keys)
	}
}

func TestPipelineRejectsUnknownGroupBy(t *testing.T) {
	path, _ := seedPipelineOpps(t)
	_, err := runNovelCmd(t, newNovelPipelineCmd, "--group-by", "stage", "--db", path)
	if err == nil {
		t.Fatal("expected usage error for --group-by stage")
	}
}
