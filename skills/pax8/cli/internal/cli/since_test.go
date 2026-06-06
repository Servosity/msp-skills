// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written (Phase 3): behavioral tests for the since change feed against a seeded store.

package cli

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestNovelSinceCommand(t *testing.T) {
	db := seedTranscendenceStore(t)
	dbPath := db.Path()
	db.Close()

	// Wide window: every seeded subscription (createdDate 2026-01-01 .. 2026-05-27)
	// must surface as a "new" change.
	cmd := newNovelSinceCmd(&rootFlags{asJSON: true})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"520w", "--db", dbPath})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("since execute: %v", err)
	}
	var changes []subChange
	if err := json.Unmarshal(buf.Bytes(), &changes); err != nil {
		t.Fatalf("parsing since JSON: %v\n%s", err, buf.String())
	}
	if len(changes) != 3 {
		t.Errorf("expected 3 changes in wide window, got %d: %s", len(changes), buf.String())
	}
	seen := map[string]string{}
	for _, c := range changes {
		seen[c.SubscriptionID] = c.Change
		if c.Change != "new" {
			t.Errorf("subscription %s: expected change=new, got %q", c.SubscriptionID, c.Change)
		}
	}
	if seen["s-noinv"] == "" || seen["s-active"] == "" || seen["s-cancelled"] == "" {
		t.Errorf("expected s-active, s-noinv, s-cancelled in change feed, got %v", seen)
	}
	// Product name join: s-noinv is p-azure -> "Azure Plan".
	for _, c := range changes {
		if c.SubscriptionID == "s-noinv" && c.ProductName != "Azure Plan" {
			t.Errorf("s-noinv product name join: expected Azure Plan, got %q", c.ProductName)
		}
	}

	// Absence-of-correctness: a tiny window must return zero changes, not stale data.
	cmd2 := newNovelSinceCmd(&rootFlags{asJSON: true})
	var buf2 bytes.Buffer
	cmd2.SetOut(&buf2)
	cmd2.SetArgs([]string{"1h", "--db", dbPath})
	if err := cmd2.Execute(); err != nil {
		t.Fatalf("since 1h execute: %v", err)
	}
	var none []subChange
	if err := json.Unmarshal(buf2.Bytes(), &none); err != nil {
		t.Fatalf("parsing since 1h JSON: %v\n%s", err, buf2.String())
	}
	if len(none) != 0 {
		t.Errorf("expected 0 changes in 1h window, got %d: %s", len(none), buf2.String())
	}
}
