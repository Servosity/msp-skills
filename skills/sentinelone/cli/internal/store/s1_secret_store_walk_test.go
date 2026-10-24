// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): the old-store scrub walks every table that
// can hold data, discovered at run time, so a table added later is covered.

package store

import (
	"context"
	"path/filepath"
	"testing"
)

// TestScrubCoversEveryDataBearingTable derives the scrub's table walk from
// the same discovery function the scrub runs (secretScrubPlan) and checks it
// against sqlite_master: every table is walked, except SQLite's own
// bookkeeping and the search index with its shadow tables (rebuilt from the
// scrubbed resources rows instead). A table created after the scrub was
// written is walked too, so a future table cannot be missed.
func TestScrubCoversEveryDataBearingTable(t *testing.T) {
	ctx := context.Background()
	s, err := Open(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.DB().Exec(`CREATE TABLE future_feature (id INTEGER PRIMARY KEY, payload TEXT)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB().Exec(`CREATE TABLE future_kv (k TEXT PRIMARY KEY, v TEXT) WITHOUT ROWID`); err != nil {
		t.Fatal(err)
	}
	conn, err := s.DB().Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	walk, skipped, err := secretScrubPlan(ctx, conn)
	if err != nil {
		t.Fatal(err)
	}

	walked := map[string]secretScrubTable{}
	for _, tb := range walk {
		walked[tb.name] = tb
	}
	allowedSkip := map[string]bool{
		"sqlite_sequence": true, "resources_fts": true, "resources_fts_config": true, "resources_fts_content": true,
		"resources_fts_data": true, "resources_fts_docsize": true, "resources_fts_idx": true,
	}
	isSkipped := map[string]bool{}
	for _, name := range skipped {
		isSkipped[name] = true
		if !allowedSkip[name] {
			t.Errorf("the scrub skips %q, a table that can hold data", name)
		}
	}

	rows, err := conn.QueryContext(ctx, `SELECT name FROM sqlite_master WHERE type = 'table'`)
	if err != nil {
		t.Fatal(err)
	}
	var all []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		all = append(all, name)
	}
	_ = rows.Close()
	for _, name := range all {
		_, w := walked[name]
		switch {
		case w && isSkipped[name]:
			t.Errorf("table %q is both walked and skipped", name)
		case !w && !isSkipped[name]:
			t.Errorf("table %q is neither walked nor deliberately skipped", name)
		}
	}
	if len(walk)+len(skipped) != len(all) {
		t.Errorf("walk (%d) + skipped (%d) != tables in sqlite_master (%d)", len(walk), len(skipped), len(all))
	}

	want := []string{"resources", "sync_state", "fleet_snapshots", "fleet_walk_rows", "fleet_capture_log",
		"learn_candidates", "learn_events", "search_learnings", "search_patterns", "learning_playbooks",
		"entity_lookups", "future_feature", "future_kv"}
	for _, table := range typedListTableByResource {
		want = append(want, table)
	}
	for _, name := range want {
		if _, ok := walked[name]; !ok {
			t.Errorf("the scrub does not walk %q", name)
		}
	}
	if tb := walked["future_kv"]; !tb.withoutRowid {
		t.Errorf("future_kv is a WITHOUT ROWID table; the walk must key it by its primary key")
	}
	hasData := false
	for _, c := range walked["actions"].columns {
		if c.name == "bare_id" {
			t.Errorf("the walk lists the generated column actions.bare_id, which cannot be written")
		}
		if c.name == "data" {
			hasData = true
		}
	}
	if !hasData {
		t.Errorf("the walk of actions does not include its data column")
	}
}
