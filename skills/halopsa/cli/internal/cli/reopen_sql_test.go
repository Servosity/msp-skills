package cli

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

// reopenFixture models the action trails measured on the reporting tenant
// (#218). Ticket 100 is the operator-confirmed case: closed, reopened by a
// customer email, closed again, with ZERO explicit Re-Open actions. It is the
// reason the two signals must be unioned rather than either chosen.
func reopenFixture(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	if _, err := db.Exec(`CREATE TABLE actions (id TEXT PRIMARY KEY, data JSON NOT NULL)`); err != nil {
		t.Fatalf("create: %v", err)
	}
	add := func(key string, ticket int, outcome string) {
		t.Helper()
		if _, err := db.Exec(`INSERT INTO actions (id, data) VALUES (?, json_object('ticket_id', ?, 'outcome', ?))`,
			key, ticket, outcome); err != nil {
			t.Fatalf("insert %s: %v", key, err)
		}
	}

	// 100: repeated Resolved only (the customer-email reopen).
	add("100-10", 100, "Resolved")
	add("100-12", 100, "Email Update")
	add("100-18", 100, "Resolved")
	// 200: explicit Re-Open outcome, resolved once, not yet re-resolved.
	add("200-1", 200, "Resolved")
	add("200-2", 200, "Re-Open Ticket 1st")
	// 300: both signals; explicit count (2) exceeds repeat-resolution count (1).
	add("300-1", 300, "Resolved")
	add("300-2", 300, "Re-Open Ticket 1st")
	add("300-3", 300, "Resolved")
	add("300-4", 300, "Re-Open Ticket 2nd")
	// 400: never reopened.
	add("400-1", 400, "Resolved")
	add("400-2", 400, "Email Update")
	return db
}

func reopenCounts(t *testing.T, db *sql.DB) map[int]int {
	t.Helper()
	rows, err := db.Query(reopenCountsSQL)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer rows.Close()
	out := map[int]int{}
	for rows.Next() {
		var id, n int
		if err := rows.Scan(&id, &n); err != nil {
			t.Fatalf("scan: %v", err)
		}
		out[id] = n
	}
	return out
}

// The headline: a reopen with no explicit Re-Open action is still detected,
// because repeated resolution is an independent signal.
func TestReopenDetectsCustomerEmailReopen(t *testing.T) {
	got := reopenCounts(t, reopenFixture(t))
	if n, ok := got[100]; !ok || n != 1 {
		t.Fatalf("ticket 100 reopens = %d (present=%v), want 1. This ticket has ZERO explicit "+
			"Re-Open actions, so only the repeated-Resolved signal can catch it", got[100], ok)
	}
}

// The mirror case: an explicit Re-Open that has not been re-resolved is only
// visible to the outcome signal.
func TestReopenDetectsUnresolvedExplicitReopen(t *testing.T) {
	got := reopenCounts(t, reopenFixture(t))
	if n, ok := got[200]; !ok || n != 1 {
		t.Fatalf("ticket 200 reopens = %d (present=%v), want 1. It has one Resolved action, so "+
			"only the explicit Re-Open outcome can catch it", got[200], ok)
	}
}

// When both signals fire, take the stronger rather than summing, so a reopen is
// never counted twice.
func TestReopenTakesStrongerSignalNotSum(t *testing.T) {
	got := reopenCounts(t, reopenFixture(t))
	if got[300] != 2 {
		t.Fatalf("ticket 300 reopens = %d, want 2 (explicit=2, repeat-resolved=1); "+
			"summing would give 3 and double-count", got[300])
	}
}

// A ticket resolved once and never reopened must not appear at all.
func TestReopenExcludesNeverReopened(t *testing.T) {
	got := reopenCounts(t, reopenFixture(t))
	if n, ok := got[400]; ok {
		t.Fatalf("ticket 400 was never reopened but reported %d", n)
	}
}

// Negative control: the pre-fix probe finds nothing on this data, which is the
// whole point of #218. If it ever starts matching, the fixture has stopped
// modelling the tenant and the tests above prove nothing.
func TestReopenPreFixTicketMarkerFindsNothing(t *testing.T) {
	db := reopenFixture(t)
	if _, err := db.Exec(`CREATE TABLE tickets (id INTEGER PRIMARY KEY, data JSON NOT NULL)`); err != nil {
		t.Fatalf("create: %v", err)
	}
	for _, id := range []int{100, 200, 300, 400} {
		if _, err := db.Exec(`INSERT INTO tickets (id, data) VALUES (?, json_object('summary','s'))`, id); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}
	var carrier int
	if err := db.QueryRow(`SELECT COUNT(*) FROM tickets WHERE json_extract(data,'$.reopened') IS NOT NULL`).Scan(&carrier); err != nil {
		t.Fatalf("probe: %v", err)
	}
	if carrier != 0 {
		t.Fatalf("the $.reopened marker is present on %d tickets; the fixture no longer models "+
			"the tenant where it is absent tenant-wide", carrier)
	}
	if len(reopenCounts(t, db)) != 3 {
		t.Fatalf("action-derived detection should find 3 reopened tickets where the marker finds 0")
	}
}

// Outcome matching is case-insensitive on the prefix, since LIKE is
// case-insensitive for ASCII in SQLite by default.
func TestReopenOutcomeMatchIsCaseInsensitive(t *testing.T) {
	db := reopenFixture(t)
	if _, err := db.Exec(`INSERT INTO actions (id, data) VALUES ('500-1', json_object('ticket_id', 500, 'outcome', 're-open request'))`); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if reopenCounts(t, db)[500] != 1 {
		t.Fatalf("lowercase 're-open request' outcome was not matched")
	}
}
