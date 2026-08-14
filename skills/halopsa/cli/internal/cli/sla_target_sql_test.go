package cli

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

// slaFixture models what a real Halo sync stores: targetdate parked at the
// 1900-01-01 sentinel, with the live deadlines in fixbydate / respondbydate
// (#213). Ticket 3 is the rare row that genuinely populates targetdate.
func slaFixture(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	if _, err := db.Exec(`CREATE TABLE tickets (id INTEGER PRIMARY KEY, data JSON NOT NULL)`); err != nil {
		t.Fatalf("create: %v", err)
	}
	rows := []struct {
		id   int
		data string
	}{
		{1, `{"fixbydate":"2030-01-02T10:00:00","respondbydate":"2030-01-01T10:00:00","targetdate":"1900-01-01T00:00:00"}`},
		{2, `{"fixbydate":"2030-06-01T00:00:00","respondbydate":"2030-05-01T00:00:00","targetdate":"1900-01-01T00:00:00"}`},
		{3, `{"fixbydate":"","respondbydate":"","targetdate":"2030-09-09T09:00:00"}`},
		{4, `{"fixbydate":"","respondbydate":"","targetdate":"1900-01-01T00:00:00"}`},
	}
	for _, r := range rows {
		if _, err := db.Exec(`INSERT INTO tickets (id, data) VALUES (?, ?)`, r.id, r.data); err != nil {
			t.Fatalf("insert %d: %v", r.id, err)
		}
	}
	return db
}

func slaScan(t *testing.T, db *sql.DB, q string) []sql.NullString {
	t.Helper()
	rows, err := db.Query(q)
	if err != nil {
		t.Fatalf("query: %v\nSQL:\n%s", err, q)
	}
	defer rows.Close()
	out := []sql.NullString{}
	for rows.Next() {
		var v sql.NullString
		if err := rows.Scan(&v); err != nil {
			t.Fatalf("scan: %v", err)
		}
		out = append(out, v)
	}
	return out
}

// The resolution deadline comes from fixbydate, not the sentinel targetdate.
func TestSLAResolutionTargetPrefersFixbydate(t *testing.T) {
	got := slaScan(t, slaFixture(t), `SELECT `+slaResolutionTargetExpr("")+` FROM tickets ORDER BY id`)

	want := []struct {
		valid bool
		val   string
	}{
		{true, "2030-01-02T10:00:00"},
		{true, "2030-06-01T00:00:00"},
		{true, "2030-09-09T09:00:00"}, // falls back to a real targetdate
		{false, ""},                   // sentinel only: no usable target
	}
	if len(got) != len(want) {
		t.Fatalf("got %d rows, want %d", len(got), len(want))
	}
	for i, w := range want {
		if got[i].Valid != w.valid || (w.valid && got[i].String != w.val) {
			t.Fatalf("row %d: got (%q, valid=%v), want (%q, valid=%v)",
				i+1, got[i].String, got[i].Valid, w.val, w.valid)
		}
	}
}

// The sentinel must read as "no target", so has-a-target counts only real ones.
func TestSLAResolutionTargetTreatsSentinelAsAbsent(t *testing.T) {
	got := slaScan(t, slaFixture(t),
		`SELECT COUNT(*) FROM tickets WHERE `+slaResolutionTargetExpr("")+` IS NOT NULL`)
	if got[0].String != "3" {
		t.Fatalf("with_target = %s, want 3: the 1900-01-01 sentinel must not count as a target", got[0].String)
	}
}

// The response deadline resolves independently and ignores the sentinel.
func TestSLAResponseTargetResolvesRespondbydate(t *testing.T) {
	got := slaScan(t, slaFixture(t), `SELECT `+slaResponseTargetExpr("")+` FROM tickets ORDER BY id`)
	if !got[0].Valid || got[0].String != "2030-01-01T10:00:00" {
		t.Fatalf("row 1 response target = %q (valid=%v), want 2030-01-01T10:00:00", got[0].String, got[0].Valid)
	}
	if got[3].Valid {
		t.Fatalf("row 4 has no respondbydate; got %q", got[3].String)
	}
}

// Negative control: the pre-fix expression must find nothing on this fixture.
// If it ever starts matching, the fixture has stopped modelling the sentinel and
// the tests above prove nothing.
func TestSLAPreFixTargetdateQueryFindsNothing(t *testing.T) {
	db := slaFixture(t)
	preFix := `SELECT COUNT(*) FROM tickets
	           WHERE datetime(COALESCE(json_extract(data,'$.targetdate'),''))
	                 BETWEEN datetime('2029-12-01') AND datetime('2030-12-01')`
	got := slaScan(t, db, preFix)
	if got[0].String != "1" {
		t.Fatalf("pre-fix window matched %s rows, want 1 (only the rare real targetdate). "+
			"The fixture no longer reproduces #213's sentinel.", got[0].String)
	}

	fixed := `SELECT COUNT(*) FROM tickets
	          WHERE datetime(` + slaResolutionTargetExpr("") + `)
	                BETWEEN datetime('2029-12-01') AND datetime('2030-12-01')`
	if g := slaScan(t, db, fixed); g[0].String != "3" {
		t.Fatalf("fixed window matched %s rows, want 3", g[0].String)
	}
}

// The aliased form must qualify every column it reads.
func TestSLATargetExprRespectsAlias(t *testing.T) {
	got := slaScan(t, slaFixture(t),
		`SELECT `+slaResolutionTargetExpr("t")+` FROM tickets t WHERE t.id = 1`)
	if !got[0].Valid || got[0].String != "2030-01-02T10:00:00" {
		t.Fatalf("aliased form returned %q (valid=%v)", got[0].String, got[0].Valid)
	}
}
