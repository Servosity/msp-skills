// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored: guards the agent-name resolution and grouping key in
// buildTriageQuery. See skills/halopsa/handfixes.json (triage-agent-name-join).

package cli

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"halopsa-pp-cli/internal/store"
)

// seedTriageFixture builds a local DB shaped like a real Halo sync: tickets
// carry agent_id but a blank agent_name and a blank datecreated, the agent
// names live in the generic resources table, and tickets.who exists and is
// NULL on every row.
func seedTriageFixture(t *testing.T) *store.Store {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	agents := map[int]string{1: "Unassigned", 3: "Dana Reyes", 13: "Sam Okafor"}
	for id, name := range agents {
		payload := fmt.Sprintf(`{"id":%d,"name":%q}`, id, name)
		if _, err := db.DB().Exec(
			`INSERT INTO resources (resource_type, id, data) VALUES ('agent', ?, ?)`,
			fmt.Sprint(id), payload,
		); err != nil {
			t.Fatalf("insert agent %d: %v", id, err)
		}
	}

	// agentID -> how many open tickets, and how many days old the oldest is.
	tickets := []struct {
		id      int
		agentID int
		ageDays int
	}{
		{id: 101, agentID: 13, ageDays: 2},
		{id: 102, agentID: 13, ageDays: 16},
		{id: 103, agentID: 3, ageDays: 167},
		{id: 104, agentID: 1, ageDays: 4},
	}
	for _, tk := range tickets {
		// Real timestamps: julianday() cannot parse relative strings, and the
		// query's age math is exactly what is under test.
		stamp := time.Now().UTC().AddDate(0, 0, -tk.ageDays).Format("2006-01-02T15:04:05")
		data := fmt.Sprintf(
			`{"id":%d,"status_id":1,"agent_id":%d,"team":"Service Desk","dateoccurred":%q,"lastactiondate":%q}`,
			tk.id, tk.agentID, stamp, stamp,
		)
		// agent_name and datecreated deliberately blank, mirroring live Halo.
		if _, err := db.DB().Exec(
			`INSERT INTO tickets (id, agent_id, agent_name, datecreated, team, data)
			 VALUES (?, ?, '', '', 'Service Desk', ?)`,
			tk.id, tk.agentID, data,
		); err != nil {
			t.Fatalf("insert ticket %d: %v", tk.id, err)
		}
	}
	return db
}

type triageRow struct {
	who        string
	open       int
	stale      int
	breaching  int
	oldestDays int
}

func runTriageQuery(t *testing.T, db *store.Store, team, agent string) []triageRow {
	t.Helper()
	q, args := buildTriageQuery(team, agent, 7, 24, 50)
	rows, err := db.DB().Query(q, args...)
	if err != nil {
		t.Fatalf("triage query: %v", err)
	}
	defer rows.Close()

	out := []triageRow{}
	for rows.Next() {
		var r triageRow
		var stale, breach, oldest sql.NullInt64
		if err := rows.Scan(&r.who, &r.open, &stale, &breach, &oldest); err != nil {
			t.Fatalf("scan: %v", err)
		}
		r.stale, r.breaching, r.oldestDays = int(stale.Int64), int(breach.Int64), int(oldest.Int64)
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
	return out
}

// TestTriage_ResolvesAgentNamesFromAgentRecords is the core regression: Halo
// never populates tickets.agent_name, so grouping on it alone labelled every
// row "(unassigned)".
func TestTriage_ResolvesAgentNamesFromAgentRecords(t *testing.T) {
	db := seedTriageFixture(t)
	got := runTriageQuery(t, db, "", "")

	byName := map[string]triageRow{}
	for _, r := range got {
		byName[r.who] = r
	}
	for _, want := range []struct {
		name string
		open int
	}{
		{"Sam Okafor", 2},
		{"Dana Reyes", 1},
		{"Unassigned", 1},
	} {
		r, ok := byName[want.name]
		if !ok {
			t.Fatalf("agent %q missing from triage output %+v; agent_name is blank in Halo, so the agent records must supply the label", want.name, got)
		}
		if r.open != want.open {
			t.Errorf("agent %q open = %d, want %d", want.name, r.open, want.open)
		}
	}
	if _, collapsed := byName["(unassigned)"]; collapsed {
		t.Errorf("output still contains a '(unassigned)' bucket: %+v", got)
	}
}

// TestTriage_DoesNotCollapseOnWhoColumn guards the subtler half: tickets has a
// real who TEXT column that is NULL on every row, and SQLite binds GROUP BY to
// source columns before output aliases. Grouping by "who" therefore produced a
// single row regardless of the SELECT expression.
func TestTriage_DoesNotCollapseOnWhoColumn(t *testing.T) {
	db := seedTriageFixture(t)

	// The collision only matters if the column really is there; assert the
	// premise so this test fails loudly rather than silently passing if the
	// generated schema ever drops it.
	var whoCols int
	if err := db.DB().QueryRow(
		`SELECT COUNT(*) FROM pragma_table_info('tickets') WHERE name = 'who'`,
	).Scan(&whoCols); err != nil {
		t.Fatalf("pragma: %v", err)
	}
	if whoCols == 0 {
		t.Skip("tickets.who no longer exists; the alias collision this guards is gone")
	}

	got := runTriageQuery(t, db, "", "")
	if len(got) != 3 {
		t.Fatalf("got %d agent rows, want 3; GROUP BY collapsed on the NULL who column: %+v", len(got), got)
	}
}

// TestTriage_OldestDaysUsesDateoccurred pins the datecreated fallback. Halo
// leaves datecreated blank, which reported every agent's oldest ticket as 0.
func TestTriage_OldestDaysUsesDateoccurred(t *testing.T) {
	db := seedTriageFixture(t)
	got := runTriageQuery(t, db, "", "")

	for _, r := range got {
		if r.oldestDays == 0 {
			t.Errorf("agent %q oldest_days = 0; datecreated is blank in Halo, so dateoccurred must be used", r.who)
		}
	}
	for _, r := range got {
		if r.who == "Dana Reyes" && r.oldestDays < 160 {
			t.Errorf("Dana Reyes oldest_days = %d, want ~167", r.oldestDays)
		}
	}
}

// TestTriage_FiltersBindInStatementOrder covers the bind ordering, which moved
// when the scope filters were pushed into the CTE ahead of the aggregate's
// own placeholders.
func TestTriage_FiltersBindInStatementOrder(t *testing.T) {
	db := seedTriageFixture(t)

	if got := runTriageQuery(t, db, "", "Dana Reyes"); len(got) != 1 || got[0].who != "Dana Reyes" {
		t.Fatalf("agent filter returned %+v, want a single Dana Reyes row", got)
	}
	if got := runTriageQuery(t, db, "", "dana reyes"); len(got) != 1 || got[0].who != "Dana Reyes" {
		t.Fatalf("agent filter is not case-insensitive: %+v", got)
	}
	// Team and agent together exercise both placeholder groups at once.
	got := runTriageQuery(t, db, "Service Desk", "Sam Okafor")
	if len(got) != 1 || got[0].who != "Sam Okafor" || got[0].open != 2 {
		t.Fatalf("team+agent filter returned %+v, want one Sam Okafor row with open=2", got)
	}
}
