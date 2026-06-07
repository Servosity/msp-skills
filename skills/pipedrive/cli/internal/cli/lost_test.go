// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

// Novel feature tests: lost. Hand-authored against the local store.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"pipedrive-pp-cli/internal/store"
)

func seedLostDeal(t *testing.T, db *store.Store, id, title string, value float64, lostTime, reason, person, org, owner string) {
	t.Helper()
	_, err := db.DB().Exec(`
		INSERT INTO deals (id, data, title, value, currency, status, is_archived,
		                   lost_time, lost_reason, person_id, person_name, org_id, org_name, user_id, owner_name)
		VALUES (?, '{}', ?, ?, 'USD', 'lost', 0, ?, ?, '7', ?, '3', ?, '11', ?)`,
		id, title, value, lostTime, reason, person, org, owner)
	if err != nil {
		t.Fatalf("seed lost deal %s: %v", id, err)
	}
}

func TestNovelLost_QueryLostDeals(t *testing.T) {
	_, db := openTestStore(t)
	// In-window lost deals (cutoff 2026-05-20).
	seedLostDeal(t, db, "1", "Lost Recently", 3000, "2026-06-01 09:00:00", "Budget cut", "Jane Doe", "Acme", "Alice")
	seedLostDeal(t, db, "2", "Lost Earlier", 1000, "2026-05-25 09:00:00", "Went with competitor", "John Roe", "Beta", "Bob")
	// Outside window.
	seedLostDeal(t, db, "3", "Lost Long Ago", 9000, "2026-01-01 09:00:00", "No budget", "Old Lead", "Gamma", "Carol")
	// Open deal -> excluded.
	seedDeal(t, db, "4", "Still Open", 8000, "USD", "open", "", 1, "Dave", "Delta")

	cutoff := "2026-05-20 00:00:00"
	res, err := queryLostDeals(context.Background(), db.DB(), cutoff, "", 50)
	if err != nil {
		t.Fatalf("queryLostDeals: %v", err)
	}
	if res.Count != 2 {
		t.Fatalf("count = %d, want 2", res.Count)
	}
	if res.TotalLostValue != 4000 {
		t.Fatalf("total lost value = %v, want 4000", res.TotalLostValue)
	}
	// Ordered lost_time DESC.
	if res.Deals[0].ID != "1" || res.Deals[1].ID != "2" {
		t.Fatalf("order = %s,%s want 1,2", res.Deals[0].ID, res.Deals[1].ID)
	}
	if res.Deals[0].PersonName != "Jane Doe" || res.Deals[0].OrgName != "Acme" || res.Deals[0].OwnerName != "Alice" {
		t.Fatalf("join wrong: %+v", res.Deals[0])
	}

	// Reason filter (case-insensitive substring).
	filtered, err := queryLostDeals(context.Background(), db.DB(), cutoff, "competitor", 50)
	if err != nil {
		t.Fatalf("queryLostDeals reason: %v", err)
	}
	if filtered.Count != 1 || filtered.Deals[0].ID != "2" {
		t.Fatalf("reason filter = %d deals, want 1 (id 2)", filtered.Count)
	}
}

func TestNovelLost_EmptyStore(t *testing.T) {
	path, _ := openTestStore(t)

	cmd := newNovelLostCmd(&rootFlags{asJSON: true})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"--since", "90d", "--db", path})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	var res lostResult
	if err := json.Unmarshal(out.Bytes(), &res); err != nil {
		t.Fatalf("invalid json: %v (raw=%s)", err, out.String())
	}
	if res.Count != 0 {
		t.Fatalf("count = %d, want 0", res.Count)
	}
	if res.Since != "90d" {
		t.Fatalf("since = %q, want 90d", res.Since)
	}
	if !bytes.Contains(out.Bytes(), []byte(`"deals": []`)) {
		t.Fatalf("expected deals:[] in output, got %s", out.String())
	}
}

// Regression: a date-only lost_time on the cutoff's calendar day must be
// included (the WHERE compares at date() granularity, not lexically).
func TestNovelLost_DateOnlyCutoffBoundary(t *testing.T) {
	_, db := openTestStore(t)
	seedLostDeal(t, db, "10", "Date-Only Boundary", 500, "2026-05-20", "Budget", "Pat Lee", "Edge", "Erin")

	res, err := queryLostDeals(context.Background(), db.DB(), "2026-05-20 00:00:00", "", 50)
	if err != nil {
		t.Fatalf("queryLostDeals: %v", err)
	}
	if res.Count != 1 || len(res.Deals) != 1 || res.Deals[0].ID != "10" {
		t.Fatalf("date-only lost_time on cutoff day dropped: %+v", res)
	}

	// Day before the cutoff stays excluded.
	res, err = queryLostDeals(context.Background(), db.DB(), "2026-05-21 00:00:00", "", 50)
	if err != nil {
		t.Fatalf("queryLostDeals: %v", err)
	}
	if res.Count != 0 {
		t.Fatalf("day-before-cutoff deal leaked in: %+v", res)
	}
}
