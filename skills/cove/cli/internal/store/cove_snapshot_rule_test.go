// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Codex review round 2, class "snapshot comparison scope": one rule decides
// whether two snapshots may be diffed, and an unknown scope never qualifies.
package store

import (
	"context"
	"testing"
	"time"
)

func TestCoveSnapshotsComparableNeedsTheSameKnownScope(t *testing.T) {
	ep := "POST https://api.backup.management/jsonapi"
	known := func(id, partner int64, endpoint string) CoveSnapshot {
		return CoveSnapshot{ID: id, RootPartnerID: partner, Endpoint: endpoint}
	}
	cases := []struct {
		name string
		a, b CoveSnapshot
		want bool
	}{
		{"same known scope", known(1, 7, ep), known(2, 7, ep), true},
		{"both endpoints unknown", known(1, 7, ""), known(2, 7, ""), false},
		{"one endpoint unknown", known(1, 7, ep), known(2, 7, ""), false},
		{"different endpoint", known(1, 7, ep), known(2, 7, "POST https://other.example/jsonapi"), false},
		{"different partner", known(1, 7, ep), known(2, 8, ep), false},
		{"partner unknown", known(1, 0, ep), known(2, 0, ep), false},
		{"same snapshot", known(1, 7, ep), known(1, 7, ep), false},
		// The older binding form lowercased zone ids and dropped the query, so
		// two snapshots recorded in it cannot prove they share a destination.
		{"both in the older binding form", known(1, 7, "https://api.backup.management"), known(2, 7, "https://api.backup.management"), false},
	}
	for _, c := range cases {
		if got := CoveSnapshotsComparable(c.a, c.b); got != c.want {
			t.Errorf("%s: comparable = %v, want %v", c.name, got, c.want)
		}
	}
}

// Two legacy snapshots of the same partner (endpoint unknown) are never a
// baseline for each other, even when asked for the empty endpoint.
func TestCoveSnapshotBeforeInScopeRefusesAnUnknownEndpoint(t *testing.T) {
	s := openCoveTestStore(t)
	ctx := context.Background()
	t0 := time.Unix(1792800000, 0)
	if _, err := s.InsertCoveSnapshot(ctx, t0, 7, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := s.InsertCoveSnapshot(ctx, t0.Add(time.Minute), 7, nil); err != nil {
		t.Fatal(err)
	}
	if base, ok, err := s.CoveSnapshotBeforeInScope(ctx, t0.Add(time.Hour), 7, ""); err != nil || ok {
		t.Fatalf("an unknown endpoint picked baseline %+v (ok=%v err=%v)", base, ok, err)
	}
}
