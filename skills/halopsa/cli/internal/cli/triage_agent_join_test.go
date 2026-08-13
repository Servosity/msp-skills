package cli

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

// triageFixture builds the minimum schema the triage aggregate touches: a
// tickets table carrying Halo's real payload shape (agent_id + dateoccurred
// populated, agent_name + datecreated blank, and a real `who` column that is
// NULL on every row) plus the synced agent records in `resources`.
func triageFixture(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "t.db"))
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

	// Three open tickets: two for Ada, one for Grace. agent_name/datecreated
	// are blank exactly as a real Halo sync leaves them; `who` is NULL.
	mustExec(t, db, `INSERT INTO tickets (id, data, agent_id, agent_name, datecreated, team, who) VALUES
		(1,'{"status_id":1,"dateoccurred":"2020-01-01T00:00:00Z"}',11,'','',NULL,NULL),
		(2,'{"status_id":1,"dateoccurred":"2020-06-01T00:00:00Z"}',11,'','',NULL,NULL),
		(3,'{"status_id":1,"dateoccurred":"2021-01-01T00:00:00Z"}',22,'','',NULL,NULL)`)
	return db
}

func mustExec(t *testing.T, db *sql.DB, q string) {
	t.Helper()
	if _, err := db.Exec(q); err != nil {
		t.Fatalf("exec %s: %v", q, err)
	}
}

type triageRow struct {
	who        string
	openCount  int
	oldestDays int
}

func runTriage(t *testing.T, db *sql.DB, team, agent string) []triageRow {
	t.Helper()
	q, args := buildTriageQuery(team, agent, 7, 24, 50)
	rows, err := db.Query(q, args...)
	if err != nil {
		t.Fatalf("query: %v\nSQL:\n%s", err, q)
	}
	defer rows.Close()

	out := []triageRow{}
	for rows.Next() {
		var r triageRow
		var stale, breach, oldest sql.NullInt64
		if err := rows.Scan(&r.who, &r.openCount, &stale, &breach, &oldest); err != nil {
			t.Fatalf("scan: %v", err)
		}
		r.oldestDays = int(oldest.Int64)
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
	return out
}

// The reported symptom: one "(unassigned)" row for everyone. Two independent
// causes had to be fixed together: the blank agent_name lookup AND the
// GROUP BY alias colliding with the real tickets.who column (#203).
func TestTriageReportsOneRowPerAgent(t *testing.T) {
	got := runTriage(t, triageFixture(t), "", "")

	if len(got) != 2 {
		t.Fatalf("expected one row per agent (2), got %d: %+v\n"+
			"a single row means GROUP BY bound to the NULL tickets.who column instead of the output alias", len(got), got)
	}
	byName := map[string]triageRow{}
	for _, r := range got {
		byName[r.who] = r
	}
	if _, ok := byName["Ada"]; !ok {
		t.Errorf("agent names not resolved from agent_id; got %+v", got)
	}
	if byName["Ada"].openCount != 2 {
		t.Errorf("Ada open_count = %d, want 2", byName["Ada"].openCount)
	}
	if byName["Grace"].openCount != 1 {
		t.Errorf("Grace open_count = %d, want 1", byName["Grace"].openCount)
	}
}

// datecreated is blank on every Halo row; reading it reported oldest_days 0 for
// everyone. The real creation timestamp is dateoccurred.
func TestTriageOldestDaysFallsThroughToDateoccurred(t *testing.T) {
	for _, r := range runTriage(t, triageFixture(t), "", "") {
		if r.oldestDays <= 0 {
			t.Fatalf("%s oldest_days = %d, want > 0: datecreated is blank, so the age must "+
				"come from dateoccurred", r.who, r.oldestDays)
		}
	}
}

// --agent filters on the resolved label, so it must survive the CTE split, and
// its bind arguments must stay in statement order.
func TestTriageAgentFilterMatchesResolvedName(t *testing.T) {
	got := runTriage(t, triageFixture(t), "", "Grace")
	if len(got) != 1 || got[0].who != "Grace" {
		t.Fatalf("--agent Grace returned %+v, want exactly the Grace row", got)
	}
}

// A populated agent_name must still win, so a future Halo version that fills
// the column needs no further change here.
func TestTriagePrefersPopulatedAgentName(t *testing.T) {
	db := triageFixture(t)
	mustExec(t, db, `UPDATE tickets SET agent_name = 'Explicit' WHERE id = 3`)

	found := false
	for _, r := range runTriage(t, db, "", "") {
		if r.who == "Explicit" {
			found = true
		}
	}
	if !found {
		t.Fatalf("a populated agent_name must take precedence over the agent_id join")
	}
}

// Guard the specific collision: the aggregate must not group on a bare `who`.
func TestTriageDoesNotGroupByCollidingAlias(t *testing.T) {
	q, _ := buildTriageQuery("", "", 7, 24, 50)
	if strings.Contains(q, "GROUP BY who") {
		t.Fatal("GROUP BY who collides with the real tickets.who column; SQLite resolves the " +
			"name against source columns before output aliases and collapses every agent into one row")
	}
}
