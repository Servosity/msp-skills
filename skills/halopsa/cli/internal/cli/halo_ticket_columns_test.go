package cli

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

// haloFixture builds the ticket/agent shape a REAL Halo sync produces, as
// reported in #203: agent_id and dateoccurred populated, agent_name and
// datecreated blank on every row, and the generated `who` column NULL.
func haloFixture(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	for _, stmt := range []string{
		`CREATE TABLE tickets (
			id INTEGER PRIMARY KEY, data JSON NOT NULL, agent_id INTEGER,
			agent_name TEXT, datecreated TEXT, client_name TEXT, team TEXT,
			summary TEXT, who TEXT
		)`,
		`CREATE TABLE resources (
			id TEXT NOT NULL, resource_type TEXT NOT NULL, data JSON NOT NULL,
			PRIMARY KEY (resource_type, id)
		)`,
		`INSERT INTO resources (id, resource_type, data) VALUES
			('11','agent','{"id":11,"name":"Ada"}'),
			('22','agent','{"id":22,"name":"Grace"}')`,
		`INSERT INTO tickets (id, data, agent_id, agent_name, datecreated, client_name, team, summary, who) VALUES
			(1,'{"status_id":1,"dateoccurred":"2020-01-01T00:00:00Z","lastactiondate":"2020-02-01T00:00:00Z"}',11,'','','Acme','A','t1',NULL),
			(2,'{"status_id":1,"dateoccurred":"2020-06-01T00:00:00Z"}',11,'','','Acme','A','t2',NULL),
			(3,'{"status_id":8,"dateoccurred":"2021-01-01T00:00:00Z","lastactiondate":"2021-02-01T00:00:00Z","reopened":2}',22,'','','Globex','A','t3',NULL)`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("exec %.40s: %v", stmt, err)
		}
	}
	return db
}

func queryStrings(t *testing.T, db *sql.DB, q string, args ...any) []string {
	t.Helper()
	rows, err := db.Query(q, args...)
	if err != nil {
		t.Fatalf("query: %v\nSQL:\n%s", err, q)
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var v sql.NullString
		if err := rows.Scan(&v); err != nil {
			t.Fatalf("scan: %v", err)
		}
		out = append(out, v.String)
	}
	return out
}

// The agent label must come from the synced agent records, because Halo leaves
// tickets.agent_name blank on every row.
func TestHaloAgentLabelExprResolvesFromAgentRecords(t *testing.T) {
	db := haloFixture(t)
	got := queryStrings(t, db, `SELECT `+haloAgentLabelExpr("", "(unassigned)")+` FROM tickets ORDER BY id`)

	want := []string{"Ada", "Ada", "Grace"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("got %v, want %v: names must resolve via agent_id when agent_name is blank", got, want)
	}
}

// A populated agent_name still wins, so a future Halo version needs no change.
func TestHaloAgentLabelExprPrefersPopulatedColumn(t *testing.T) {
	db := haloFixture(t)
	if _, err := db.Exec(`UPDATE tickets SET agent_name = 'Explicit' WHERE id = 1`); err != nil {
		t.Fatalf("update: %v", err)
	}
	got := queryStrings(t, db, `SELECT `+haloAgentLabelExpr("", "(unassigned)")+` FROM tickets WHERE id = 1`)
	if len(got) != 1 || got[0] != "Explicit" {
		t.Fatalf("got %v, want [Explicit]", got)
	}
}

// An unresolvable agent falls back to the caller's label rather than NULL.
func TestHaloAgentLabelExprFallsBackWhenUnresolvable(t *testing.T) {
	db := haloFixture(t)
	if _, err := db.Exec(`UPDATE tickets SET agent_id = 999 WHERE id = 1`); err != nil {
		t.Fatalf("update: %v", err)
	}
	got := queryStrings(t, db, `SELECT `+haloAgentLabelExpr("", "(unassigned)")+` FROM tickets WHERE id = 1`)
	if len(got) != 1 || got[0] != "(unassigned)" {
		t.Fatalf("got %v, want [(unassigned)]", got)
	}
}

// datecreated is blank, so any age computed from it was 0; dateoccurred is the
// real creation timestamp.
func TestHaloTicketCreatedExprFallsThroughToDateoccurred(t *testing.T) {
	db := haloFixture(t)
	got := queryStrings(t, db, `SELECT `+haloTicketCreatedExpr("")+` FROM tickets WHERE id = 1`)
	if len(got) != 1 || got[0] != "2020-01-01T00:00:00Z" {
		t.Fatalf("got %v, want [2020-01-01T00:00:00Z]", got)
	}

	ages := queryStrings(t, db,
		`SELECT CAST(julianday('now') - julianday(`+haloTicketCreatedExpr("")+`) AS INTEGER) FROM tickets ORDER BY id`)
	for _, a := range ages {
		if a == "0" || a == "" {
			t.Fatalf("ticket age computed as %q; this is the oldest_days=0 symptom from #203", a)
		}
	}
}

// Last activity prefers the most recent action, else the creation timestamp.
func TestHaloTicketActivityExprPrefersLastAction(t *testing.T) {
	db := haloFixture(t)
	got := queryStrings(t, db, `SELECT `+haloTicketActivityExpr("")+` FROM tickets ORDER BY id`)
	want := []string{"2020-02-01T00:00:00Z", "2020-06-01T00:00:00Z", "2021-02-01T00:00:00Z"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("got %v, want %v", got, want)
	}
}

