// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

// Novel feature tests: next-activity. Hand-authored against the local store.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"pipedrive-pp-cli/internal/store"
)

// openTestStore opens a fresh temp store (schema is created on open) and
// returns its path plus the *store.Store. The store is closed via t.Cleanup.
func openTestStore(t *testing.T) (string, *store.Store) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "data.db")
	db, err := store.Open(path)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return path, db
}

// nullableStr maps "" to a real NULL so tests can model both NULL and ”.
func nullableStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func seedDeal(t *testing.T, db *store.Store, id, title string, value float64, currency, status, nextActivity string, pipeline int, owner, org string) {
	t.Helper()
	_, err := db.DB().Exec(`
		INSERT INTO deals (id, data, title, value, currency, status, is_archived,
		                   next_activity_date, pipeline_id, stage_id, user_id, owner_name, org_name,
		                   last_activity_date, add_time)
		VALUES (?, '{}', ?, ?, ?, ?, 0, ?, ?, 1, '11', ?, ?, '2026-05-01 10:00:00', '2026-04-01 10:00:00')`,
		id, title, value, currency, status, nullableStr(nextActivity), pipeline, owner, org)
	if err != nil {
		t.Fatalf("seed deal %s: %v", id, err)
	}
}

func TestNovelNextActivity_QueryColdDeals(t *testing.T) {
	_, db := openTestStore(t)
	// Cold: open, no next activity (one NULL, one '').
	seedDeal(t, db, "1", "Big Cold Deal", 5000, "USD", "open", "", 1, "Alice", "Acme")
	seedDeal(t, db, "2", "Small Cold Deal", 500, "USD", "open", "", 1, "Bob", "Beta")
	// Has a next activity -> excluded.
	seedDeal(t, db, "3", "Planned Deal", 9000, "USD", "open", "2026-07-01", 1, "Carol", "Gamma")
	// Won -> excluded.
	seedDeal(t, db, "4", "Won Deal", 7000, "USD", "won", "", 1, "Dave", "Delta")
	// Different pipeline -> excluded by --pipeline 1 filter.
	seedDeal(t, db, "5", "Other Pipeline", 8000, "USD", "open", "", 2, "Eve", "Epsilon")

	res, err := queryColdDeals(context.Background(), db.DB(), 1, 25)
	if err != nil {
		t.Fatalf("queryColdDeals: %v", err)
	}
	if res.Count != 2 {
		t.Fatalf("count = %d, want 2", res.Count)
	}
	if res.TotalValueAtRisk != 5500 {
		t.Fatalf("total value at risk = %v, want 5500", res.TotalValueAtRisk)
	}
	// Ordered by value DESC.
	if res.Deals[0].ID != "1" || res.Deals[1].ID != "2" {
		t.Fatalf("order = %s,%s want 1,2", res.Deals[0].ID, res.Deals[1].ID)
	}
	if res.Deals[0].OwnerName != "Alice" || res.Deals[0].OrgName != "Acme" {
		t.Fatalf("owner/org join wrong: %+v", res.Deals[0])
	}
}

func TestNovelNextActivity_EmptyStore(t *testing.T) {
	path, _ := openTestStore(t)

	cmd := newNovelNextActivityCmd(&rootFlags{asJSON: true})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"--missing", "--db", path})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	var res coldResult
	if err := json.Unmarshal(out.Bytes(), &res); err != nil {
		t.Fatalf("invalid json: %v (raw=%s)", err, out.String())
	}
	if res.Count != 0 {
		t.Fatalf("count = %d, want 0", res.Count)
	}
	// Empty slice must marshal as [], not null.
	if !bytes.Contains(out.Bytes(), []byte(`"deals": []`)) {
		t.Fatalf("expected deals:[] in output, got %s", out.String())
	}
}

func TestNovelNextActivity_MissingRequired(t *testing.T) {
	path, _ := openTestStore(t)
	cmd := newNovelNextActivityCmd(&rootFlags{asJSON: true})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	// A non-mode flag set but no --missing -> usage error.
	cmd.SetArgs([]string{"--limit", "5", "--db", path})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected a usage error when --missing is absent")
	}
}
