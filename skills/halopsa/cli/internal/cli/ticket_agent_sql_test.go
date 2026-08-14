// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored: guards the shared ticket-agent SQL fragment and its use by the
// novel commands named in issue #211. See skills/halopsa/handfixes.json
// (novel-commands-share-ticket-agent-sql).

package cli

import (
	"database/sql"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// scopedFixture mirrors a real Halo sync: agent_name and datecreated blank on
// every ticket, the agent names living in resources, and a real but NULL `who`
// column on tickets. Adds client_name and summary so the client-facing
// projections can be exercised too.
func scopedFixture(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "scoped.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	mustExec(t, db, `CREATE TABLE tickets (
		id INTEGER PRIMARY KEY,
		data JSON NOT NULL,
		agent_id INTEGER,
		agent_name TEXT,
		datecreated TEXT,
		client_name TEXT,
		summary TEXT,
		team TEXT,
		who TEXT
	)`)
	mustExec(t, db, `CREATE TABLE resources (
		id TEXT NOT NULL,
		resource_type TEXT NOT NULL,
		data JSON NOT NULL,
		PRIMARY KEY (resource_type, id)
	)`)
	mustExec(t, db, `INSERT INTO resources (id, resource_type, data) VALUES
		('11','agent','{"id":11,"name":"Ada"}'),
		('22','agent','{"id":22,"name":"Grace"}')`)
	// Two closed tickets for Ada, one for Grace, one with no agent at all.
	mustExec(t, db, `INSERT INTO tickets (id, data, agent_id, agent_name, datecreated, client_name, summary, team, who) VALUES
		(1,'{"status_id":8,"dateoccurred":"2020-01-01T00:00:00Z","lastactiondate":"2020-02-01T00:00:00Z"}',11,'','','Acme','a',NULL,NULL),
		(2,'{"status_id":9,"dateoccurred":"2020-06-01T00:00:00Z","lastactiondate":"2020-07-01T00:00:00Z"}',11,'','','Acme','b',NULL,NULL),
		(3,'{"status_id":8,"dateoccurred":"2021-01-01T00:00:00Z","lastactiondate":"2021-02-01T00:00:00Z"}',22,'','','Beta','c',NULL,NULL),
		(4,'{"status_id":8,"dateoccurred":"2021-03-01T00:00:00Z","lastactiondate":"2021-04-01T00:00:00Z"}',99,'','','Beta','d',NULL,NULL)`)
	return db
}

// TestScopedCTEResolvesAgentsAndDoesNotCollapse is the core regression for the
// family: grouping on agent_label must produce one row per agent, where the
// pre-fix "AS who ... GROUP BY who" collapsed everything into a single bucket
// because tickets.who is a real NULL column.
func TestScopedCTEResolvesAgentsAndDoesNotCollapse(t *testing.T) {
	db := scopedFixture(t)

	rows, err := db.Query(ticketAgentScopedCTE + `
		SELECT agent_label, COUNT(*) FROM scoped
		WHERE json_extract(data,'$.status_id') IN (8,9)
		GROUP BY agent_label ORDER BY agent_label`)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer rows.Close()

	got := map[string]int{}
	for rows.Next() {
		var label string
		var n int
		if err := rows.Scan(&label, &n); err != nil {
			t.Fatalf("scan: %v", err)
		}
		got[label] = n
	}
	want := map[string]int{"Ada": 2, "Grace": 1, "(unassigned)": 1}
	if len(got) != len(want) {
		t.Fatalf("got %d groups (%v), want %d; the grouping key collapsed", len(got), got, len(want))
	}
	for label, n := range want {
		if got[label] != n {
			t.Errorf("%s = %d, want %d", label, got[label], n)
		}
	}
}

// TestScopedCTEFallsThroughToDateoccurred pins the created_at fallback, which
// is what reported every age as 0 while datecreated is blank.
func TestScopedCTEFallsThroughToDateoccurred(t *testing.T) {
	db := scopedFixture(t)
	var oldest int
	if err := db.QueryRow(ticketAgentScopedCTE + `
		SELECT CAST(MAX(julianday('now') - julianday(created_at)) AS INTEGER) FROM scoped`).Scan(&oldest); err != nil {
		t.Fatalf("query: %v", err)
	}
	if oldest <= 0 {
		t.Fatalf("oldest age = %d, want > 0; datecreated is blank so created_at must use dateoccurred", oldest)
	}
}

// TestScopedCTEPrefersPopulatedAgentName keeps the fallback ordering safe, so a
// future Halo version (or an includeagent sync) that does populate agent_name
// still wins over the join.
func TestScopedCTEPrefersPopulatedAgentName(t *testing.T) {
	db := scopedFixture(t)
	mustExec(t, db, `UPDATE tickets SET agent_name = 'From Payload' WHERE id = 1`)

	var label string
	if err := db.QueryRow(ticketAgentScopedCTE + `SELECT agent_label FROM scoped WHERE id = 1`).Scan(&label); err != nil {
		t.Fatalf("query: %v", err)
	}
	if label != "From Payload" {
		t.Fatalf("agent_label = %q, want the populated agent_name to win", label)
	}
}

// TestScopedCTEAliasesCannotCollide guards the trap that caused the original
// bug: neither derived name may exist as a real tickets column, or GROUP BY
// would bind the column instead of the alias.
func TestScopedCTEAliasesCannotCollide(t *testing.T) {
	db := scopedFixture(t)
	for _, alias := range []string{"agent_label", "created_at"} {
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('tickets') WHERE name = ?`, alias).Scan(&n); err != nil {
			t.Fatalf("pragma: %v", err)
		}
		if n != 0 {
			t.Errorf("tickets has a real %q column; the alias would collide exactly like `who` does", alias)
		}
	}
}

// TestNovelTicketCommandsUseSharedFragment is the source-level guard: the
// commands #211 names must read through the fragment rather than the blank
// generated columns, and must not reintroduce the colliding grouping key.
func TestNovelTicketCommandsUseSharedFragment(t *testing.T) {
	groupByWho := regexp.MustCompile(`GROUP BY\s+who\b`)
	for _, file := range []string{
		"standup.go",
		"agent_workload.go",
		"tickets_ageout.go",
		"client_card.go",
	} {
		b, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		src := string(b)
		if !strings.Contains(src, "ticketAgentScopedCTE") {
			t.Errorf("%s does not use ticketAgentScopedCTE; it would read the blank agent_name/datecreated columns", file)
		}
		// Only the tickets-backed aggregates are in scope. Commands reading
		// FROM actions legitimately group on `who` because actions has no such
		// column, so restrict the ban to queries selecting FROM tickets.
		for _, stmt := range strings.Split(src, "`") {
			if !strings.Contains(stmt, "FROM tickets") && !strings.Contains(stmt, "FROM scoped") {
				continue
			}
			if groupByWho.MatchString(stmt) {
				t.Errorf("%s groups a tickets-backed query by `who`, which binds the real NULL column", file)
			}
		}
	}
}
