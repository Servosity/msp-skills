// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored behavioral tests for the subs changed novel feature.

package cli

import (
	"testing"
	"time"
)

func TestSubsChangedBuckets(t *testing.T) {
	db, path := newNovelTestDB(t)
	now := time.Now().UTC()

	seedNovelResource(t, db, rtCompanies, "c1", map[string]any{"uuid": "c1", "name": "Acme MSP"})

	// Created inside the 7d window.
	seedNovelResource(t, db, rtSubscriptions, "s-new", map[string]any{
		"id": "s-new", "status": "ACTIVE",
		"company":      map[string]any{"id": "c1"},
		"creationDate": epochMS(now.Add(-2 * 24 * time.Hour)),
	})
	// Ended inside the window and now cancelled.
	seedNovelResource(t, db, rtSubscriptions, "s-ended", map[string]any{
		"id": "s-ended", "status": "CANCELLED",
		"company":      map[string]any{"id": "c1"},
		"creationDate": epochMS(now.Add(-300 * 24 * time.Hour)),
		"endDate":      epochMS(now.Add(-1 * 24 * time.Hour)),
	})
	// Old ACTIVE subscription: must appear in NO bucket.
	seedNovelResource(t, db, rtSubscriptions, "s-quiet", map[string]any{
		"id": "s-quiet", "status": "ACTIVE",
		"company":      map[string]any{"id": "c1"},
		"creationDate": epochMS(now.Add(-300 * 24 * time.Hour)),
	})
	// Cancelled long ago: endDate far outside the window, excluded.
	seedNovelResource(t, db, rtSubscriptions, "s-ancient", map[string]any{
		"id": "s-ancient", "status": "CANCELLED",
		"company":      map[string]any{"id": "c1"},
		"creationDate": epochMS(now.Add(-400 * 24 * time.Hour)),
		"endDate":      epochMS(now.Add(-200 * 24 * time.Hour)),
	})

	out, err := runNovelCmd(t, newNovelSubsChangedCmd, "--since", "7d", "--db", path)
	if err != nil {
		t.Fatalf("subs changed: %v\n%s", err, out)
	}
	view := decodeNovelJSON(t, out)

	ids := func(key string) map[string]bool {
		set := map[string]bool{}
		for _, e := range novelList(t, view, key) {
			set[e.(map[string]any)["subscriptionId"].(string)] = true
		}
		return set
	}
	created, ended, inactive := ids("created"), ids("ended"), ids("inactive")

	if !created["s-new"] || len(created) != 1 {
		t.Fatalf("created = %v, want exactly {s-new}\n%s", created, out)
	}
	if !ended["s-ended"] || len(ended) != 1 {
		t.Fatalf("ended = %v, want exactly {s-ended}\n%s", ended, out)
	}
	if !inactive["s-ended"] || inactive["s-ancient"] {
		t.Fatalf("inactive = %v, want s-ended without s-ancient\n%s", inactive, out)
	}
	if created["s-quiet"] || ended["s-quiet"] || inactive["s-quiet"] {
		t.Fatalf("quiet ACTIVE sub leaked into a bucket: %s", out)
	}
	if view["scanned_subscriptions"].(float64) != 4 {
		t.Fatalf("scanned_subscriptions = %v, want 4", view["scanned_subscriptions"])
	}
}

func TestSubsChangedFutureCancellationIsInactive(t *testing.T) {
	db, path := newNovelTestDB(t)
	now := time.Now().UTC()
	// Cancelled today, effective at period end next month: churn signal NOW.
	seedNovelResource(t, db, rtSubscriptions, "s-future", map[string]any{
		"id": "s-future", "status": "CANCELLED",
		"creationDate": epochMS(now.Add(-100 * 24 * time.Hour)),
		"endDate":      epochMS(now.Add(30 * 24 * time.Hour)),
	})
	out, err := runNovelCmd(t, newNovelSubsChangedCmd, "--since", "7d", "--db", path)
	if err != nil {
		t.Fatalf("subs changed: %v", err)
	}
	view := decodeNovelJSON(t, out)
	inactive := novelList(t, view, "inactive")
	if len(inactive) != 1 || inactive[0].(map[string]any)["subscriptionId"] != "s-future" {
		t.Fatalf("future-effective cancellation missing from inactive: %s", out)
	}
	if len(novelList(t, view, "ended")) != 0 {
		t.Fatalf("future endDate must not count as ended: %s", out)
	}
}

func TestSubsChangedEmptyWindowHasNote(t *testing.T) {
	db, path := newNovelTestDB(t)
	seedNovelResource(t, db, rtSubscriptions, "s1", map[string]any{
		"id": "s1", "status": "ACTIVE",
		"creationDate": epochMS(time.Now().UTC().Add(-300 * 24 * time.Hour)),
	})
	out, err := runNovelCmd(t, newNovelSubsChangedCmd, "--since", "7d", "--db", path)
	if err != nil {
		t.Fatalf("subs changed: %v", err)
	}
	view := decodeNovelJSON(t, out)
	if note, _ := view["note"].(string); note == "" {
		t.Fatal("expected honest empty-result note")
	}
}
