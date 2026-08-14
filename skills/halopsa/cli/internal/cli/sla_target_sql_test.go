// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored: guards the SLA target field resolution. See
// skills/halopsa/handfixes.json (sla-target-reads-fixbydate).

package cli

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// slaFixture mirrors a real Halo sync: targetdate parked at the 1900-01-01
// sentinel while the usable deadlines live in fixbydate and respondbydate.
func slaFixture(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "sla.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	mustExec(t, db, `CREATE TABLE tickets (id INTEGER PRIMARY KEY, data JSON NOT NULL)`)
	mustExec(t, db, `INSERT INTO tickets (id, data) VALUES
		-- Sentinel targetdate, real fixbydate due in ~16h: must be seen.
		(1, json_object('targetdate','1900-01-01T00:00:00',
		                'fixbydate', datetime('now','+16 hours'),
		                'respondbydate', datetime('now','-27 hours'))),
		-- Sentinel targetdate, fixbydate far out: must not be seen.
		(2, json_object('targetdate','1900-01-01T00:00:00',
		                'fixbydate', datetime('now','+9 days'))),
		-- No fixbydate but a real targetdate: the fallback must still work.
		(3, json_object('targetdate', datetime('now','+3 hours'))),
		-- Nothing usable at all: must resolve to NULL, not to the sentinel.
		(4, json_object('targetdate','1900-01-01T00:00:00'))`)
	return db
}

func resolvedTarget(t *testing.T, db *sql.DB, id int) (string, bool) {
	t.Helper()
	var v sql.NullString
	if err := db.QueryRow(`SELECT `+slaResolutionTargetSQL+` FROM tickets WHERE id = ?`, id).Scan(&v); err != nil {
		t.Fatalf("resolve target for %d: %v", id, err)
	}
	return v.String, v.Valid
}

// TestSLAResolutionTargetPrefersFixbydate is the core regression: the sentinel
// must never be treated as a deadline, and fixbydate must win.
func TestSLAResolutionTargetPrefersFixbydate(t *testing.T) {
	db := slaFixture(t)

	got, ok := resolvedTarget(t, db, 1)
	if !ok {
		t.Fatal("ticket 1 resolved to NULL; fixbydate carries the real deadline")
	}
	if strings.HasPrefix(got, "1900-01-01") {
		t.Fatalf("resolved target = %q, want fixbydate rather than the sentinel", got)
	}
}

// TestSLAResolutionTargetFallsBackToTargetdate keeps tenants that genuinely
// populate targetdate working.
func TestSLAResolutionTargetFallsBackToTargetdate(t *testing.T) {
	db := slaFixture(t)
	if got, ok := resolvedTarget(t, db, 3); !ok || strings.HasPrefix(got, "1900-01-01") {
		t.Fatalf("ticket 3 resolved to (%q, ok=%v); a real targetdate must be used when fixbydate is absent", got, ok)
	}
}

// TestSLAResolutionTargetIsNullWhenAbsent pins the sentinel handling, so
// "has a target" tests count real deadlines only.
func TestSLAResolutionTargetIsNullWhenAbsent(t *testing.T) {
	db := slaFixture(t)
	if got, ok := resolvedTarget(t, db, 4); ok {
		t.Fatalf("ticket 4 resolved to %q, want NULL; the 1900-01-01 sentinel is not a deadline", got)
	}
}

// TestSLABreachWindowMatchesOnlyRealDeadlines is the end-to-end shape of the
// breach query: pre-fix this returned zero rows on every tenant because the
// sentinel can never fall inside the window.
func TestSLABreachWindowMatchesOnlyRealDeadlines(t *testing.T) {
	db := slaFixture(t)

	rows, err := db.Query(`SELECT id FROM tickets
		WHERE datetime(` + slaResolutionTargetSQL + `) BETWEEN datetime('now') AND datetime('now','+24 hours')
		ORDER BY id`)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer rows.Close()

	var got []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("scan: %v", err)
		}
		got = append(got, id)
	}
	// 1 is due in 16h via fixbydate, 3 in 3h via the targetdate fallback.
	// 2 is 9 days out and 4 has no deadline.
	want := []int{1, 3}
	if len(got) != len(want) {
		t.Fatalf("breaching = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("breaching = %v, want %v", got, want)
		}
	}
}

// TestSLAScorecardCountsRealTargetsOnly covers the met/with-target pair, which
// pre-fix reported every ticket as having an unmeetable target.
func TestSLAScorecardCountsRealTargetsOnly(t *testing.T) {
	db := slaFixture(t)
	mustExec(t, db, `INSERT INTO tickets (id, data) VALUES
		(5, json_object('targetdate','1900-01-01T00:00:00',
		                'fixbydate', datetime('now','-2 days'),
		                'lastactiondate', datetime('now','-3 days')))`)

	var withTarget, met int
	if err := db.QueryRow(`SELECT
		SUM(CASE WHEN (`+slaResolutionTargetSQL+`) IS NOT NULL THEN 1 ELSE 0 END),
		SUM(CASE WHEN (`+slaResolutionTargetSQL+`) IS NOT NULL
		          AND datetime(json_extract(data,'$.lastactiondate')) <= datetime(`+slaResolutionTargetSQL+`)
		     THEN 1 ELSE 0 END)
		FROM tickets WHERE id = 5`).Scan(&withTarget, &met); err != nil {
		t.Fatalf("scorecard query: %v", err)
	}
	if withTarget != 1 {
		t.Fatalf("with_target = %d, want 1", withTarget)
	}
	if met != 1 {
		t.Fatalf("met = %d, want 1; the ticket was actioned before its fixbydate", met)
	}
}

// TestSLAViewsDoNotReadTargetdateDirectly is the source-level guard: the
// SLA-facing commands must go through the shared expression rather than
// reading the legacy column.
func TestSLAViewsDoNotReadTargetdateDirectly(t *testing.T) {
	for _, file := range []string{
		"sla_breaching.go",
		"sla_scorecard.go",
		"triage.go",
		"client_card.go",
	} {
		b, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		src := string(b)
		if !strings.Contains(src, "slaResolutionTargetSQL") {
			t.Errorf("%s does not use slaResolutionTargetSQL", file)
		}
		for _, raw := range []string{`json_extract(data,'$.targetdate')`, `json_extract(data, '$.targetdate')`} {
			if strings.Contains(src, raw) {
				t.Errorf("%s still reads targetdate directly; HaloPSA leaves it at the 1900-01-01 sentinel", file)
			}
		}
	}
}