// The alias form must qualify every column it touches, so it can be dropped
// into a joined query.
func TestHaloExprsRespectTableAlias(t *testing.T) {
	db := haloFixture(t)
	got := queryStrings(t, db,
		`SELECT `+haloAgentLabelExpr("t", "?")+` FROM tickets t WHERE t.id = 3`)
	if len(got) != 1 || got[0] != "Grace" {
		t.Fatalf("aliased form returned %v, want [Grace]", got)
	}
	if !strings.Contains(haloTicketCreatedExpr("t"), "t.datecreated") ||
		!strings.Contains(haloTicketCreatedExpr("t"), "t.data") {
		t.Fatal("aliased created-expr must qualify both datecreated and data")
	}
}

// standup carried triage's exact defect: it aliased its grouping expression AS
// who over `FROM tickets`, and tickets has a real, always-NULL `who` column that
// SQLite binds GROUP BY to ahead of the output alias, collapsing every agent
// into one row. Verified by running the shipped shape of the query.
func TestStandupGroupsPerAgentNotCollapsed(t *testing.T) {
	db := haloFixture(t)
	// Both closed tickets belong to different agents once names resolve.
	if _, err := db.Exec(
		`UPDATE tickets SET data = json_set(data,'$.status_id',8) WHERE id IN (1,2,3)`,
	); err != nil {
		t.Fatalf("update: %v", err)
	}
	if _, err := db.Exec(`UPDATE tickets SET agent_id = 22 WHERE id = 2`); err != nil {
		t.Fatalf("update: %v", err)
	}

	q := `WITH scoped AS (
                SELECT
                    ` + haloAgentLabelExpr("", "(unassigned)") + ` AS agent_label,
                    COALESCE(client_name, '?') AS client_label
                FROM tickets
                WHERE json_extract(data, '$.status_id') IN (8,9)
                  AND datetime(` + haloTicketActivityExpr("") + `) >= datetime(?)
            )
            SELECT
                agent_label AS who,
                COUNT(*) AS closed,
                client_label AS top_client
            FROM scoped
            GROUP BY agent_label
            ORDER BY closed DESC LIMIT ?`

	rows, err := db.Query(q, "2000-01-01T00:00:00Z", 10)
	if err != nil {
		t.Fatalf("standup query: %v\nSQL:\n%s", err, q)
	}
	defer rows.Close()
	seen := map[string]int{}
	for rows.Next() {
		var who, client string
		var closed int
		if err := rows.Scan(&who, &closed, &client); err != nil {
			t.Fatalf("scan: %v", err)
		}
		seen[who] = closed
	}
	if len(seen) != 2 {
		t.Fatalf("standup returned %d rows (%v); want one per agent. A single row means "+
			"GROUP BY bound to the NULL tickets.who column", len(seen), seen)
	}
	if _, ok := seen["Ada"]; !ok {
		t.Fatalf("agent names not resolved in standup: %v", seen)
	}
}

// The pre-fix standup query, run against the same fixture, must reproduce the
// collapse. This is the negative control: if it ever stops failing, the fixture
// no longer models the bug and the test above proves nothing.
func TestStandupPreFixQueryCollapsesToOneRow(t *testing.T) {
	db := haloFixture(t)
	if _, err := db.Exec(`UPDATE tickets SET data = json_set(data,'$.status_id',8)`); err != nil {
		t.Fatalf("update: %v", err)
	}
	preFix := `SELECT
                COALESCE(NULLIF(agent_name,''), '(unassigned)') AS who,
                COUNT(*) AS closed
            FROM tickets
            WHERE json_extract(data, '$.status_id') IN (8,9)
            GROUP BY who`
	rows, err := db.Query(preFix)
	if err != nil {
		t.Fatalf("prefix query: %v", err)
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		n++
	}
	if n != 1 {
		t.Fatalf("expected the pre-fix query to collapse to 1 row, got %d; the fixture no "+
			"longer reproduces the #203 defect and the fix test above is not proving anything", n)
	}
}

// The label is interpolated into generated SQL, so a quote in it must not be
// able to terminate the literal.
func TestHaloAgentLabelExprEscapesLabelQuotes(t *testing.T) {
	db := haloFixture(t)
	if _, err := db.Exec(`UPDATE tickets SET agent_id = 999 WHERE id = 1`); err != nil {
		t.Fatalf("update: %v", err)
	}
	got := queryStrings(t, db,
		`SELECT `+haloAgentLabelExpr("", "it's nobody")+` FROM tickets WHERE id = 1`)
	if len(got) != 1 || got[0] != "it's nobody" {
		t.Fatalf("got %v, want [it's nobody]: the label must survive quoting intact", got)
	}
}

// Two agent rows whose ids differ as text but not as integers must not make the
// lookup non-deterministic.
func TestHaloAgentLabelExprIsDeterministicOnAmbiguousIDs(t *testing.T) {
	db := haloFixture(t)
	if _, err := db.Exec(
		`INSERT INTO resources (id, resource_type, data) VALUES ('011','agent','{"id":11,"name":"Shadow"}')`,
	); err != nil {
		t.Fatalf("insert: %v", err)
	}
	for i := 0; i < 5; i++ {
		got := queryStrings(t, db, `SELECT `+haloAgentLabelExpr("", "?")+` FROM tickets WHERE id = 1`)
		if len(got) != 1 {
			t.Fatalf("expected exactly one row, got %v", got)
		}
	}
}
